package resolvers

import (
	"context"
	"errors"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
	"github.com/goliatone/go-crud/gql/helpers"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type heldDeliveryBus struct {
	input chan EventMessage
	calls int
}

func (b *heldDeliveryBus) Publish(context.Context, string, any) error { return nil }
func (b *heldDeliveryBus) Subscribe(context.Context, string) (<-chan EventMessage, error) {
	b.calls++
	return b.input, nil
}

func TestGeneratedSubscriptionOwnerJoinsBlockedDelivery(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		t.Run(map[bool]string{true: "pointer", false: "value"}[pointer], func(t *testing.T) {
			resolver, base, cleanup := setupResolver(t)
			defer cleanup()
			ctx, scope := helpers.WithSubscriptionScope(base)
			bus := &heldDeliveryBus{input: make(chan EventMessage)}
			resolver.Events = bus
			out, err := resolver.TagCreated(ctx)
			require.NoError(t, err)
			var payload any = model.Tag{Name: "held"}
			if pointer {
				payload = &model.Tag{Name: "held"}
			}
			select {
			case bus.input <- EventMessage{Payload: payload}:
			case <-time.After(time.Second):
				t.Fatal("event not accepted")
			}
			wait, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			require.NoError(t, scope.Close(wait))
			select {
			case _, ok := <-out:
				require.False(t, ok)
			default:
				t.Fatal("owner returned before output closed")
			}
			_, err = resolver.TagCreated(ctx)
			require.ErrorIs(t, err, helpers.ErrSubscriptionScopeClosed)
			require.Equal(t, 1, bus.calls)
		})
	}
}
func TestGeneratedSubscriptionAuthAndScopeBeforeBus(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()
	bus := &heldDeliveryBus{input: make(chan EventMessage)}
	resolver.Events = bus
	out, err := resolver.TagCreated(t.Context())
	require.ErrorContains(t, err, "unauthorized")
	require.Nil(t, out)
	require.Zero(t, bus.calls)
	resolver.ScopeGuard = func(_ context.Context, entity, action string) error {
		require.Equal(t, "Tag", entity)
		require.Equal(t, "subscribe", action)
		return errors.New("denied")
	}
	out, err = resolver.TagCreated(ctx)
	require.ErrorContains(t, err, "denied")
	require.Nil(t, out)
	require.Zero(t, bus.calls)
}
func TestGeneratedM2MFilterCountAndPage(t *testing.T) {
	resolver, ctx, cleanup := setupResolver(t)
	defer cleanup()
	limit, offset := 1, 1
	conn, err := resolver.ListAuthor(ctx, &model.PaginationInput{Limit: &limit, Offset: &offset}, []*model.OrderByInput{{Field: "fullName"}}, []*model.FilterInput{{Field: "tags.name", Operator: model.FilterOperatorEQ, Value: "Science Fiction"}})
	require.NoError(t, err)
	require.Len(t, conn.Edges, 1)
	require.Equal(t, "Miles Dorsey", conn.Edges[0].Node.FullName)
	require.Equal(t, 2, conn.PageInfo.Total)
	require.True(t, conn.PageInfo.HasPreviousPage)
	require.False(t, conn.PageInfo.HasNextPage)
}
