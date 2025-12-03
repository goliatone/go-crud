package usersink

import (
	"context"
	"strings"
	"time"

	"github.com/goliatone/go-crud/pkg/activity"
	usertypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

// Hook adapts activity events to a go-users ActivitySink.
type Hook struct {
	Sink usertypes.ActivitySink
}

// Notify maps the event into an ActivityRecord and forwards it to the sink.
func (h Hook) Notify(ctx context.Context, event activity.Event) error {
	if h.Sink == nil {
		return nil
	}

	normalized := activity.NormalizeEvent(event)
	if normalized.Verb == "" || normalized.ObjectType == "" || normalized.ObjectID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	record := usertypes.ActivityRecord{
		ActorID:    parseUUID(normalized.ActorID),
		UserID:     parseUUID(normalized.UserID),
		TenantID:   parseUUID(normalized.TenantID),
		Verb:       normalized.Verb,
		ObjectType: normalized.ObjectType,
		ObjectID:   normalized.ObjectID,
		Channel:    normalized.Channel,
		Data:       cloneMap(normalized.Metadata),
		OccurredAt: normalized.OccurredAt,
	}
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now()
	}
	if normalized.DefinitionCode != "" {
		if record.Data == nil {
			record.Data = map[string]any{}
		}
		record.Data["definition_code"] = normalized.DefinitionCode
	}
	if len(normalized.Recipients) > 0 {
		if record.Data == nil {
			record.Data = map[string]any{}
		}
		record.Data["recipients"] = append([]string{}, normalized.Recipients...)
	}

	return h.Sink.Log(ctx, record)
}

func parseUUID(input string) uuid.UUID {
	value := strings.TrimSpace(input)
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
