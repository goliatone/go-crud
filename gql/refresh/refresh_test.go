package refresh

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/gql/internal/generator"
)

func TestRegisterSchemaRefresh_TriggersGenerator(t *testing.T) {
	var listener crud.SchemaListener
	done := make(chan struct{}, 1)

	RegisterSchemaRefresh(context.Background(), Options{
		RegisterListener: func(l crud.SchemaListener) { listener = l },
		Generator: func(ctx context.Context, opts generator.Options) (generator.Result, error) {
			done <- struct{}{}
			return generator.Result{}, nil
		},
	})

	require.NotNil(t, listener)
	listener(crud.SchemaEntry{Resource: "article"})

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected generator to run")
	}
}

func TestRegisterSchemaRefresh_Debounces(t *testing.T) {
	var listener crud.SchemaListener
	var calls int32
	done := make(chan struct{}, 1)

	RegisterSchemaRefresh(context.Background(), Options{
		Debounce:         20 * time.Millisecond,
		RegisterListener: func(l crud.SchemaListener) { listener = l },
		Generator: func(ctx context.Context, opts generator.Options) (generator.Result, error) {
			if atomic.AddInt32(&calls, 1) == 1 {
				done <- struct{}{}
			}
			return generator.Result{}, nil
		},
	})

	require.NotNil(t, listener)
	listener(crud.SchemaEntry{Resource: "article"})
	listener(crud.SchemaEntry{Resource: "article"})
	listener(crud.SchemaEntry{Resource: "article"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected generator to run")
	}

	time.Sleep(30 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestRegisterSchemaRefresh_RunsPending(t *testing.T) {
	var listener crud.SchemaListener
	var calls int32
	started := make(chan struct{})
	release := make(chan struct{})

	RegisterSchemaRefresh(context.Background(), Options{
		RegisterListener: func(l crud.SchemaListener) { listener = l },
		Generator: func(ctx context.Context, opts generator.Options) (generator.Result, error) {
			if atomic.AddInt32(&calls, 1) == 1 {
				close(started)
				<-release
			}
			return generator.Result{}, nil
		},
	})

	require.NotNil(t, listener)
	listener(crud.SchemaEntry{Resource: "article"})

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected first generator run to start")
	}

	listener(crud.SchemaEntry{Resource: "article"})
	close(release)

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&calls) == 2
	}, 500*time.Millisecond, 10*time.Millisecond)
}
