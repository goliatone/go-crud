package resolvers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-auth"
	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/dataloader"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/generated"
	"github.com/goliatone/go-crud/examples/relationships-gql/internal/routerws"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

func TestSubscriptions_WebSocketFlow(t *testing.T) {
	ctx := t.Context()
	user := &auth.User{ID: uuid.New(), Username: "subscription-test", Role: auth.RoleAdmin}
	withAuth := func(ctx context.Context) context.Context {
		ctx = auth.WithContext(ctx, user)
		return auth.WithActorContext(ctx, &auth.ActorContext{ActorID: user.ID.String(), Role: string(auth.RoleAdmin)})
	}
	client, err := relationships.SetupDatabase(ctx)
	require.NoError(t, err)
	require.NotNil(t, client)
	t.Cleanup(func() {
		if client != nil {
			_ = client.Close()
		}
	})

	db := client.DB()
	require.NoError(t, relationships.MigrateSchema(ctx, db))
	require.NoError(t, relationships.SeedDatabase(ctx, client))

	resolver := NewResolver(relationships.RegisterRepositories(db))
	bus := &recordingBus{EventBus: NewEventBus()}
	resolver.Events = bus

	executableSchema := generated.NewExecutableSchema(generated.Config{Resolvers: resolver})
	httpSrv := handler.New(executableSchema)
	httpSrv.AddTransport(transport.Options{})
	httpSrv.AddTransport(transport.POST{})

	wsExec := executor.New(executableSchema)
	wsTransport := routerws.Websocket{
		KeepAlivePingInterval: 5 * time.Second,
		MissingPongOk:         true,
	}
	wsHandler := wsTransport.Handler(wsExec)

	app := router.NewFiberAdapter()
	_ = app.Router()
	wsConfig := router.DefaultWebSocketConfig()
	wsConfig.Subprotocols = []string{"graphql-transport-ws", "graphql-ws"}
	wsConfig.PingPeriod = 5 * time.Second
	wsConfig.PongWait = 10 * time.Second
	wsConfig.CheckOrigin = func(origin string) bool { return true }
	wsConfig.OnPreUpgrade = func(c router.Context) (router.UpgradeData, error) {
		return router.UpgradeData{
			"authorization": c.Header("Authorization"),
		}, nil
	}

	handlerDone := make(chan struct{})
	wsFiberHandler := router.FiberWebSocketHandler(wsConfig, func(ws router.WebSocketContext) error {
		defer close(handlerDone)
		ctx := withAuth(ws.Context())
		if ldr := newTestLoader(resolver, db); ldr != nil {
			ctx = dataloader.Inject(ctx, ldr)
		}
		ws.SetContext(ctx)
		return wsHandler(ws)
	})

	secured := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(withAuth(r.Context()))
		if ldr := newTestLoader(resolver, db); ldr != nil {
			r = r.WithContext(dataloader.Inject(r.Context(), ldr))
		}
		httpSrv.ServeHTTP(w, r)
	})
	app.Init()

	fiberApp := app.WrappedRouter()
	fiberApp.All("/graphql", func(c *fiber.Ctx) error {
		if fiberws.IsWebSocketUpgrade(c) {
			return wsFiberHandler(c)
		}
		return adaptor.HTTPHandler(secured)(c)
	})

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err, "cannot bind listener for websocket smoke test")

	errCh := make(chan error, 1)
	go func() {
		errCh <- fiberApp.Listener(ln)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, app.Shutdown(shutdownCtx))
		select {
		case <-errCh:
		default:
		}
	}()

	wsURL := "ws://" + ln.Addr().String() + "/graphql"
	dialer := websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}}
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err, "websocket upgrade failed")
	defer conn.Close()

	require.NoError(t, conn.WriteJSON(map[string]any{
		"type": "connection_init",
	}))

	readWithTimeout(t, conn, func(msg map[string]any) bool {
		return msg["type"] == "connection_ack"
	})

	subscription := `subscription { tagCreated { id name category } }`
	require.NoError(t, conn.WriteJSON(map[string]any{
		"id":   "1",
		"type": "subscribe",
		"payload": map[string]any{
			"query": subscription,
		},
	}))

	// Subscription registration is asynchronous; publish only once the bus is listening.
	require.Eventually(t, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return slices.Contains(bus.subscribed, "tag.created")
	}, 5*time.Second, 10*time.Millisecond)

	mutation := `mutation { createTag(input: { name: "sub-demo", category: "demo" }) { id name } }`
	body, err := json.Marshal(map[string]string{"query": mutation})
	require.NoError(t, err)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	defer httpClient.CloseIdleConnections()
	resp, err := httpClient.Post("http://"+ln.Addr().String()+"/graphql", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	responseBody, readErr := io.ReadAll(resp.Body)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, readErr)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var result struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []json.RawMessage          `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(responseBody, &result))
	require.Empty(t, result.Errors, string(responseBody))
	require.NotEmpty(t, result.Data["createTag"], string(responseBody))

	readWithTimeout(t, conn, func(msg map[string]any) bool {
		if msg["type"] != "next" {
			return false
		}
		payload, ok := msg["payload"].(map[string]any)
		if !ok {
			return false
		}
		data, ok := payload["data"].(map[string]any)
		if !ok {
			return false
		}
		created, ok := data["tagCreated"].(map[string]any)
		if !ok {
			return false
		}
		return created["name"] == "sub-demo" && created["category"] == "demo"
	})

	require.NoError(t, conn.WriteJSON(map[string]any{"id": "1", "type": "complete"}))
	require.NoError(t, conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")))

	bus.mu.Lock()
	subscribed := append([]string(nil), bus.subscribed...)
	published := append([]string(nil), bus.published...)
	bus.mu.Unlock()
	require.Contains(t, subscribed, "tag.created")
	require.Contains(t, published, "tag.created")
	require.NoError(t, conn.Close())
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("websocket handler did not finish after disconnect")
	}
}

func newTestLoader(resolver *Resolver, db bun.IDB) *dataloader.Loader {
	if resolver == nil {
		return nil
	}

	services := dataloader.Services{
		Author:          resolver.AuthorSvc,
		AuthorProfile:   resolver.AuthorProfileSvc,
		Book:            resolver.BookSvc,
		Chapter:         resolver.ChapterSvc,
		Headquarters:    resolver.HeadquartersSvc,
		PublishingHouse: resolver.PublishingHouseSvc,
		Tag:             resolver.TagSvc,
	}

	opts := []dataloader.Option{}
	if db != nil {
		opts = append(opts, dataloader.WithDB(db))
	}
	if resolver.ContextFactory != nil {
		opts = append(opts, dataloader.WithContextFactory(resolver.ContextFactory))
	}

	return dataloader.New(services, opts...)
}

func readWithTimeout(t *testing.T, conn *websocket.Conn, match func(map[string]any) bool) {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	defer func() { require.NoError(t, conn.SetReadDeadline(time.Time{})) }()
	for {
		var msg map[string]any
		require.NoError(t, conn.ReadJSON(&msg), "read websocket message")
		if match(msg) {
			return
		}
	}
}

type recordingBus struct {
	EventBus
	mu         sync.Mutex
	published  []string
	subscribed []string
}

func (b *recordingBus) Publish(ctx context.Context, topic string, payload any) error {
	b.mu.Lock()
	b.published = append(b.published, topic)
	b.mu.Unlock()
	if b.EventBus == nil {
		return nil
	}
	return b.EventBus.Publish(ctx, topic, payload)
}

func (b *recordingBus) Subscribe(ctx context.Context, topic string) (<-chan EventMessage, error) {
	if b.EventBus == nil {
		ch := make(chan EventMessage)
		close(ch)
		return ch, nil
	}
	ch, err := b.EventBus.Subscribe(ctx, topic)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.subscribed = append(b.subscribed, topic)
	b.mu.Unlock()
	return ch, nil
}
