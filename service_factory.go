package crud

import (
	"context"
	"fmt"
	"reflect"

	"github.com/goliatone/go-crud/pkg/activity"
	repository "github.com/goliatone/go-repository-bun"
)

// ValidatorFunc validates a single record before persistence/update.
type ValidatorFunc[T any] func(ctx Context, record T) error

// VirtualFieldProcessor is implemented by VirtualFieldHandler and test doubles.
type VirtualFieldProcessor[T any] interface {
	BeforeSave(HookContext, T) error
	AfterLoad(HookContext, T) error
	AfterLoadBatch(HookContext, []T) error
}

// ServiceConfig holds all optional business logic layers applied by NewService.
// ResourceName/ResourceType help field policies resolve friendly names when
// reflection on the model type is not sufficient or needs overriding.
type ServiceConfig[T any] struct {
	Repository          repository.Repository[T]
	Hooks               LifecycleHooks[T]
	ScopeGuard          ScopeGuardFunc[T]
	FieldPolicy         FieldPolicyProvider[T]
	VirtualFields       VirtualFieldProcessor[T]
	Validator           ValidatorFunc[T]
	ActivityHooks       activity.Hooks
	ActivityConfig      activity.Config
	NotificationEmitter NotificationEmitter
	ResourceName        string
	ResourceType        reflect.Type
	BatchReturnOrderByID bool
}

// NewService composes the repository-backed service with optional layers in the
// default order (inner → outer): repo → virtual fields → validation → hooks →
// scope guard → field policy → activity/notifications. Alternate orderings
// should be implemented as custom wrappers by callers.
func NewService[T any](cfg ServiceConfig[T]) Service[T] {
	opts := RepositoryServiceOptions{}
	if cfg.BatchReturnOrderByID {
		opts.BatchInsertCriteria = []repository.InsertCriteria{repository.InsertReturnOrderByID()}
		opts.BatchUpdateCriteria = []repository.UpdateCriteria{repository.UpdateReturnOrderByID()}
	}
	base := NewRepositoryServiceWithOptions(cfg.Repository, opts)

	var svc Service[T] = base

	if cfg.VirtualFields != nil {
		svc = &virtualFieldService[T]{next: svc, handler: cfg.VirtualFields}
	}

	if cfg.Validator != nil {
		svc = &validationService[T]{next: svc, validate: cfg.Validator}
	}

	if !hooksEmpty(cfg.Hooks) {
		svc = &hooksService[T]{next: svc, hooks: cfg.Hooks}
	}

	if cfg.ScopeGuard != nil {
		svc = &scopeGuardService[T]{next: svc, guard: cfg.ScopeGuard}
	}

	if cfg.FieldPolicy != nil {
		resourceType := cfg.ResourceType
		if resourceType == nil {
			var zero T
			resourceType = reflect.TypeOf(zero)
		}
		svc = &fieldPolicyService[T]{
			next:         svc,
			provider:     cfg.FieldPolicy,
			resourceName: cfg.ResourceName,
			resourceType: resourceType,
		}
	}

	emitter := activity.NewEmitter(cfg.ActivityHooks, cfg.ActivityConfig)
	if (emitter != nil && emitter.Enabled()) || cfg.NotificationEmitter != nil {
		svc = &activityService[T]{
			next:                svc,
			emitter:             emitter,
			notificationEmitter: cfg.NotificationEmitter,
		}
	}

	return svc
}

// hooksEmpty reports whether all lifecycle hook slices are empty.
func hooksEmpty[T any](hooks LifecycleHooks[T]) bool {
	return len(hooks.BeforeCreate) == 0 &&
		len(hooks.AfterCreate) == 0 &&
		len(hooks.BeforeCreateBatch) == 0 &&
		len(hooks.AfterCreateBatch) == 0 &&
		len(hooks.BeforeUpdate) == 0 &&
		len(hooks.AfterUpdate) == 0 &&
		len(hooks.BeforeUpdateBatch) == 0 &&
		len(hooks.AfterUpdateBatch) == 0 &&
		len(hooks.AfterRead) == 0 &&
		len(hooks.AfterList) == 0 &&
		len(hooks.BeforeDelete) == 0 &&
		len(hooks.AfterDelete) == 0 &&
		len(hooks.BeforeDeleteBatch) == 0 &&
		len(hooks.AfterDeleteBatch) == 0
}

