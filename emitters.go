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

// ActivityEmitter receives normalized activity events for auditing/analytics layers.
type ActivityEmitter interface {
	EmitActivity(ctx context.Context, event ActivityEvent) error
}

// ActivityEvent captures the resource metadata, actor/scope, and affected records.
type ActivityEvent struct {
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
	Metadata      map[string]any
	Error         error
}

// ActivityEventOption configures optional fields before dispatching the event.
type ActivityEventOption func(*ActivityEvent)

// WithActivityEventMetadata merges the provided metadata map into the event.
func WithActivityEventMetadata(metadata map[string]any) ActivityEventOption {
	return func(evt *ActivityEvent) {
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

// WithActivityEventMetaValue sets or overwrites a single metadata entry.
func WithActivityEventMetaValue(key string, value any) ActivityEventOption {
	return func(evt *ActivityEvent) {
		if key == "" {
			return
		}
		if evt.Metadata == nil {
			evt.Metadata = make(map[string]any, 1)
		}
		evt.Metadata[key] = value
	}
}

// WithActivityEventError attaches an error to the event (useful for failure tracking).
func WithActivityEventError(err error) ActivityEventOption {
	return func(evt *ActivityEvent) {
		evt.Error = err
	}
}

// EmitActivity emits an activity event for a single record.
func EmitActivity[T any](hctx HookContext, phase ActivityPhase, record T, opts ...ActivityEventOption) error {
	if !hctx.HasActivityEmitter() || isNil(record) {
		return nil
	}
	event := newActivityEventFromHook(hctx, phase)
	event.Records = []any{record}
	for _, opt := range opts {
		opt(&event)
	}
	return hctx.activityEmitter.EmitActivity(hookUserContext(hctx), event)
}

// EmitActivityBatch emits an activity event for a batch of records.
func EmitActivityBatch[T any](hctx HookContext, phase ActivityPhase, records []T, opts ...ActivityEventOption) error {
	if !hctx.HasActivityEmitter() || len(records) == 0 {
		return nil
	}
	event := newActivityEventFromHook(hctx, phase)
	event.Records = toAnySlice(records)
	for _, opt := range opts {
		opt(&event)
	}
	return hctx.activityEmitter.EmitActivity(hookUserContext(hctx), event)
}

// NotificationEmitter sends user-facing notifications derived from lifecycle events.
type NotificationEmitter interface {
	SendNotification(ctx context.Context, event NotificationEvent) error
}

// NotificationEvent mirrors ActivityEvent while adding notification-specific fields.
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

func newActivityEventFromHook(hctx HookContext, phase ActivityPhase) ActivityEvent {
	return ActivityEvent{
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

func hookUserContext(hctx HookContext) context.Context {
	if hctx.Context != nil {
		if base := hctx.Context.UserContext(); base != nil {
			return base
		}
	}
	return context.Background()
}
