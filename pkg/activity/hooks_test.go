package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeEventDefaults(t *testing.T) {
	meta := map[string]any{"k": "v"}
	recipients := []string{"a", "b"}
	event := Event{
		Verb:           " create ",
		ActorID:        " actor ",
		UserID:         " user ",
		TenantID:       " tenant ",
		ObjectType:     " thing ",
		ObjectID:       " 123 ",
		Channel:        " ",
		DefinitionCode: " def ",
		Recipients:     recipients,
		Metadata:       meta,
	}

	normalized := NormalizeEvent(event)

	require.Equal(t, "create", normalized.Verb)
	require.Equal(t, "actor", normalized.ActorID)
	require.Equal(t, "user", normalized.UserID)
	require.Equal(t, "tenant", normalized.TenantID)
	require.Equal(t, "thing", normalized.ObjectType)
	require.Equal(t, "123", normalized.ObjectID)
	require.Empty(t, normalized.Channel)
	require.Equal(t, "def", normalized.DefinitionCode)
	require.Equal(t, recipients, normalized.Recipients)
	normalized.Recipients[0] = "changed"
	require.Equal(t, []string{"a", "b"}, recipients)

	require.Equal(t, meta, normalized.Metadata)
	normalized.Metadata["k"] = "other"
	require.Equal(t, "v", meta["k"])

	require.False(t, normalized.OccurredAt.IsZero())
}

func TestHooksNotifyShortCircuitsMissingFields(t *testing.T) {
	var called bool
	hooks := Hooks{HookFunc(func(context.Context, Event) error {
		called = true
		return nil
	})}

	require.NoError(t, hooks.Notify(context.Background(), Event{}))
	require.False(t, called)
}

func TestHooksNotifyJoinsErrors(t *testing.T) {
	fooErr := errors.New("foo")
	barErr := errors.New("bar")
	hooks := Hooks{
		HookFunc(func(context.Context, Event) error { return fooErr }),
		HookFunc(func(context.Context, Event) error { return nil }),
		HookFunc(func(context.Context, Event) error { return barErr }),
	}

	err := hooks.Notify(context.Background(), Event{
		Verb:       "crud.test.create",
		ObjectType: "test",
		ObjectID:   "id",
		OccurredAt: time.Now(),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, fooErr)
	require.ErrorIs(t, err, barErr)
}