// hookContextFor builds a HookContext populated with request metadata, actor,
// scope, and correlation IDs gathered from the incoming crud.Context.
func hookContextFor(ctx Context, op CrudOperation) HookContext {
	uc := context.Background()
	if ctx != nil && ctx.UserContext() != nil {
		uc = ctx.UserContext()
	}

	meta := mergeHookMetadata(HookMetadata{Operation: op}, HookMetadataFromContext(uc))
	return HookContext{
		Context:              ctx,
		Metadata:             meta,
		Actor:                ActorFromContext(uc),
		Scope:                ScopeFromContext(uc),
		RequestID:            RequestIDFromContext(uc),
		CorrelationID:        CorrelationIDFromContext(uc),
		activityEmitterHooks: ActivityEmitterFromContext(uc),
		notificationEmitter:  NotificationEmitterFromContext(uc),
	}
}

// mergeHookMetadata prefers override metadata fields when they are non-empty.
func mergeHookMetadata(base, override HookMetadata) HookMetadata {
	if override.Operation != "" {
		base.Operation = override.Operation
	}
	if override.Resource != "" {
		base.Resource = override.Resource
	}
	if override.RouteName != "" {
		base.RouteName = override.RouteName
	}
	if override.Method != "" {
		base.Method = override.Method
	}
	if override.Path != "" {
		base.Path = override.Path
	}
	return base
}

// --- wrappers ---

// virtualFieldService injects virtual field processing before persisting and
// after loading records to keep metadata/virtuals in sync.
type virtualFieldService[T any] struct {
	next    Service[T]
	handler VirtualFieldProcessor[T]
}

// validationService runs ValidatorFunc before delegating to the next service.
type validationService[T any] struct {
	next     Service[T]
	validate ValidatorFunc[T]
}

// hooksService wraps lifecycle hooks around the downstream service calls.
type hooksService[T any] struct {
	next  Service[T]
	hooks LifecycleHooks[T]
}

// scopeGuardService resolves actor/scope and annotates the request context once
// per operation before invoking the next service.
type scopeGuardService[T any] struct {
	next  Service[T]
	guard ScopeGuardFunc[T]
}

// fieldPolicyService enforces row/column level policies returned by the
// provider before returning data to callers.
type fieldPolicyService[T any] struct {
	next         Service[T]
	provider     FieldPolicyProvider[T]
	resourceName string
	resourceType reflect.Type
}

// activityService emits activity and notifications after successful mutations.
type activityService[T any] struct {
	next                Service[T]
	emitter             *activity.Emitter
	notificationEmitter NotificationEmitter
}

// --- virtual fields ---

func (s *virtualFieldService[T]) Create(ctx Context, record T) (T, error) {
	hctx := hookContextFor(ctx, OpCreate)
	if err := s.handler.BeforeSave(hctx, record); err != nil {
		return record, err
	}
	res, err := s.next.Create(ctx, record)
	if err != nil {
		return res, err
	}
	_ = s.handler.AfterLoad(hctx, res)
	return res, nil
}

