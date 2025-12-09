package resolvers

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud/examples/relationships-gql/graph/generated"
)

func TestSubscriptions_WebSocketFlow(t *testing.T) {
	resolver, _, cleanup := setupResolver(t)
	defer cleanup()

	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(&transport.Websocket{
		KeepAlivePingInterval: 5 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	})

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket smoke test; cannot bind listener: %v", err)
	}
	ts := httptest.NewUnstartedServer(srv)
	ts.Listener = ln
	ts.Start()
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/graphql"
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
	resp, err := ts.Client().Post(ts.URL+"/graphql", "application/json", bytes.NewBuffer(body))
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
