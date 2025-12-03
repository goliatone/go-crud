package usersink

import (
	"context"
	"testing"
	"time"

	"github.com/goliatone/go-crud/pkg/activity"
	usertypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type captureSink struct {
	records []usertypes.ActivityRecord
	err     error
}

func (s *captureSink) Log(_ context.Context, record usertypes.ActivityRecord) error {
	s.records = append(s.records, record)
	return s.err
}

func TestHookMapsEventToActivityRecord(t *testing.T) {
	sink := &captureSink{}
	hook := Hook{Sink: sink}

	actorID := uuid.New()
	userID := uuid.New()
	tenantID := uuid.New()
	occurred := time.Now().Add(-time.Minute).UTC()
	meta := map[string]any{"foo": "bar"}

	err := hook.Notify(context.Background(), activity.Event{
		Verb:           "crud.user.create",
		ActorID:        actorID.String(),
		UserID:         userID.String(),
		TenantID:       tenantID.String(),
		ObjectType:     "user",
		ObjectID:       "123",
		Channel:        "crud",
		DefinitionCode: "def",
		Recipients:     []string{"a@example.com"},
		Metadata:       meta,
		OccurredAt:     occurred,
	})
	require.NoError(t, err)
	require.Len(t, sink.records, 1)

	record := sink.records[0]
	require.Equal(t, actorID, record.ActorID)
	require.Equal(t, userID, record.UserID)
	require.Equal(t, tenantID, record.TenantID)
	require.Equal(t, "crud.user.create", record.Verb)
	require.Equal(t, "user", record.ObjectType)
	require.Equal(t, "123", record.ObjectID)
	require.Equal(t, "crud", record.Channel)
	require.Equal(t, occurred, record.OccurredAt)
	require.Equal(t, map[string]any{
		"foo":             "bar",
		"definition_code": "def",
		"recipients":      []string{"a@example.com"},
	}, record.Data)
	record.Data["foo"] = "baz"
	require.Equal(t, "bar", meta["foo"])
}

func TestHookDefaultsOccurredAt(t *testing.T) {
	sink := &captureSink{}
	hook := Hook{Sink: sink}

	err := hook.Notify(context.Background(), activity.Event{
		Verb:       "crud.user.create",
		ObjectType: "user",
		ObjectID:   "123",
	})
	require.NoError(t, err)
	require.Len(t, sink.records, 1)
	require.False(t, sink.records[0].OccurredAt.IsZero())
}

func TestHookNoSinkNoop(t *testing.T) {
	hook := Hook{}
	require.NoError(t, hook.Notify(context.Background(), activity.Event{
		Verb:       "crud.user.create",
		ObjectType: "user",
		ObjectID:   "123",
	}))
}
