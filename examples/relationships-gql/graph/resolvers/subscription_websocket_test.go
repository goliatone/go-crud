package resolvers

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
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

	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/dataloader"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/generated"
	"github.com/goliatone/go-crud/examples/relationships-gql/internal/routerws"
	"github.com/goliatone/go-router"
	"github.com/uptrace/bun"
)

func TestSubscriptions_WebSocketFlow(t *testing.T) {
	ctx := context.Background()
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

	wsFiberHandler := router.FiberWebSocketHandler(wsConfig, func(ws router.WebSocketContext) error {
		ctx := context.Background()
		if ldr := newTestLoader(resolver, db); ldr != nil {
			ctx = dataloader.Inject(ctx, ldr)
		}
		ws.SetContext(ctx)
		return wsHandler(ws)
	})

	secured := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		_ = app.Shutdown(context.Background())
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

	mutation := `mutation { createTag(input: { name: "sub-demo", category: "demo" }) { id name } }`
	body, err := json.Marshal(map[string]string{"query": mutation})
	require.NoError(t, err)
	resp, err := http.Post("http://"+ln.Addr().String()+"/graphql", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

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

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			t.Fatalf("timed out waiting for websocket message")
		default:
			var msg map[string]any
			if err := conn.ReadJSON(&msg); err != nil {
				t.Fatalf("read websocket message: %v", err)
			}
			if match(msg) {
				return
			}
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
	b.mu.Lock()
	b.subscribed = append(b.subscribed, topic)
	b.mu.Unlock()
	if b.EventBus == nil {
		ch := make(chan EventMessage)
		close(ch)
		return ch, nil
	}
	return b.EventBus.Subscribe(ctx, topic)
}
