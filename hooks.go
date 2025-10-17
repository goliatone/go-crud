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
	Context  Context
	Metadata HookMetadata
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
