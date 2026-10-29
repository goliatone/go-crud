package routerws

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/goliatone/go-router"
)

func TestHandlerJoinsInitializationReader(t *testing.T) {
	for _, cancelContext := range []bool{false, true} {
		name := "timeout"
		if cancelContext {
			name = "context cancellation"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ws := &blockedInitialization{
				ctx: ctx, started: make(chan struct{}), readDone: make(chan struct{}), closed: make(chan struct{}),
			}
			defer ws.CloseWithStatus(router.CloseNormalClosure, "cleanup")
			transport := Websocket{InitTimeout: 20 * time.Millisecond}
			if cancelContext {
				transport.InitTimeout = 0
			}
			done := make(chan error, 1)
			go func() { done <- transport.Handler(nil)(ws) }()
			select {
			case <-ws.started:
			case <-time.After(time.Second):
				t.Fatal("initialization reader did not start")
			}
			if cancelContext {
				cancel()
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("initialization did not stop")
			}
			select {
			case <-ws.readDone:
			default:
				t.Fatal("handler returned while initialization reader was still active")
			}
		})
	}
}

type blockedInitialization struct {
	router.WebSocketContext
	ctx      context.Context
	started  chan struct{}
	readDone chan struct{}
	closed   chan struct{}
	once     sync.Once
}

func (ws *blockedInitialization) Header(string) string     { return "" }
func (ws *blockedInitialization) Context() context.Context { return ws.ctx }
func (ws *blockedInitialization) Subprotocol() string      { return graphqltransportwsSubprotocol }
func (ws *blockedInitialization) ReadMessage() (int, []byte, error) {
	close(ws.started)
	defer close(ws.readDone)
	<-ws.closed
	return 0, nil, io.EOF
}
func (ws *blockedInitialization) CloseWithStatus(int, string) error {
	ws.once.Do(func() { close(ws.closed) })
	return nil
}
