package crud

import (
	"context"
	"strings"

	"github.com/goliatone/go-crud/pkg/activity"
)

type requestContextKey string

const (
	ctxKeyActor       requestContextKey = "crud.actor"
	ctxKeyRequestID   requestContextKey = "crud.request_id"
	ctxKeyCorrelation requestContextKey = "crud.correlation_id"
	ctxKeyScope       requestContextKey = "crud.scope_filter"
	ctxKeyHookMeta    requestContextKey = "crud.hook_metadata"
	ctxKeyActivity    requestContextKey = "crud.activity_emitter"
	ctxKeyNotify      requestContextKey = "crud.notification_emitter"
)

// ContextWithActor stores the provided actor metadata on the standard context.
func ContextWithActor(ctx context.Context, actor ActorContext) context.Context {
	if ctx == nil || actor.IsZero() {
		return ctx
	}
	clone := actor.Clone()
	return context.WithValue(ctx, ctxKeyActor, clone)
}

// ActorFromContext retrieves the ActorContext previously stored on the context.
func ActorFromContext(ctx context.Context) ActorContext {
	if ctx == nil {
		return ActorContext{}
	}
	if actor, ok := ctx.Value(ctxKeyActor).(ActorContext); ok {
		return actor
	}
	return ActorContext{}
}

// ContextWithRequestID stores the current request identifier on the context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil || strings.TrimSpace(requestID) == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRequestID, strings.TrimSpace(requestID))
}

// RequestIDFromContext returns the request identifier stored in the context.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return strings.TrimSpace(requestID)
	}
	return ""
}

// ContextWithCorrelationID stores the correlation ID on the context.
func ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if ctx == nil || strings.TrimSpace(correlationID) == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyCorrelation, strings.TrimSpace(correlationID))
}

// CorrelationIDFromContext extracts the stored correlation ID.
func CorrelationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if correlationID, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return strings.TrimSpace(correlationID)
	}
	return ""
}

// ContextWithScope stores the guard scope filter on the context.
func ContextWithScope(ctx context.Context, scope ScopeFilter) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyScope, scope.clone())
}

// ScopeFromContext extracts the guard scope filter from the context.
func ScopeFromContext(ctx context.Context) ScopeFilter {
	if ctx == nil {
		return ScopeFilter{}
	}
	if scope, ok := ctx.Value(ctxKeyScope).(ScopeFilter); ok {
		return scope
	}
	return ScopeFilter{}
}

type userContextSetter interface {
	SetUserContext(context.Context)
}

type headerProvider interface {
	Header(string) string
}

func attachActorToRequestContext(ctx Context, actor ActorContext) {
	if ctx == nil || actor.IsZero() {
		return
	}
	base := ctx.UserContext()
	updated := ContextWithActor(base, actor)
	if setter, ok := ctx.(userContextSetter); ok && updated != nil {
		setter.SetUserContext(updated)
	}
}

func attachScopeToRequestContext(ctx Context, scope ScopeFilter) {
	if ctx == nil || (!scope.HasFilters() && !scope.Bypass && len(scope.Labels) == 0 && len(scope.Raw) == 0) {
		return
	}
	base := ctx.UserContext()
	updated := ContextWithScope(base, scope)
	if setter, ok := ctx.(userContextSetter); ok && updated != nil {
		setter.SetUserContext(updated)
	}
}

func attachIdentifiersToRequestContext(ctx Context, requestID, correlationID string) {
	if ctx == nil {
		return
	}

	base := ctx.UserContext()
	if requestID != "" {
		base = ContextWithRequestID(base, requestID)
	}
	if correlationID != "" {
		base = ContextWithCorrelationID(base, correlationID)
	}

	if setter, ok := ctx.(userContextSetter); ok && base != nil {
		setter.SetUserContext(base)
	}
}

func resolveRequestID(ctx Context) string {
	if ctx == nil {
		return ""
	}

	if reqID := RequestIDFromContext(ctx.UserContext()); reqID != "" {
		return reqID
	}

	if provider, ok := ctx.(headerProvider); ok {
		if header := strings.TrimSpace(provider.Header("X-Request-ID")); header != "" {
			return header
		}
		if header := strings.TrimSpace(provider.Header("Request-ID")); header != "" {
			return header
		}
	}
	return ""
}

func resolveCorrelationID(ctx Context) string {
	if ctx == nil {
		return ""
	}

	if corrID := CorrelationIDFromContext(ctx.UserContext()); corrID != "" {
		return corrID
	}

	if provider, ok := ctx.(headerProvider); ok {
		if header := strings.TrimSpace(provider.Header("X-Correlation-ID")); header != "" {
			return header
		}
		if header := strings.TrimSpace(provider.Header("Correlation-ID")); header != "" {
			return header
		}
	}
	return ""
}

// ContextWithHookMetadata attaches hook metadata to the context.
func ContextWithHookMetadata(ctx context.Context, meta HookMetadata) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyHookMeta, meta)
}

// HookMetadataFromContext retrieves hook metadata stored on the context.
func HookMetadataFromContext(ctx context.Context) HookMetadata {
	if ctx == nil {
		return HookMetadata{}
	}
	if meta, ok := ctx.Value(ctxKeyHookMeta).(HookMetadata); ok {
		return meta
	}
	return HookMetadata{}
}

// ContextWithActivityEmitter stores the activity emitter used by hooks.
func ContextWithActivityEmitter(ctx context.Context, emitter *activity.Emitter) context.Context {
	if ctx == nil || emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyActivity, emitter)
}

// ActivityEmitterFromContext returns the hook activity emitter stored on the context.
func ActivityEmitterFromContext(ctx context.Context) *activity.Emitter {
	if ctx == nil {
		return nil
	}
	emitter, _ := ctx.Value(ctxKeyActivity).(*activity.Emitter)
	return emitter
}

// ContextWithNotificationEmitter stores the notification emitter for hooks.
func ContextWithNotificationEmitter(ctx context.Context, emitter NotificationEmitter) context.Context {
	if ctx == nil || emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyNotify, emitter)
}

// NotificationEmitterFromContext extracts the notification emitter from the context.
func NotificationEmitterFromContext(ctx context.Context) NotificationEmitter {
	if ctx == nil {
		return nil
	}
	if emitter, ok := ctx.Value(ctxKeyNotify).(NotificationEmitter); ok {
		return emitter
	}
	return nil
}
