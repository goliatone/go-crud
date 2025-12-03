package activity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Event describes an activity occurrence that can be fanned out to hooks.
// IDs are stringly-typed to avoid coupling call sites to specific UUID types.
type Event struct {
	Verb           string
	ActorID        string
	UserID         string
	TenantID       string
	ObjectType     string
	ObjectID       string
	Channel        string
	DefinitionCode string
	Recipients     []string
	Metadata       map[string]any
	OccurredAt     time.Time
}

// ActivityHook receives normalized activity events.
type ActivityHook interface {
	Notify(ctx context.Context, event Event) error
}

// HookFunc allows plain functions to satisfy ActivityHook.
type HookFunc func(ctx context.Context, event Event) error

// Notify dispatches to the underlying function.
func (fn HookFunc) Notify(ctx context.Context, event Event) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, event)
}

// Hooks fans out events to zero or more hooks.
type Hooks []ActivityHook

// Enabled reports whether there are any hooks to notify.
func (h Hooks) Enabled() bool {
	return len(h) > 0
}

// Notify forwards the event to all hooks, returning a joined error if any fail.
// It normalizes the event, short-circuiting when required fields are missing.
func (h Hooks) Notify(ctx context.Context, event Event) error {
	if len(h) == 0 {
		return nil
	}

	normalized := NormalizeEvent(event)
	if normalized.Verb == "" || normalized.ObjectType == "" || normalized.ObjectID == "" {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var errs []error
	for _, hook := range h {
		if hook == nil {
			continue
		}
		if err := hook.Notify(ctx, normalized); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// NormalizeEvent trims whitespace, clones metadata, and ensures a timestamp is present.
func NormalizeEvent(event Event) Event {
	normalized := event
	normalized.Verb = strings.TrimSpace(event.Verb)
	normalized.ActorID = strings.TrimSpace(event.ActorID)
	normalized.UserID = strings.TrimSpace(event.UserID)
	normalized.TenantID = strings.TrimSpace(event.TenantID)
	normalized.ObjectType = strings.TrimSpace(event.ObjectType)
	normalized.ObjectID = strings.TrimSpace(event.ObjectID)
	normalized.Channel = strings.TrimSpace(event.Channel)
	normalized.DefinitionCode = strings.TrimSpace(event.DefinitionCode)
	normalized.Metadata = cloneMap(event.Metadata)
	if len(event.Recipients) > 0 {
		normalized.Recipients = append([]string{}, event.Recipients...)
	} else {
		normalized.Recipients = nil
	}
	if normalized.OccurredAt.IsZero() {
		normalized.OccurredAt = time.Now()
	}
	return normalized
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
