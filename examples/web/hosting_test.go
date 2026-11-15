package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/goliatone/go-router"
	"github.com/stretchr/testify/require"
)

// Observe the actual production coordinator without replacing native hosting.
type observedServer struct {
	router.ManagedServer
	runs      chan router.ManagedRun
	configure func(*router.ManagedOptions)
}

func (s *observedServer) StartManaged(ctx context.Context, opts router.ManagedOptions) (router.ManagedRun, error) {
	if s.configure != nil {
		s.configure(&opts)
	}
	run, err := s.ManagedServer.StartManaged(ctx, opts)
	if err == nil {
		s.runs <- run
	}
	return run, err
}
func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for lifecycle boundary")
	}
}
func receiveResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(4 * time.Second):
		t.Fatal("coordinator did not finish")
		return nil
	}
}
func startObserved(t *testing.T, server router.ManagedServer, timeout time.Duration, configure func(*router.ManagedOptions)) (router.ManagedRun, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	observed := &observedServer{ManagedServer: server, runs: make(chan router.ManagedRun, 1), configure: configure}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, observed, "127.0.0.1:0", timeout) }()
	var run router.ManagedRun
	select {
	case run = <-observed.runs:
	case err := <-done:
		cancel()
		t.Fatalf("start: %v", err)
	case <-time.After(4 * time.Second):
		cancel()
		t.Fatal("start timed out")
	}
	t.Cleanup(func() {
		cancel()
		wait, stop := context.WithTimeout(context.Background(), 4*time.Second)
		defer stop()
		_ = run.Stop(wait)
		if err := run.Wait(wait); errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, router.ErrManagedDrainTimeout) {
			t.Errorf("managed workers did not join: %v", err)
		}
	})
	return run, cancel, done
}

func TestServeStartupFailure(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	server := testApp(t).server
	result := make(chan error, 1)
	go func() {
		result <- serve(context.Background(), server.(router.ManagedServer), listener.Addr().String(), time.Second)
	}()
	err = receiveResult(t, result)
	var phase *router.ManagedError
	require.ErrorAs(t, err, &phase)
	require.Equal(t, "startup", phase.Phase)
}

func TestServePreCancellation(t *testing.T) {
	server := testApp(t).server.(router.ManagedServer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, serve(ctx, server, "127.0.0.1:0", time.Second), context.Canceled)
	// The pre-cancelled call must not reserve a managed lifetime.
	run, cancel, done := startObserved(t, server, time.Second, nil)
	await(t, run.Serving())
	cancel()
	require.NoError(t, receiveResult(t, done))
}

func TestServeCancellationDuringStartup(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	run, cancel, done := startObserved(t, testApp(t).server.(router.ManagedServer), time.Second, func(o *router.ManagedOptions) {
		o.OnBound = func(context.Context, string) error { close(entered); <-release; return nil }
	})
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	await(t, entered)
	cancel()
	await(t, run.Stopping())
	select {
	case err := <-done:
		t.Fatalf("returned before startup joined: %v", err)
	default:
	}
	once.Do(func() { close(release) })
	require.NoError(t, receiveResult(t, done))
	await(t, run.Done())
	select {
	case <-run.Serving():
		t.Fatal("cancelled startup admitted requests")
	default:
	}
}

func TestServeOrdinaryRequestDrain(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "completion"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			server := testApp(t).server
			entered, release, cancelled := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			server.Router().Get("/held", func(c router.Context) error {
				close(entered)
				if deadline {
					<-c.Context().Done()
					close(cancelled)
					<-release
					return nil
				}
				select {
				case <-release:
					return c.SendString("drained")
				case <-c.Context().Done():
					close(cancelled)
					return c.Context().Err()
				}
			})
			budget := 2 * time.Second
			if deadline {
				budget = 150 * time.Millisecond
			}
			run, cancel, done := startObserved(t, server.(router.ManagedServer), budget, nil)
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			await(t, run.Serving())
			client := &http.Client{Timeout: 4 * time.Second}
			response := make(chan error, 1)
			go func() {
				resp, err := client.Get("http://" + run.Address() + "/held")
				if err == nil {
					defer resp.Body.Close()
					var body []byte
					body, err = io.ReadAll(resp.Body)
					if !deadline && (resp.StatusCode != 200 || string(body) != "drained") {
						err = errors.New("lost drained response")
					}
				}
				response <- err
			}()
			await(t, entered)
			cancel()
			await(t, run.Stopping())
			if deadline {
				err := receiveResult(t, done)
				require.ErrorIs(t, err, context.DeadlineExceeded)
				await(t, cancelled)
				once.Do(func() { close(release) })
				wait, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				require.ErrorIs(t, run.Wait(wait), router.ErrManagedDrainTimeout)
				_ = receiveResult(t, response)
			} else {
				select {
				case <-cancelled:
					t.Fatal("signal cancelled admitted request before drain")
				case err := <-done:
					t.Fatalf("returned before request drain: %v", err)
				default:
				}
				once.Do(func() { close(release) })
				require.NoError(t, receiveResult(t, response))
				require.NoError(t, receiveResult(t, done))
				await(t, run.Done())
			}
		})
	}
}

func TestServeShutdownFailure(t *testing.T) {
	failure := errors.New("shutdown failure")
	run, cancel, done := startObserved(t, testApp(t).server.(router.ManagedServer), time.Second, func(o *router.ManagedOptions) { o.OnShutdown = func(context.Context) error { return failure } })
	await(t, run.Serving())
	cancel()
	require.ErrorIs(t, receiveResult(t, done), failure)
}

func testApp(t *testing.T) *App {
	t.Helper()
	app, err := NewApp()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, app.Close()) })
	return app
}

func TestAppContextAndResourceLifetime(t *testing.T) {
	var absent *App
	require.Error(t, absent.ServeContext(t.Context(), "127.0.0.1:0"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, run(ctx), context.Canceled)
	app := testApp(t)
	run, cancel, done := startObserved(t, app.server.(router.ManagedServer), time.Second, nil)
	await(t, run.Serving())
	require.NoError(t, app.db.PingContext(t.Context()))
	cancel()
	require.NoError(t, receiveResult(t, done))
	await(t, run.Done())
	require.NoError(t, app.db.PingContext(t.Context()), "server coordinator must not close caller-owned database")
}
