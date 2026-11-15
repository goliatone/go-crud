package helpers

import (
	"context"
	"errors"
	"sync"
)

// ErrSubscriptionScopeClosed rejects work after its transport owner starts closing.
var ErrSubscriptionScopeClosed = errors.New("subscription scope is closed")

type subscriptionScopeKey struct{}

// SubscriptionScope owns generated subscription forwarders for one transport
// lifetime. It does not own an event bus or unrelated application workers.
type SubscriptionScope struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	active int
	closed bool
	done   chan struct{}
}

// WithSubscriptionScope creates a child context for generated subscriptions.
// Pass it to the GraphQL executor and call Close before releasing transport
// resources. Create a fresh scope for each connection or independent lifetime.
func WithSubscriptionScope(parent context.Context) (context.Context, *SubscriptionScope) {
	ctx, cancel := context.WithCancel(parent)
	scope := &SubscriptionScope{cancel: cancel, done: make(chan struct{})}
	return context.WithValue(ctx, subscriptionScopeKey{}, scope), scope
}

// BeginSubscription registers a forwarder before subscribing to its event bus.
// Generated code calls the returned idempotent release function on every exit.
// Without a scope, cancellation still applies but there is no owner to join it.
func BeginSubscription(ctx context.Context) (func(), error) {
	scope, _ := ctx.Value(subscriptionScopeKey{}).(*SubscriptionScope)
	if scope == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return func() {}, nil
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if scope.closed {
		return nil, ErrSubscriptionScopeClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scope.active++
	var once sync.Once
	return func() {
		once.Do(func() {
			scope.mu.Lock()
			defer scope.mu.Unlock()
			scope.active--
			if scope.closed && scope.active == 0 {
				close(scope.done)
			}
		})
	}, nil
}

// Close seals admission, cancels the scope and waits for all registered forwarders.
// A caller timeout only bounds that wait; Close can be retried. Do not call Close
// from a registered worker. Context cancellation by itself is not a join.
func (s *SubscriptionScope) Close(ctx context.Context) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		if s.active == 0 {
			close(s.done)
		}
	}
	s.mu.Unlock()
	s.cancel()
	select {
	case <-s.done:
		return nil
	default:
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Done closes after Close has sealed the scope and all registered work exits.
func (s *SubscriptionScope) Done() <-chan struct{} { return s.done }
