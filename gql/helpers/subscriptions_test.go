package helpers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionScopeSealCancelAndJoin(t *testing.T) {
	type key struct{}
	ctx, scope := WithSubscriptionScope(context.WithValue(t.Context(), key{}, "actor"))
	require.Equal(t, "actor", ctx.Value(key{}))
	release, err := BeginSubscription(ctx)
	require.NoError(t, err)
	wait, stop := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer stop()
	require.ErrorIs(t, scope.Close(wait), context.DeadlineExceeded)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	_, err = BeginSubscription(ctx)
	require.ErrorIs(t, err, ErrSubscriptionScopeClosed)
	select {
	case <-scope.Done():
		t.Fatal("returned before admitted worker exited")
	default:
	}
	release()
	release()
	require.NoError(t, scope.Close(t.Context()))
	<-scope.Done()
}

func TestSubscriptionScopeIndependentAndUnscoped(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	first, a := WithSubscriptionScope(parent)
	second, b := WithSubscriptionScope(parent)
	release, err := BeginSubscription(second)
	require.NoError(t, err)
	require.NoError(t, a.Close(t.Context()))
	require.ErrorIs(t, first.Err(), context.Canceled)
	require.NoError(t, second.Err())
	cancel()
	_, err = BeginSubscription(second)
	require.ErrorIs(t, err, context.Canceled)
	release()
	require.NoError(t, b.Close(t.Context()))
	release, err = BeginSubscription(t.Context())
	require.NoError(t, err)
	release()
	_, err = BeginSubscription(parent)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSubscriptionScopeConcurrentAdmissionAndClose(t *testing.T) {
	for range 30 {
		ctx, scope := WithSubscriptionScope(t.Context())
		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				<-start
				release, err := BeginSubscription(ctx)
				if err == nil {
					<-ctx.Done()
					release()
				} else if !errors.Is(err, ErrSubscriptionScopeClosed) && !errors.Is(err, context.Canceled) {
					t.Errorf("unexpected admission: %v", err)
				}
			})
		}
		close(start)
		wait, cancel := context.WithTimeout(t.Context(), time.Second)
		require.NoError(t, scope.Close(wait))
		cancel()
		wg.Wait()
	}
}
