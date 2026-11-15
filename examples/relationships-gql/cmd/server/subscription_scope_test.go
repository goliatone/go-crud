package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/goliatone/go-crud/examples/relationships-gql/graph/resolvers"
	"github.com/goliatone/go-crud/gql/helpers"
	"github.com/goliatone/go-router"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type scopeRegistration struct {
	ctx     context.Context
	release func()
}
type scopeProbeBus struct {
	resolvers.EventBus
	registered chan scopeRegistration
}

func (b *scopeProbeBus) Subscribe(ctx context.Context, topic string) (<-chan resolvers.EventMessage, error) {
	stream, err := b.EventBus.Subscribe(ctx, topic)
	if err != nil {
		return nil, err
	}
	// A controlled registration proves the real host waits for scoped work after
	// the executor stops. Actual generated blocked sends are tested separately.
	release, err := helpers.BeginSubscription(ctx)
	if err != nil {
		return nil, err
	}
	b.registered <- scopeRegistration{ctx, release}
	return stream, nil
}

func TestHostJoinsSubscriptionScopeBeforeSocketRemoval(t *testing.T) {
	for _, mode := range []string{"disconnect", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			app, owner, resolver := testHost(t)
			bus := &scopeProbeBus{EventBus: resolver.Events, registered: make(chan scopeRegistration, 1)}
			resolver.Events = bus
			run, cancel, done := startObserved(t, app.(router.ManagedServer), 2*time.Second, nil, owner)
			await(t, run.Serving())
			dialer := websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}, HandshakeTimeout: time.Second}
			conn, _, err := dialer.Dial("ws://"+run.Address()+"/graphql", http.Header{"Authorization": {"Bearer demo"}})
			require.NoError(t, err)
			t.Cleanup(func() { conn.Close() })
			require.NoError(t, conn.WriteJSON(map[string]any{"type": "connection_init"}))
			readWS(t, conn, "connection_ack")
			require.NoError(t, conn.WriteJSON(map[string]any{"id": "1", "type": "subscribe", "payload": map[string]string{"query": `subscription { tagCreated { name } }`}}))
			var registration scopeRegistration
			select {
			case registration = <-bus.registered:
			case <-time.After(time.Second):
				t.Fatal("subscription did not register")
			}
			t.Cleanup(registration.release)
			if mode == "disconnect" {
				require.NoError(t, conn.Close())
			} else {
				cancel()
			}
			await(t, registration.ctx.Done())
			wait, stop := context.WithTimeout(t.Context(), 25*time.Millisecond)
			defer stop()
			require.ErrorIs(t, owner.shutdown(wait), context.DeadlineExceeded, "transport ownership ended before scoped work")
			owner.mu.Lock()
			active := len(owner.active)
			owner.mu.Unlock()
			require.Equal(t, 1, active)
			select {
			case <-owner.done:
				t.Fatal("socket owner returned before scoped release")
			default:
			}
			registration.release()
			await(t, owner.done)
			cancel()
			require.NoError(t, receiveResult(t, done))
			await(t, run.Done())
		})
	}
}
