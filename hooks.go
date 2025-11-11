package crud

// HookMetadata carries operational attributes for lifecycle hooks.
type HookMetadata struct {
	Operation CrudOperation
	Resource  string
	RouteName string
	Method    string
	Path      string
}

// HookContext bundles the request context with hook metadata.
type HookContext struct {
	Context       Context
	Metadata      HookMetadata
	Actor         ActorContext
	Scope         ScopeFilter
	RequestID     string
	CorrelationID string

	activityEmitter     ActivityEmitter
	notificationEmitter NotificationEmitter
}

// HookFunc represents a lifecycle hook for a single record.
type HookFunc[T any] func(HookContext, T) error

// HookBatchFunc represents a lifecycle hook for multiple records.
type HookBatchFunc[T any] func(HookContext, []T) error

// LifecycleHooks groups all supported CRUD lifecycle hooks.
type LifecycleHooks[T any] struct {
	BeforeCreate      HookFunc[T]
	AfterCreate       HookFunc[T]
	BeforeCreateBatch HookBatchFunc[T]
	AfterCreateBatch  HookBatchFunc[T]

	BeforeUpdate      HookFunc[T]
	AfterUpdate       HookFunc[T]
	BeforeUpdateBatch HookBatchFunc[T]
	AfterUpdateBatch  HookBatchFunc[T]

	BeforeDelete      HookFunc[T]
	AfterDelete       HookFunc[T]
	BeforeDeleteBatch HookBatchFunc[T]
	AfterDeleteBatch  HookBatchFunc[T]
}

// HasActivityEmitter reports whether the controller configured an ActivityEmitter.
func (h HookContext) HasActivityEmitter() bool {
	return h.activityEmitter != nil
}

// ActivityEmitter returns the configured ActivityEmitter (if any).
func (h HookContext) ActivityEmitter() ActivityEmitter {
	return h.activityEmitter
}

// HasNotificationEmitter reports whether the controller configured a NotificationEmitter.
func (h HookContext) HasNotificationEmitter() bool {
	return h.notificationEmitter != nil
}

// NotificationEmitter returns the configured NotificationEmitter (if any).
func (h HookContext) NotificationEmitter() NotificationEmitter {
	return h.notificationEmitter
}

// HookFromContext adapts a legacy hook that only expected crud.Context into a
// HookFunc that receives the enriched HookContext. Nil hooks return nil.
func HookFromContext[T any](hook func(Context, T) error) HookFunc[T] {
	if hook == nil {
		return nil
	}
	return func(hctx HookContext, record T) error {
		return hook(hctx.Context, record)
	}
}

// HookBatchFromContext adapts a legacy batch hook (Context + []T) into the new
// HookBatchFunc form that receives HookContext metadata.
func HookBatchFromContext[T any](hook func(Context, []T) error) HookBatchFunc[T] {
	if hook == nil {
		return nil
	}
	return func(hctx HookContext, records []T) error {
		return hook(hctx.Context, records)
	}
}
