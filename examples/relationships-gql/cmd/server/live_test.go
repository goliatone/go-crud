package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/goliatone/go-auth"
	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/resolvers"
	"github.com/goliatone/go-router"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type observedBus struct {
	resolvers.EventBus
	registered chan (<-chan resolvers.EventMessage)
}

func (b *observedBus) Subscribe(ctx context.Context, topic string) (<-chan resolvers.EventMessage, error) {
	ch, err := b.EventBus.Subscribe(ctx, topic)
	if err == nil {
		b.registered <- ch
	}
	return ch, err
}

func testHost(t *testing.T, configure ...func(*router.WebSocketConfig)) (router.Server[*fiber.App], *socketOwner, *resolvers.Resolver) {
	t.Helper()
	client, err := relationships.SetupDatabase(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	db := client.DB()
	require.NoError(t, relationships.MigrateSchema(t.Context(), db))
	require.NoError(t, relationships.SeedDatabase(t.Context(), client))
	resolver := resolvers.NewResolver(relationships.RegisterRepositories(db))
	app, owner, err := newServer(resolver, db, configure...)
	require.NoError(t, err)
	return app, owner, resolver
}

func postGraphQL(t *testing.T, address, query, token string) map[string]json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), "POST", "http://"+address+"/graphql", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	var result map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

func readWS(t *testing.T, conn *websocket.Conn, kind string) map[string]json.RawMessage {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	for {
		var msg map[string]json.RawMessage
		require.NoError(t, conn.ReadJSON(&msg))
		if string(msg["type"]) == `"`+kind+`"` {
			return msg
		}
	}
}

func TestGraphQLHostLiveContracts(t *testing.T) {
	app, owner, resolver := testHost(t)
	bus := &observedBus{EventBus: resolver.Events, registered: make(chan (<-chan resolvers.EventMessage), 4)}
	resolver.Events = bus
	guard := resolver.ScopeGuard
	resolver.ScopeGuard = func(ctx context.Context, entity, action string) error {
		if err := guard(ctx, entity, action); err != nil {
			return err
		}
		user, ok := auth.FromContext(ctx)
		if !ok {
			return errors.New("missing user")
		}
		actor, ok := auth.ActorFromContext(ctx)
		if !ok || actor.ActorID != user.ID.String() {
			return errors.New("actor context mismatch")
		}
		return nil
	}
	run, cancel, done := startObserved(t, app.(router.ManagedServer), 2*time.Second, nil, owner)
	await(t, run.Serving())
	query := `{ listAuthor { edges { node { fullName profile { biography } books { title } tags { name } } } } }`
	result := postGraphQL(t, run.Address(), query, "demo")
	require.Empty(t, result["errors"], string(result["errors"]))
	require.Contains(t, string(result["data"]), "Lina Ortiz")
	require.Contains(t, string(result["data"]), "Contact Shadows")
	denied := postGraphQL(t, run.Address(), `mutation { createTag(input: {name:"denied", category:"demo"}) { id } }`, "")
	require.Contains(t, string(denied["errors"]), "unauthorized")
	tags := postGraphQL(t, run.Address(), `{ listTag { edges { node { name } } } }`, "demo")
	require.Empty(t, tags["errors"])
	require.NotContains(t, string(tags["data"]), "denied")
	httpClient := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer httpClient.CloseIdleConnections()
	for _, path := range []string{"/", "/playground", "/missing"} {
		resp, err := httpClient.Get("http://" + run.Address() + path)
		require.NoError(t, err)
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, err)
		switch path {
		case "/":
			require.Equal(t, 307, resp.StatusCode)
			require.Equal(t, "/playground", resp.Header.Get("Location"))
		case "/playground":
			require.Equal(t, 200, resp.StatusCode)
			require.Contains(t, strings.ToLower(string(data)), "html")
		default:
			require.Equal(t, 404, resp.StatusCode)
		}
	}
	var conns []*websocket.Conn
	var streams []<-chan resolvers.EventMessage
	for _, proto := range []string{"graphql-transport-ws", "graphql-ws"} {
		dialer := websocket.Dialer{Subprotocols: []string{proto}, HandshakeTimeout: 3 * time.Second}
		conn, _, err := dialer.Dial("ws://"+run.Address()+"/graphql", http.Header{"Authorization": {"Bearer demo"}})
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })
		conns = append(conns, conn)
		require.Equal(t, proto, conn.Subprotocol())
		require.NoError(t, conn.WriteJSON(map[string]any{"type": "connection_init"}))
		readWS(t, conn, "connection_ack")
		op, event := "subscribe", "next"
		if proto == "graphql-ws" {
			op, event = "start", "data"
		}
		require.NoError(t, conn.WriteJSON(map[string]any{"id": "1", "type": op, "payload": map[string]string{"query": `subscription { tagCreated { name category } }`}}))
		select {
		case ch := <-bus.registered:
			streams = append(streams, ch)
		case <-time.After(3 * time.Second):
			t.Fatal("subscription did not register")
		}
		result := postGraphQL(t, run.Address(), `mutation { createTag(input: {name:"`+proto+`", category:"demo"}) { id name } }`, "demo")
		require.Empty(t, result["errors"], string(result["errors"]))
		msg := readWS(t, conn, event)
		require.Contains(t, string(msg["payload"]), proto)
	}

	for _, proto := range []string{"graphql-transport-ws", "graphql-ws"} {
		conn, resp, err := (&websocket.Dialer{Subprotocols: []string{proto}, HandshakeTimeout: time.Second}).Dial("ws://"+run.Address()+"/graphql", nil)
		if conn != nil {
			conn.Close()
		}
		require.Error(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	select {
	case <-bus.registered:
		t.Fatal("unauthorized subscription registered on bus")
	default:
	}

	// An upgraded connection that never initializes must also be interrupted and joined.
	idle, _, err := (&websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}}).Dial("ws://"+run.Address()+"/graphql", http.Header{"Authorization": {"Bearer demo"}})
	require.NoError(t, err)
	defer idle.Close()
	conns = append(conns, idle)
	require.Eventually(t, func() bool { owner.mu.Lock(); defer owner.mu.Unlock(); return len(owner.active) == 3 }, time.Second, time.Millisecond)
	cancel()
	require.NoError(t, receiveResult(t, done))
	await(t, run.Done())
	await(t, owner.done)
	for _, conn := range conns {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}
	for _, stream := range streams {
		select {
		case _, ok := <-stream:
			require.False(t, ok, "event bus registration survived shutdown")
		case <-time.After(time.Second):
			t.Fatal("event bus worker did not close")
		}
	}
	owner.mu.Lock()
	require.Empty(t, owner.active)
	owner.mu.Unlock()
}

