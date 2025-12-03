package crud

import (
	"context"
	"strings"
)

// ActivityPhase indicates the lifecycle moment that triggered the emitter.
type ActivityPhase string

const (
	ActivityPhaseBefore ActivityPhase = "before"
	ActivityPhaseAfter  ActivityPhase = "after"
	ActivityPhaseError  ActivityPhase = "error"
)

// NotificationEmitter sends user-facing notifications derived from lifecycle events.
type NotificationEmitter interface {
	SendNotification(ctx context.Context, event NotificationEvent) error
}

// NotificationEvent describes a notification occurrence.
type NotificationEvent struct {
	Operation     CrudOperation
	Phase         ActivityPhase
	Resource      string
	RouteName     string
	Method        string
	Path          string
	Actor         ActorContext
	Scope         ScopeFilter
	RequestID     string
	CorrelationID string
	Records       []any
	Channel       string
	Template      string
	Recipients    []string
	Metadata      map[string]any
}

// NotificationEventOption configures optional notification fields.
type NotificationEventOption func(*NotificationEvent)

// WithNotificationChannel sets the outbound channel (e.g., email, webhook).
func WithNotificationChannel(channel string) NotificationEventOption {
	return func(evt *NotificationEvent) {
		evt.Channel = strings.TrimSpace(channel)
	}
}

// WithNotificationTemplate stores the downstream template identifier.
func WithNotificationTemplate(template string) NotificationEventOption {
	return func(evt *NotificationEvent) {
		evt.Template = strings.TrimSpace(template)
	}
}

// WithNotificationRecipients declares the intended recipients (emails, IDs, etc.).
func WithNotificationRecipients(recipients ...string) NotificationEventOption {
	return func(evt *NotificationEvent) {
		filtered := make([]string, 0, len(recipients))
		for _, recipient := range recipients {
			recipient = strings.TrimSpace(recipient)
			if recipient != "" {
				filtered = append(filtered, recipient)
			}
		}
		if len(filtered) == 0 {
			return
		}
		evt.Recipients = append(evt.Recipients, filtered...)
	}
}

// WithNotificationMetadata merges arbitrary metadata into the event.
func WithNotificationMetadata(metadata map[string]any) NotificationEventOption {
	return func(evt *NotificationEvent) {
		if len(metadata) == 0 {
			return
		}
		if evt.Metadata == nil {
			evt.Metadata = make(map[string]any, len(metadata))
		}
		for k, v := range metadata {
			evt.Metadata[k] = v
		}
	}
}

// SendNotification emits a notification event for a single record.
func SendNotification[T any](hctx HookContext, phase ActivityPhase, record T, opts ...NotificationEventOption) error {
	if !hctx.HasNotificationEmitter() || isNil(record) {
		return nil
	}
	event := newNotificationEventFromHook(hctx, phase)
	event.Records = []any{record}
	for _, opt := range opts {
		opt(&event)
	}
	return hctx.notificationEmitter.SendNotification(hookUserContext(hctx), event)
}

// SendNotificationBatch emits a notification event for multiple records.
func SendNotificationBatch[T any](hctx HookContext, phase ActivityPhase, records []T, opts ...NotificationEventOption) error {
	if !hctx.HasNotificationEmitter() || len(records) == 0 {
		return nil
	}
	event := newNotificationEventFromHook(hctx, phase)
	event.Records = toAnySlice(records)
	for _, opt := range opts {
		opt(&event)
	}
	return hctx.notificationEmitter.SendNotification(hookUserContext(hctx), event)
}

func newNotificationEventFromHook(hctx HookContext, phase ActivityPhase) NotificationEvent {
	return NotificationEvent{
		Operation:     hctx.Metadata.Operation,
		Phase:         phase,
		Resource:      hctx.Metadata.Resource,
		RouteName:     hctx.Metadata.RouteName,
		Method:        hctx.Metadata.Method,
		Path:          hctx.Metadata.Path,
		Actor:         hctx.Actor.Clone(),
		Scope:         hctx.Scope.clone(),
		RequestID:     hctx.RequestID,
		CorrelationID: hctx.CorrelationID,
	}
}

func toAnySlice[T any](records []T) []any {
	if len(records) == 0 {
		return nil
	}
	result := make([]any, len(records))
	for i, record := range records {
		result[i] = record
	}
	return result
}
