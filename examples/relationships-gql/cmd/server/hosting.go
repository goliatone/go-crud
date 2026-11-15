package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/goliatone/go-router"
)

// socketOwner seals admission and joins transport handlers before the database closes.
// The transport itself joins its initialization reader, keepalive and operation workers.
type socketOwner struct {
	mu       sync.Mutex
	active   map[router.WebSocketReadInterrupter]context.CancelFunc
	stopping bool
	done     chan struct{}
}

func newSocketOwner() *socketOwner {
	return &socketOwner{active: make(map[router.WebSocketReadInterrupter]context.CancelFunc), done: make(chan struct{})}
}

func (s *socketOwner) wrap(next func(router.WebSocketContext) error) func(router.WebSocketContext) error {
	return func(ws router.WebSocketContext) error {
		reader, ok := ws.(router.WebSocketReadInterrupter)
		if !ok {
			return errors.New("websocket read interruption unavailable")
		}
		s.mu.Lock()
		if s.stopping {
			s.mu.Unlock()
			return ws.Close()
		}
		ctx, cancel := context.WithCancel(context.WithoutCancel(ws.Context()))
		s.active[reader] = cancel
		s.mu.Unlock()
		defer func() {
			cancel()
			_ = ws.Close()
			s.mu.Lock()
			delete(s.active, reader)
			if s.stopping && len(s.active) == 0 {
				close(s.done)
			}
			s.mu.Unlock()
		}()
		return next(newWebsocketContext(ctx, nil, ws))
	}
}

func (s *socketOwner) shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.stopping {
		s.stopping = true
		// Keep removal serialized with interruption: never touch a recycled connection.
		for reader, cancel := range s.active {
			cancel()
			_ = reader.InterruptRead()
		}
		if len(s.active) == 0 {
			close(s.done)
		}
	}
	s.mu.Unlock()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func serve(ctx context.Context, server router.ManagedServer, sockets *socketOwner, addr string, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	run, err := server.StartManaged(context.WithoutCancel(ctx), router.ManagedOptions{
		Address: addr, Network: "tcp4", DrainTimeout: timeout, OnShutdown: sockets.shutdown,
	})
	if err != nil {
		return err
	}
	serving := run.Serving()
	for {
		select {
		case <-serving:
			log.Printf("GraphQL serving at http://%s/graphql (Playground: /playground)", run.Address())
			serving = nil
		case <-run.Done():
			return run.Wait(context.Background())
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			stopErr := run.Stop(shutdown)
			return errors.Join(stopErr, run.Wait(shutdown))
		}
	}
}