// Controlled socket fixture isolates admission and delayed callback completion.
type heldSocket struct {
	router.WebSocketContext
	closed chan struct{}
	once   sync.Once
}

func (s *heldSocket) Context() context.Context { return context.Background() }
func (s *heldSocket) InterruptRead() error     { return nil }
func (s *heldSocket) Close() error             { s.once.Do(func() { close(s.closed) }); return nil }

func TestSocketOwnerLateAdmissionAndDeadline(t *testing.T) {
	owner := newSocketOwner()
	first := &heldSocket{closed: make(chan struct{})}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	go func() {
		finished <- owner.wrap(func(ws router.WebSocketContext) error { close(entered); <-ws.Context().Done(); <-release; return nil })(first)
	}()
	await(t, entered)
	short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, owner.shutdown(short), context.DeadlineExceeded)
	late := &heldSocket{closed: make(chan struct{})}
	require.NoError(t, owner.wrap(func(router.WebSocketContext) error { return errors.New("late handler admitted") })(late))
	await(t, late.closed)
	select {
	case <-owner.done:
		t.Fatal("did not join active handler")
	default:
	}
	once.Do(func() { close(release) })
	require.NoError(t, receiveResult(t, finished))
	require.NoError(t, owner.shutdown(t.Context()))
	await(t, first.closed)
	await(t, owner.done)
}

func TestGraphQLUpgradeOverlapsShutdown(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	app, owner, _ := testHost(t, func(config *router.WebSocketConfig) {
		config.OnConnect = func(router.WebSocketContext) error { close(entered); <-release; return nil }
	})
	run, cancel, done := startObserved(t, app.(router.ManagedServer), time.Second, nil, owner)
	await(t, run.Serving())
	dialed := make(chan *websocket.Conn, 1)
	go func() {
		conn, _, _ := (&websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}, HandshakeTimeout: 2 * time.Second}).Dial("ws://"+run.Address()+"/graphql", http.Header{"Authorization": {"Bearer demo"}})
		dialed <- conn
	}()
	await(t, entered)
	cancel()
	await(t, run.Stopping())
	await(t, owner.done)
	once.Do(func() { close(release) })
	require.NoError(t, receiveResult(t, done))
	await(t, run.Done())
	select {
	case conn := <-dialed:
		if conn != nil {
			defer conn.Close()
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
			_, _, err := conn.ReadMessage()
			require.Error(t, err)
			var timeout net.Error
			if errors.As(err, &timeout) {
				require.False(t, timeout.Timeout(), "late upgrade remained open")
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("upgrade did not finish")
	}
	owner.mu.Lock()
	require.Empty(t, owner.active)
	owner.mu.Unlock()
}