func (s *virtualFieldService[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	hctx := hookContextFor(ctx, OpCreateBatch)
	for i := range records {
		if err := s.handler.BeforeSave(hctx, records[i]); err != nil {
			return nil, err
		}
	}
	res, err := s.next.CreateBatch(ctx, records)
	if err != nil {
		return res, err
	}
	_ = s.handler.AfterLoadBatch(hctx, res)
	return res, nil
}

func (s *virtualFieldService[T]) Update(ctx Context, record T) (T, error) {
	hctx := hookContextFor(ctx, OpUpdate)
	if err := s.handler.BeforeSave(hctx, record); err != nil {
		return record, err
	}
	res, err := s.next.Update(ctx, record)
	if err != nil {
		return res, err
	}
	_ = s.handler.AfterLoad(hctx, res)
	return res, nil
}

func (s *virtualFieldService[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	hctx := hookContextFor(ctx, OpUpdateBatch)
	for i := range records {
		if err := s.handler.BeforeSave(hctx, records[i]); err != nil {
			return nil, err
		}
	}
	res, err := s.next.UpdateBatch(ctx, records)
	if err != nil {
		return res, err
	}
	_ = s.handler.AfterLoadBatch(hctx, res)
	return res, nil
}

func (s *virtualFieldService[T]) Delete(ctx Context, record T) error {
	return s.next.Delete(ctx, record)
}

func (s *virtualFieldService[T]) DeleteBatch(ctx Context, records []T) error {
	return s.next.DeleteBatch(ctx, records)
}

func (s *virtualFieldService[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	hctx := hookContextFor(ctx, OpList)
	res, count, err := s.next.Index(ctx, criteria)
	if err != nil {
		return res, count, err
	}
	_ = s.handler.AfterLoadBatch(hctx, res)
	return res, count, nil
}

func (s *virtualFieldService[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	hctx := hookContextFor(ctx, OpRead)
	res, err := s.next.Show(ctx, id, criteria)
	if err != nil {
		return res, err
	}
	_ = s.handler.AfterLoad(hctx, res)
	return res, nil
}

// --- validation ---

func (s *validationService[T]) Create(ctx Context, record T) (T, error) {
	if err := s.validate(ctx, record); err != nil {
		return record, err
	}
	return s.next.Create(ctx, record)
}

func (s *validationService[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	for i := range records {
		if err := s.validate(ctx, records[i]); err != nil {
			return nil, err
		}
	}
	return s.next.CreateBatch(ctx, records)
}

func (s *validationService[T]) Update(ctx Context, record T) (T, error) {
	if err := s.validate(ctx, record); err != nil {
		return record, err
	}
	return s.next.Update(ctx, record)
}

func (s *validationService[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	for i := range records {
		if err := s.validate(ctx, records[i]); err != nil {
			return nil, err
		}
	}
	return s.next.UpdateBatch(ctx, records)
}

func (s *validationService[T]) Delete(ctx Context, record T) error {
	return s.next.Delete(ctx, record)
}

func (s *validationService[T]) DeleteBatch(ctx Context, records []T) error {
	return s.next.DeleteBatch(ctx, records)
}

func (s *validationService[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	return s.next.Index(ctx, criteria)
}

func (s *validationService[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	return s.next.Show(ctx, id, criteria)
}

// --- hooks ---

func (s *hooksService[T]) Create(ctx Context, record T) (T, error) {
	meta := hookContextFor(ctx, OpCreate)
	if err := runHookFuncs(meta, s.hooks.BeforeCreate, record); err != nil {
		return record, err
	}
	res, err := s.next.Create(ctx, record)
	if err != nil {
		return res, err
	}
	if err := runHookFuncs(meta, s.hooks.AfterCreate, res); err != nil {
		return res, err
	}
	return res, nil
}

func (s *hooksService[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	meta := hookContextFor(ctx, OpCreateBatch)
	if err := runBatchHookFuncs(meta, s.hooks.BeforeCreateBatch, records); err != nil {
		return nil, err
	}
	res, err := s.next.CreateBatch(ctx, records)
	if err != nil {
		return res, err
	}
	if err := runBatchHookFuncs(meta, s.hooks.AfterCreateBatch, res); err != nil {
		return res, err
	}
	return res, nil
}

func (s *hooksService[T]) Update(ctx Context, record T) (T, error) {
	meta := hookContextFor(ctx, OpUpdate)
	if err := runHookFuncs(meta, s.hooks.BeforeUpdate, record); err != nil {
		return record, err
	}
	res, err := s.next.Update(ctx, record)
	if err != nil {
		return res, err
	}
	if err := runHookFuncs(meta, s.hooks.AfterUpdate, res); err != nil {
		return res, err
	}
	return res, nil
}

func (s *hooksService[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	meta := hookContextFor(ctx, OpUpdateBatch)
	if err := runBatchHookFuncs(meta, s.hooks.BeforeUpdateBatch, records); err != nil {
		return nil, err
	}
	res, err := s.next.UpdateBatch(ctx, records)
	if err != nil {
		return res, err
	}
	if err := runBatchHookFuncs(meta, s.hooks.AfterUpdateBatch, res); err != nil {
		return res, err
	}
	return res, nil
}

func (s *hooksService[T]) Delete(ctx Context, record T) error {
	meta := hookContextFor(ctx, OpDelete)
	if err := runHookFuncs(meta, s.hooks.BeforeDelete, record); err != nil {
		return err
	}
	if err := s.next.Delete(ctx, record); err != nil {
		return err
	}
	return runHookFuncs(meta, s.hooks.AfterDelete, record)
}

func (s *hooksService[T]) DeleteBatch(ctx Context, records []T) error {
	meta := hookContextFor(ctx, OpDeleteBatch)
	if err := runBatchHookFuncs(meta, s.hooks.BeforeDeleteBatch, records); err != nil {
		return err
	}
	if err := s.next.DeleteBatch(ctx, records); err != nil {
		return err
	}
	return runBatchHookFuncs(meta, s.hooks.AfterDeleteBatch, records)
}

func (s *hooksService[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	res, count, err := s.next.Index(ctx, criteria)
	if err != nil {
		return res, count, err
	}
	meta := hookContextFor(ctx, OpList)
	if err := runBatchHookFuncs(meta, s.hooks.AfterList, res); err != nil {
		return res, count, err
	}
	return res, count, nil
}

func (s *hooksService[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	res, err := s.next.Show(ctx, id, criteria)
	if err != nil {
		return res, err
	}
	meta := hookContextFor(ctx, OpRead)
	if err := runHookFuncs(meta, s.hooks.AfterRead, res); err != nil {
		return res, err
	}
	return res, nil
}

func runHookFuncs[T any](ctx HookContext, hooks []HookFunc[T], record T) error {
	for _, h := range hooks {
		if h == nil {
			continue
		}
		if err := h(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func runBatchHookFuncs[T any](ctx HookContext, hooks []HookBatchFunc[T], records []T) error {
	for _, h := range hooks {
		if h == nil {
			continue
		}
		if err := h(ctx, records); err != nil {
			return err
		}
	}
	return nil
}

// --- scope guard ---

func (s *scopeGuardService[T]) Create(ctx Context, record T) (T, error) {
	if meta, err := s.resolveGuard(ctx, OpCreate); err != nil {
		return record, err
	} else {
		ctx = meta
	}
	return s.next.Create(ctx, record)
}

func (s *scopeGuardService[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	if meta, err := s.resolveGuard(ctx, OpCreateBatch); err != nil {
		return nil, err
	} else {
		ctx = meta
	}
	return s.next.CreateBatch(ctx, records)
}

func (s *scopeGuardService[T]) Update(ctx Context, record T) (T, error) {
	if meta, err := s.resolveGuard(ctx, OpUpdate); err != nil {
		return record, err
	} else {
		ctx = meta
	}
	return s.next.Update(ctx, record)
}

func (s *scopeGuardService[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	if meta, err := s.resolveGuard(ctx, OpUpdateBatch); err != nil {
		return nil, err
	} else {
		ctx = meta
	}
	return s.next.UpdateBatch(ctx, records)
}

func (s *scopeGuardService[T]) Delete(ctx Context, record T) error {
	if meta, err := s.resolveGuard(ctx, OpDelete); err != nil {
		return err
	} else {
		ctx = meta
	}
	return s.next.Delete(ctx, record)
}

func (s *scopeGuardService[T]) DeleteBatch(ctx Context, records []T) error {
	if meta, err := s.resolveGuard(ctx, OpDeleteBatch); err != nil {
		return err
	} else {
		ctx = meta
	}
	return s.next.DeleteBatch(ctx, records)
}

func (s *scopeGuardService[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	guardCtx, err := s.resolveGuard(ctx, OpList)
	if err != nil {
		var zero []T
		return zero, 0, err
	}
	scope := ScopeFromContext(guardCtx.UserContext())
	criteria = append(criteria, scope.selectCriteria()...)
	return s.next.Index(guardCtx, criteria)
}

func (s *scopeGuardService[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	guardCtx, err := s.resolveGuard(ctx, OpRead)
	if err != nil {
		var zero T
		return zero, err
	}
	scope := ScopeFromContext(guardCtx.UserContext())
	criteria = append(criteria, scope.selectCriteria()...)
	return s.next.Show(guardCtx, id, criteria)
}

func (s *scopeGuardService[T]) resolveGuard(ctx Context, op CrudOperation) (Context, error) {
	actor, scope, err := s.guard(ctx, op)
	if err != nil {
		return ctx, err
	}
	attachActorToRequestContext(ctx, actor)
	attachScopeToRequestContext(ctx, scope)
	attachIdentifiersToRequestContext(ctx, resolveRequestID(ctx), resolveCorrelationID(ctx))
	return ctx, nil
}

// --- field policy ---

func (s *fieldPolicyService[T]) Create(ctx Context, record T) (T, error) {
	return s.next.Create(ctx, record)
}

func (s *fieldPolicyService[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	return s.next.CreateBatch(ctx, records)
}

func (s *fieldPolicyService[T]) Update(ctx Context, record T) (T, error) {
	return s.next.Update(ctx, record)
}

func (s *fieldPolicyService[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	return s.next.UpdateBatch(ctx, records)
}

func (s *fieldPolicyService[T]) Delete(ctx Context, record T) error {
	return s.next.Delete(ctx, record)
}

func (s *fieldPolicyService[T]) DeleteBatch(ctx Context, records []T) error {
	return s.next.DeleteBatch(ctx, records)
}

func (s *fieldPolicyService[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	decision, err := s.resolvePolicy(ctx, OpList)
	if err != nil {
		var zero []T
		return zero, 0, err
	}
	criteria = s.applyCriteria(criteria, decision)
	records, count, err := s.next.Index(ctx, criteria)
	if err != nil {
		return records, count, err
	}
	applyFieldPolicyToSlice(records, decision)
	return records, count, nil
}

func (s *fieldPolicyService[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	decision, err := s.resolvePolicy(ctx, OpRead)
	if err != nil {
		var zero T
		return zero, err
	}
	criteria = s.applyCriteria(criteria, decision)
	record, err := s.next.Show(ctx, id, criteria)
	if err != nil {
		return record, err
	}
	applyFieldPolicyToRecord(record, decision)
	return record, nil
}

func (s *fieldPolicyService[T]) resolvePolicy(ctx Context, op CrudOperation) (resolvedFieldPolicy, error) {
	if s.provider == nil {
		return resolvedFieldPolicy{}, nil
	}

	resource := s.resourceName
	if resource == "" && s.resourceType != nil {
		resource, _ = GetResourceName(s.resourceType)
	}

	request := FieldPolicyRequest[T]{
		Context:     ctx,
		Operation:   op,
		Actor:       ActorFromContext(ctx.UserContext()),
		Scope:       ScopeFromContext(ctx.UserContext()),
		Resource:    resource,
		ResourceTyp: s.resourceType,
	}

	policy, err := s.provider(request)
	if err != nil {
		return resolvedFieldPolicy{}, err
	}
	return buildResolvedFieldPolicy[T](policy, getAllowedFields[T](), resource, op), nil
}

func (s *fieldPolicyService[T]) applyCriteria(criteria []repository.SelectCriteria, decision resolvedFieldPolicy) []repository.SelectCriteria {
	if decision.isZero() {
		return criteria
	}
	return append(criteria, decision.rowFilterCriteria().selectCriteria()...)
}

// --- activity/notifications ---

func (s *activityService[T]) Create(ctx Context, record T) (T, error) {
	res, err := s.next.Create(ctx, record)
	s.emit(ctx, OpCreate, []T{res}, err)
	return res, err
}

func (s *activityService[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	res, err := s.next.CreateBatch(ctx, records)
	s.emit(ctx, OpCreateBatch, res, err)
	return res, err
}

func (s *activityService[T]) Update(ctx Context, record T) (T, error) {
	res, err := s.next.Update(ctx, record)
	s.emit(ctx, OpUpdate, []T{res}, err)
	return res, err
}

func (s *activityService[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	res, err := s.next.UpdateBatch(ctx, records)
	s.emit(ctx, OpUpdateBatch, res, err)
	return res, err
}

func (s *activityService[T]) Delete(ctx Context, record T) error {
	err := s.next.Delete(ctx, record)
	s.emit(ctx, OpDelete, []T{record}, err)
	return err
}

func (s *activityService[T]) DeleteBatch(ctx Context, records []T) error {
	err := s.next.DeleteBatch(ctx, records)
	s.emit(ctx, OpDeleteBatch, records, err)
	return err
}

func (s *activityService[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	return s.next.Index(ctx, criteria)
}

func (s *activityService[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	return s.next.Show(ctx, id, criteria)
}

func (s *activityService[T]) emit(ctx Context, op CrudOperation, records []T, err error) {
	if err != nil {
		return
	}
	if s.emitter != nil && s.emitter.Enabled() {
		for _, rec := range records {
			objType, objID := extractObjectInfo(rec)
			_ = s.emitter.Emit(ctx.UserContext(), activity.Event{
				Verb:       string(op),
				ObjectType: objType,
				ObjectID:   objID,
			})
		}
	}
	if s.notificationEmitter != nil && len(records) > 0 {
		hctx := hookContextFor(ctx, op)
		_ = SendNotificationBatch(hctx, ActivityPhaseAfter, records)
	}
}

func extractObjectInfo[T any](record T) (string, string) {
	rv := reflect.ValueOf(record)
	rt := reflect.TypeOf(record)
	if rt == nil {
		return "", ""
	}
	if rv.Kind() == reflect.Ptr && !rv.IsNil() {
		rv = rv.Elem()
		rt = rt.Elem()
	}
	id := ""
	if rv.IsValid() && rv.Kind() == reflect.Struct {
		if field := rv.FieldByName("ID"); field.IsValid() {
			id = fmt.Sprint(field.Interface())
		}
	}
	return rt.String(), id
}
