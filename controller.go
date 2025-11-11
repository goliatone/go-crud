package crud

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	mergo "dario.cat/mergo"
	"github.com/ettle/strcase"
	"github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"
)

// CrudOperation defines the type for CRUD operations.
type CrudOperation string

const (
	OpCreate      CrudOperation = "create"
	OpCreateBatch CrudOperation = "create:batch"
	OpRead        CrudOperation = "read"
	OpList        CrudOperation = "list"
	OpUpdate      CrudOperation = "update"
	OpUpdateBatch CrudOperation = "update:batch"
	OpDelete      CrudOperation = "delete"
	OpDeleteBatch CrudOperation = "delete:batch"
)

var operationDefaultMethods = map[CrudOperation]string{
	OpCreate:      http.MethodPost,
	OpCreateBatch: http.MethodPost,
	OpRead:        http.MethodGet,
	OpList:        http.MethodGet,
	OpUpdate:      http.MethodPut,
	OpUpdateBatch: http.MethodPut,
	OpDelete:      http.MethodDelete,
	OpDeleteBatch: http.MethodDelete,
}

// Controller handles CRUD operations for a given model.
type Controller[T any] struct {
	Repo                repository.Repository[T]
	deserializer        func(op CrudOperation, ctx Context) (T, error)
	deserialiMany       func(op CrudOperation, ctx Context) ([]T, error)
	resp                ResponseHandler[T]
	service             Service[T]
	resource            string
	resourceType        reflect.Type
	logger              Logger
	fieldMapProvider    FieldMapProvider
	queryLoggingEnabled bool
	routeConfig         RouteConfig
	hooks               LifecycleHooks[T]
	activityEmitter     ActivityEmitter
	notificationEmitter NotificationEmitter
	fieldPolicyProvider FieldPolicyProvider[T]
	actions             []Action[T]
	actionDescriptors   []ActionDescriptor
	actionRouteDefs     []router.RouteDefinition
	adminScopeMetadata  AdminScopeMetadata
	adminMenuMetadata   AdminMenuMetadata
	rowFilterHints      []RowFilterHint
	routeMethods        map[CrudOperation]string
	routePaths          map[CrudOperation]string
	routeNames          map[CrudOperation]string
	relationProvider    router.RelationMetadataProvider
	scopeGuard          ScopeGuardFunc[T]
}

type optionItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type guardRequestContext struct {
	actor         ActorContext
	scope         ScopeFilter
	requestID     string
	correlationID string
}

func mergeRecordWithExisting[T any](record, existing T) (T, error) {
	rv := reflect.ValueOf(record)
	if !rv.IsValid() {
		var zero T
		return zero, fmt.Errorf("invalid record")
	}

	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv = reflect.New(rv.Type().Elem())
			if rv.CanInterface() {
				if converted, ok := rv.Interface().(T); ok {
					record = converted
				}
			}
		}
		if err := mergo.Merge(record, existing); err != nil {
			var zero T
			return zero, err
		}
		return record, nil
	}

	ptr := reflect.New(rv.Type())
	ptr.Elem().Set(rv)
	if err := mergo.Merge(ptr.Interface(), existing); err != nil {
		var zero T
		return zero, err
	}
	merged, ok := ptr.Elem().Interface().(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("failed to merge record")
	}
	return merged, nil
}

func (c *Controller[T]) resolveGuardContext(ctx Context, op CrudOperation) (guardRequestContext, error) {
	meta := guardRequestContext{}
	if ctx != nil {
		meta.actor = ActorFromContext(ctx.UserContext())
		meta.scope = ScopeFromContext(ctx.UserContext())
	}

	if c.scopeGuard != nil {
		actor, scope, err := c.scopeGuard(ctx, op)
		if err != nil {
			return guardRequestContext{}, err
		}
		if !actor.IsZero() {
			meta.actor = actor.Clone()
			attachActorToRequestContext(ctx, meta.actor)
		}
		meta.scope = scope.clone()
		attachScopeToRequestContext(ctx, meta.scope)
	} else {
		if !meta.actor.IsZero() {
			attachActorToRequestContext(ctx, meta.actor)
		}
		if meta.scope.HasFilters() || meta.scope.Bypass {
			attachScopeToRequestContext(ctx, meta.scope)
		}
	}

	meta.requestID = resolveRequestID(ctx)
	meta.correlationID = resolveCorrelationID(ctx)
	attachIdentifiersToRequestContext(ctx, meta.requestID, meta.correlationID)

	// Refresh from context to capture any identifiers stored by upstream middleware.
	if ctx != nil {
		if meta.actor.IsZero() {
			meta.actor = ActorFromContext(ctx.UserContext())
		}
		if !meta.scope.HasFilters() && !meta.scope.Bypass && len(meta.scope.Labels) == 0 && len(meta.scope.Raw) == 0 {
			meta.scope = ScopeFromContext(ctx.UserContext())
		}
	}

	return meta, nil
}

func (c *Controller[T]) applyScopeCriteria(criteria []repository.SelectCriteria, scope ScopeFilter) []repository.SelectCriteria {
	additional := scope.selectCriteria()
	if len(additional) == 0 {
		return criteria
	}
	if len(criteria) == 0 {
		return append([]repository.SelectCriteria{}, additional...)
	}
	return append(criteria, additional...)
}

func (c *Controller[T]) applyFieldPolicyCriteria(criteria []repository.SelectCriteria, decision resolvedFieldPolicy) []repository.SelectCriteria {
	filters := decision.rowFilterCriteria().selectCriteria()
	if len(filters) == 0 {
		return criteria
	}
	if len(criteria) == 0 {
		return append([]repository.SelectCriteria{}, filters...)
	}
	return append(criteria, filters...)
}

func (c *Controller[T]) resolveFieldPolicy(ctx Context, op CrudOperation, meta guardRequestContext) (resolvedFieldPolicy, error) {
	if c.fieldPolicyProvider == nil {
		return resolvedFieldPolicy{}, nil
	}

	request := FieldPolicyRequest[T]{
		Context:     ctx,
		Operation:   op,
		Actor:       meta.actor.Clone(),
		Scope:       meta.scope.clone(),
		Resource:    c.resource,
		ResourceTyp: c.resourceType,
	}

	policy, err := c.fieldPolicyProvider(request)
	if err != nil {
		return resolvedFieldPolicy{}, err
	}

	baseFields := getAllowedFields[T]()
	return buildResolvedFieldPolicy[T](policy, baseFields, c.resource, op), nil
}

func (c *Controller[T]) policyQueryOptions(decision resolvedFieldPolicy) []QueryBuilderOption {
	override := decision.allowedFieldOverride()
	if len(override) == 0 {
		return nil
	}
	return []QueryBuilderOption{WithAllowedFields(override)}
}

func (c *Controller[T]) logFieldPolicyDecision(decision resolvedFieldPolicy) {
	if decision.isZero() {
		return
	}
	LogFieldPolicyDecision(c.logger, decision.auditEntry())
}

// NewController creates a new Controller with functional options.
func NewController[T any](repo repository.Repository[T], opts ...Option[T]) *Controller[T] {
	var t T
	controller := &Controller[T]{
		Repo:             repo,
		deserializer:     DefaultDeserializer[T],
		deserialiMany:    DefaultDeserializerMany[T],
		resp:             NewDefaultResponseHandler[T](),
		service:          nil,
		resourceType:     reflect.TypeOf(t),
		logger:           &defaultLogger{},
		routeConfig:      DefaultRouteConfig(),
		routeMethods:     make(map[CrudOperation]string),
		routePaths:       make(map[CrudOperation]string),
		routeNames:       make(map[CrudOperation]string),
		relationProvider: router.NewDefaultRelationProvider(),
	}

	for _, opt := range opts {
		opt(controller)
	}

	if controller.service == nil {
		controller.service = NewRepositoryService(controller.Repo)
	}

	if controller.fieldMapProvider == nil {
		if provider := newFieldMapProviderFromRepo(controller.Repo, controller.resourceType); provider != nil {
			controller.fieldMapProvider = provider
		}
	}

	if controller.relationProvider == nil {
		controller.relationProvider = router.NewDefaultRelationProvider()
	}

	registerRelationProvider(controller.resourceType, controller.relationProvider)
	registerQueryConfig(controller.resourceType, controller.fieldMapProvider)

	return controller
}

func (c *Controller[T]) RegisterRoutes(r Router) {
	resource, resources := GetResourceName(c.resourceType)

	c.resource = resource

	resolvedActions := resolveActions(c.actions, resource, resources)
	c.setResolvedActions(resolvedActions)

	metadata := c.GetMetadata()
	routeMeta := map[string]router.RouteDefinition{}
	for _, def := range metadata.Routes {
		key := fmt.Sprintf("%s %s", string(def.Method), def.Path)
		routeMeta[key] = def
	}

	applyMeta := func(method, path string, info RouterRouteInfo) {
		metaAware, ok := info.(MetadataRouterRouteInfo)
		if !ok {
			return
		}
		if def, ok := routeMeta[fmt.Sprintf("%s %s", method, path)]; ok {
			if def.Description != "" {
				metaAware.Description(def.Description)
			}
			if def.Summary != "" {
				metaAware.Summary(def.Summary)
			}
			if len(def.Tags) > 0 {
				metaAware.Tags(def.Tags...)
			}
			for _, p := range def.Parameters {
				metaAware.Parameter(p.Name, p.In, p.Required, p.Schema)
			}
			if def.RequestBody != nil {
				metaAware.RequestBody(def.RequestBody.Description, def.RequestBody.Required, def.RequestBody.Content)
			}
			for _, resp := range def.Responses {
				metaAware.Response(resp.Code, resp.Description, resp.Content)
			}
		}
	}

	schemaPath := fmt.Sprintf("/%s/schema", resource)
	schemaRoute := fmt.Sprintf("%s:%s", resource, "schema")
	r.Get(schemaPath, c.Schema).
		Name(schemaRoute)

	registerRoute := func(op CrudOperation, defaultMethod, path string, handler func(Context) error, routeName string) {
		enabled, method := c.routeConfig.resolve(op, defaultMethod)
		if !enabled {
			return
		}
		info := invokeRoute(r, method, path, handler)
		if info == nil {
			return
		}
		named := info.Name(routeName)
		applyMeta(method, path, named)
		c.recordRouteMetadata(op, method, path, routeName)
	}

	showPath := fmt.Sprintf("/%s/:id", resource)
	readRoute := fmt.Sprintf("%s:%s", resource, OpRead)
	registerRoute(OpRead, http.MethodGet, showPath, c.Show, readRoute)

	listPath := fmt.Sprintf("/%s", resources)
	listRoute := fmt.Sprintf("%s:%s", resource, OpList)
	registerRoute(OpList, http.MethodGet, listPath, c.Index, listRoute)

	createBatchPath := fmt.Sprintf("/%s/batch", resource)
	createBatchRoute := fmt.Sprintf("%s:%s", resource, OpCreateBatch)
	registerRoute(OpCreateBatch, http.MethodPost, createBatchPath, c.CreateBatch, createBatchRoute)

	createPath := fmt.Sprintf("/%s", resource)
	createRoute := fmt.Sprintf("%s:%s", resource, OpCreate)
	registerRoute(OpCreate, http.MethodPost, createPath, c.Create, createRoute)

	updateBatchRoute := fmt.Sprintf("%s:%s", resource, OpUpdateBatch)
	registerRoute(OpUpdateBatch, http.MethodPut, createBatchPath, c.UpdateBatch, updateBatchRoute)

	updateRoute := fmt.Sprintf("%s:%s", resource, OpUpdate)
	registerRoute(OpUpdate, http.MethodPut, showPath, c.Update, updateRoute)

	deleteBatchRoute := fmt.Sprintf("%s:%s", resource, OpDeleteBatch)
	registerRoute(OpDeleteBatch, http.MethodDelete, createBatchPath, c.DeleteBatch, deleteBatchRoute)

	deleteRoute := fmt.Sprintf("%s:%s", resource, OpDelete)
	registerRoute(OpDelete, http.MethodDelete, showPath, c.Delete, deleteRoute)

	c.registerActionRoutes(r, resolvedActions, applyMeta)
}

func invokeRoute(r Router, method, path string, handler func(Context) error) RouterRouteInfo {
	switch method {
	case http.MethodGet:
		return r.Get(path, handler)
	case http.MethodPost:
		return r.Post(path, handler)
	case http.MethodPut:
		return r.Put(path, handler)
	case http.MethodPatch:
		return r.Patch(path, handler)
	case http.MethodDelete:
		return r.Delete(path, handler)
	default:
		return nil
	}
}

func (c *Controller[T]) setResolvedActions(actions []resolvedAction[T]) {
	if len(actions) == 0 {
		c.actionDescriptors = nil
		c.actionRouteDefs = nil
		return
	}
	c.actionDescriptors = make([]ActionDescriptor, 0, len(actions))
	c.actionRouteDefs = make([]router.RouteDefinition, 0, len(actions))
	for _, action := range actions {
		c.actionDescriptors = append(c.actionDescriptors, action.descriptor)
		c.actionRouteDefs = append(c.actionRouteDefs, action.routeDef)
	}
}

func (c *Controller[T]) registerActionRoutes(r Router, actions []resolvedAction[T], applyMeta func(method, path string, info RouterRouteInfo)) {
	for _, action := range actions {
		handler := c.buildActionHandler(action)
		info := invokeRoute(r, action.method, action.path, handler)
		if info == nil {
			continue
		}
		named := info.Name(action.routeName)
		if applyMeta != nil {
			applyMeta(action.method, action.path, named)
		}
		c.recordRouteMetadata(action.operation, action.method, action.path, action.routeName)
	}
}

func (c *Controller[T]) buildActionHandler(action resolvedAction[T]) func(Context) error {
	return func(ctx Context) error {
		meta, err := c.resolveGuardContext(ctx, action.operation)
		if err != nil {
			return c.resp.OnError(ctx, err, action.operation)
		}
		actx := ActionContext[T]{
			Context:       ctx,
			Actor:         meta.actor.Clone(),
			Scope:         meta.scope.clone(),
			RequestID:     meta.requestID,
			CorrelationID: meta.correlationID,
			Action:        action.descriptor,
			Operation:     action.operation,
		}
		if err := action.handler(actx); err != nil {
			return c.resp.OnError(ctx, err, action.operation)
		}
		return nil
	}
}

func (c *Controller[T]) recordRouteMetadata(op CrudOperation, method, path, routeName string) {
	if method != "" {
		c.routeMethods[op] = method
	}

	if path != "" {
		c.routePaths[op] = path
	}

	if routeName != "" {
		c.routeNames[op] = routeName
	}
}

func (c *Controller[T]) methodForOperation(op CrudOperation) string {
	if method, ok := c.routeMethods[op]; ok && method != "" {
		return method
	}

	if def, ok := operationDefaultMethods[op]; ok {
		return def
	}

	return ""
}

func (c *Controller[T]) routeNameForOperation(op CrudOperation) string {
	if name, ok := c.routeNames[op]; ok && name != "" {
		return name
	}

	if c.resource != "" {
		return fmt.Sprintf("%s:%s", c.resource, op)
	}

	return string(op)
}

func (c *Controller[T]) hookMetadata(op CrudOperation) HookMetadata {
	return HookMetadata{
		Operation: op,
		Resource:  c.resource,
		RouteName: c.routeNameForOperation(op),
		Method:    c.methodForOperation(op),
		Path:      c.routePaths[op],
	}
}

func (c *Controller[T]) newHookContext(ctx Context, op CrudOperation, meta guardRequestContext) HookContext {
	return HookContext{
		Context:             ctx,
		Metadata:            c.hookMetadata(op),
		Actor:               meta.actor.Clone(),
		Scope:               meta.scope.clone(),
		RequestID:           meta.requestID,
		CorrelationID:       meta.correlationID,
		activityEmitter:     c.activityEmitter,
		notificationEmitter: c.notificationEmitter,
	}
}

func (c *Controller[T]) runHook(ctx Context, op CrudOperation, hook HookFunc[T], record T, meta guardRequestContext) error {
	if hook == nil || isNil(record) {
		return nil
	}
	return hook(c.newHookContext(ctx, op, meta), record)
}

func (c *Controller[T]) runBatchHook(ctx Context, op CrudOperation, hook HookBatchFunc[T], records []T, meta guardRequestContext) error {
	if hook == nil || len(records) == 0 {
		return nil
	}
	return hook(c.newHookContext(ctx, op, meta), records)
}

func (c *Controller[T]) Schema(ctx Context) error {
	meta := c.GetMetadata()
	if meta.Name == "" {
		return ctx.SendStatus(http.StatusNotFound)
	}

	aggregator := router.NewMetadataAggregator().
		WithRelationProvider(router.NewDefaultRelationProvider())
	aggregator.AddProvider(c)

	relatedTypes := collectRelationResourceTypes(c.resourceType)
	if len(relatedTypes) > 0 {
		added := map[string]struct{}{meta.Name: {}}
		for _, typ := range relatedTypes {
			if typ == nil {
				continue
			}
			relatedMeta := router.GetResourceMetadata(typ)
			if relatedMeta == nil || relatedMeta.Name == "" {
				continue
			}
			if _, exists := added[relatedMeta.Name]; exists {
				continue
			}
			aggregator.AddProvider(staticMetadataProvider{metadata: *relatedMeta})
			added[relatedMeta.Name] = struct{}{}
		}
	}
	aggregator.Compile()

	doc := aggregator.GenerateOpenAPI()
	if len(doc) == 0 {
		return ctx.SendStatus(http.StatusNoContent)
	}

	c.applyAdminExtensions(doc, meta)
	return ctx.JSON(doc)
}

func (c *Controller[T]) applyAdminExtensions(doc map[string]any, meta router.ResourceMetadata) {
	if doc == nil {
		return
	}
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return
	}
	schemaName := meta.Schema.Name
	if schemaName == "" {
		schemaName = meta.Name
	}
	if schemaName == "" {
		return
	}
	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		return
	}
	if ext := c.adminScopeMetadata.toMap(); len(ext) > 0 {
		schema["x-admin-scope"] = ext
	}
	if len(c.actionDescriptors) > 0 {
		schema["x-admin-actions"] = c.actionDescriptors
	}
	if ext := c.adminMenuMetadata.toMap(); len(ext) > 0 {
		schema["x-admin-menu"] = ext
	}
	if len(c.rowFilterHints) > 0 {
		schema["x-admin-row-filters"] = cloneRowFilterHints(c.rowFilterHints)
	}
}

// Show supports different query string parameters:
// GET /user?include=Company,Profile
// GET /user?select=id,age,email
func (c *Controller[T]) Show(ctx Context) error {
	meta, err := c.resolveGuardContext(ctx, OpRead)
	if err != nil {
		return c.resp.OnError(ctx, err, OpRead)
	}

	policy, err := c.resolveFieldPolicy(ctx, OpRead, meta)
	if err != nil {
		return c.resp.OnError(ctx, err, OpRead)
	}
	c.logFieldPolicyDecision(policy)

	queryOpts := c.policyQueryOptions(policy)
	criteria, filters, err := BuildQueryCriteriaWithLogger[T](ctx, OpRead, c.logger, c.queryLoggingEnabled, queryOpts...)
	if err != nil {
		return c.resp.OnError(ctx, err, OpRead)
	}
	criteria = c.applyScopeCriteria(criteria, meta.scope)
	criteria = c.applyFieldPolicyCriteria(criteria, policy)

	id := ctx.Params("id")
	record, err := c.service.Show(ctx, id, criteria)
	if err != nil {
		return c.resp.OnError(ctx, &NotFoundError{err}, OpRead)
	}
	applyFieldPolicyToRecord(record, policy)
	return c.resp.OnData(ctx, record, OpRead, filters)
}

// Index supports different query string parameters:
// GET /users?limit=10&offset=20
// GET /users?order=name asc,created_at desc
// GET /users?select=id,name,email
// GET /users?include=company,profile
// GET /users?name__ilike=John&age__gte=30
// GET /users?name__and=John,Jack
// GET /users?name__or=John,Jack
func (c *Controller[T]) Index(ctx Context) error {
	meta, err := c.resolveGuardContext(ctx, OpList)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}

	policy, err := c.resolveFieldPolicy(ctx, OpList, meta)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}
	c.logFieldPolicyDecision(policy)

	queryOpts := c.policyQueryOptions(policy)
	criteria, filters, err := BuildQueryCriteriaWithLogger[T](ctx, OpList, c.logger, c.queryLoggingEnabled, queryOpts...)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}
	criteria = c.applyScopeCriteria(criteria, meta.scope)
	criteria = c.applyFieldPolicyCriteria(criteria, policy)

	records, count, err := c.service.Index(ctx, criteria)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}

	filters.Count = count
	applyFieldPolicyToSlice(records, policy)

	if shouldReturnOptions(ctx) {
		options := c.buildOptionItems(records)
		return ctx.Status(http.StatusOK).JSON(options)
	}

	return c.resp.OnList(ctx, records, OpList, filters)
}

func (c *Controller[T]) Create(ctx Context) error {
	meta, err := c.resolveGuardContext(ctx, OpCreate)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}

	policy, err := c.resolveFieldPolicy(ctx, OpCreate, meta)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}
	c.logFieldPolicyDecision(policy)

	record, err := c.deserializer(OpCreate, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpCreate)
	}

	if err := c.runHook(ctx, OpCreate, c.hooks.BeforeCreate, record, meta); err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}

	createdRecord, err := c.service.Create(ctx, record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}

	if err := c.runHook(ctx, OpCreate, c.hooks.AfterCreate, createdRecord, meta); err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}
	applyFieldPolicyToRecord(createdRecord, policy)
	return c.resp.OnData(ctx, createdRecord, OpCreate)
}

func (c *Controller[T]) CreateBatch(ctx Context) error {
	meta, err := c.resolveGuardContext(ctx, OpCreateBatch)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreateBatch)
	}

	policy, err := c.resolveFieldPolicy(ctx, OpCreateBatch, meta)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreateBatch)
	}
	c.logFieldPolicyDecision(policy)

	records, err := c.deserialiMany(OpCreateBatch, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpCreateBatch)
	}

	if err := c.runBatchHook(ctx, OpCreateBatch, c.hooks.BeforeCreateBatch, records, meta); err != nil {
		return c.resp.OnError(ctx, err, OpCreateBatch)
	}

	createdRecords, err := c.service.CreateBatch(ctx, records)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreateBatch)
	}

	if err := c.runBatchHook(ctx, OpCreateBatch, c.hooks.AfterCreateBatch, createdRecords, meta); err != nil {
		return c.resp.OnError(ctx, err, OpCreateBatch)
	}
	applyFieldPolicyToSlice(createdRecords, policy)

	if shouldReturnOptions(ctx) {
		options := c.buildOptionItems(createdRecords)
		return ctx.Status(http.StatusOK).JSON(options)
	}

	return c.resp.OnList(ctx, createdRecords, OpCreateBatch, &Filters{
		Count:     len(createdRecords),
		Operation: string(OpCreateBatch),
	})
}

func (c *Controller[T]) Update(ctx Context) error {
	meta, err := c.resolveGuardContext(ctx, OpUpdate)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}

	policy, err := c.resolveFieldPolicy(ctx, OpUpdate, meta)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}
	c.logFieldPolicyDecision(policy)

	idStr := ctx.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpUpdate)
	}

	record, err := c.deserializer(OpUpdate, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpUpdate)
	}

	c.Repo.Handlers().SetID(record, id)
	criteria := c.applyScopeCriteria(nil, meta.scope)
	criteria = c.applyFieldPolicyCriteria(criteria, policy)
	existingRecord, err := c.service.Show(ctx, idStr, criteria)
	if err != nil {
		return c.resp.OnError(ctx, &NotFoundError{err}, OpUpdate)
	}

	record, err = mergeRecordWithExisting(record, existingRecord)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}

	if err := c.runHook(ctx, OpUpdate, c.hooks.BeforeUpdate, record, meta); err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}

	updatedRecord, err := c.service.Update(ctx, record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}

	if err := c.runHook(ctx, OpUpdate, c.hooks.AfterUpdate, updatedRecord, meta); err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}
	applyFieldPolicyToRecord(updatedRecord, policy)
	return c.resp.OnData(ctx, updatedRecord, OpUpdate)
}

func (c *Controller[T]) UpdateBatch(ctx Context) error {
	meta, err := c.resolveGuardContext(ctx, OpUpdateBatch)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}

	policy, err := c.resolveFieldPolicy(ctx, OpUpdateBatch, meta)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}
	c.logFieldPolicyDecision(policy)

	records, err := c.deserialiMany(OpUpdateBatch, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpUpdateBatch)
	}

	if err := c.runBatchHook(ctx, OpUpdateBatch, c.hooks.BeforeUpdateBatch, records, meta); err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}

	updatedRecords, err := c.service.UpdateBatch(ctx, records)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}

	if err := c.runBatchHook(ctx, OpUpdateBatch, c.hooks.AfterUpdateBatch, updatedRecords, meta); err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}
	applyFieldPolicyToSlice(updatedRecords, policy)

	if shouldReturnOptions(ctx) {
		options := c.buildOptionItems(updatedRecords)
		return ctx.Status(http.StatusOK).JSON(options)
	}

	return c.resp.OnList(ctx, updatedRecords, OpUpdateBatch, &Filters{
		Count:     len(updatedRecords),
		Operation: string(OpUpdateBatch),
	})
}

func (c *Controller[T]) Delete(ctx Context) error {
	meta, err := c.resolveGuardContext(ctx, OpDelete)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	policy, err := c.resolveFieldPolicy(ctx, OpDelete, meta)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}
	c.logFieldPolicyDecision(policy)

	id := ctx.Params("id")
	criteria := c.applyScopeCriteria(nil, meta.scope)
	criteria = c.applyFieldPolicyCriteria(criteria, policy)
	record, err := c.service.Show(ctx, id, criteria)
	if err != nil {
		return c.resp.OnError(ctx, &NotFoundError{err}, OpDelete)
	}

	if err := c.runHook(ctx, OpDelete, c.hooks.BeforeDelete, record, meta); err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	err = c.service.Delete(ctx, record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	if err := c.runHook(ctx, OpDelete, c.hooks.AfterDelete, record, meta); err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	return c.resp.OnEmpty(ctx, OpDelete)
}

func (c *Controller[T]) DeleteBatch(ctx Context) error {
	meta, err := c.resolveGuardContext(ctx, OpDeleteBatch)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDeleteBatch)
	}

	policy, err := c.resolveFieldPolicy(ctx, OpDeleteBatch, meta)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDeleteBatch)
	}
	c.logFieldPolicyDecision(policy)

	records, err := c.deserialiMany(OpDeleteBatch, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpDeleteBatch)
	}

	if err := c.runBatchHook(ctx, OpDeleteBatch, c.hooks.BeforeDeleteBatch, records, meta); err != nil {
		return c.resp.OnError(ctx, err, OpDeleteBatch)
	}

	err = c.service.DeleteBatch(ctx, records)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDeleteBatch)
	}

	if err := c.runBatchHook(ctx, OpDeleteBatch, c.hooks.AfterDeleteBatch, records, meta); err != nil {
		return c.resp.OnError(ctx, err, OpDeleteBatch)
	}

	return c.resp.OnEmpty(ctx, OpDeleteBatch)
}

func shouldReturnOptions(ctx Context) bool {
	return strings.EqualFold(strings.TrimSpace(ctx.Query("format")), "options")
}

func (c *Controller[T]) buildOptionItems(records []T) []optionItem {
	if len(records) == 0 {
		return nil
	}

	metadata := c.GetMetadata()
	labelField := metadata.Schema.LabelField
	handlers := c.Repo.Handlers()

	getID := handlers.GetID
	getIdentifierValue := handlers.GetIdentifierValue

	options := make([]optionItem, 0, len(records))
	for _, record := range records {
		value := ""
		if getID != nil {
			value = strings.TrimSpace(fmt.Sprint(getID(record)))
		}
		if value == "" && getIdentifierValue != nil {
			value = strings.TrimSpace(getIdentifierValue(record))
		}
		if value == "" {
			if v, ok := jsonFieldAsString(record, "id"); ok {
				value = v
			}
		}

		label := ""
		if labelField != "" {
			if v, ok := jsonFieldAsString(record, labelField); ok {
				label = v
			}
		}
		if label == "" && getIdentifierValue != nil {
			if v := strings.TrimSpace(getIdentifierValue(record)); v != "" {
				label = v
			}
		}
		if label == "" {
			label = value
		}

		options = append(options, optionItem{
			Value: value,
			Label: label,
		})
	}

	return options
}

func jsonFieldAsString(record any, target string) (string, bool) {
	if target == "" {
		return "", false
	}

	value := reflect.ValueOf(record)
	if !value.IsValid() {
		return "", false
	}

	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", false
		}
		value = value.Elem()
	}

	if value.Kind() != reflect.Struct {
		return "", false
	}

	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldName := jsonFieldName(field)
		if fieldName != target {
			continue
		}

		fieldValue := value.Field(i)
		for fieldValue.Kind() == reflect.Pointer {
			if fieldValue.IsNil() {
				return "", false
			}
			fieldValue = fieldValue.Elem()
		}

		return strings.TrimSpace(fmt.Sprint(fieldValue.Interface())), true
	}

	return "", false
}

func jsonFieldName(field reflect.StructField) string {
	if tagValue := field.Tag.Get("json"); tagValue != "" {
		if name := strings.Split(tagValue, ",")[0]; name != "" && name != "-" {
			return name
		}
	}
	return strcase.ToSnake(field.Name)
}

func isNil[T any](val T) bool {
	v := any(val)
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func applyFieldPolicyToRecord[T any](record T, decision resolvedFieldPolicy) {
	if decision.isZero() || isNil(record) {
		return
	}
	applyFieldPolicyValue(reflect.ValueOf(record), decision)
}

func applyFieldPolicyToSlice[T any](records []T, decision resolvedFieldPolicy) {
	if decision.isZero() || len(records) == 0 {
		return
	}
	rv := reflect.ValueOf(records)
	if rv.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < rv.Len(); i++ {
		applyFieldPolicyValue(rv.Index(i), decision)
	}
}

func applyFieldPolicyValue(val reflect.Value, decision resolvedFieldPolicy) {
	if !val.IsValid() {
		return
	}
	switch val.Kind() {
	case reflect.Pointer:
		if val.IsNil() {
			return
		}
		applyFieldPolicyStruct(val.Elem(), decision)
	case reflect.Struct:
		applyFieldPolicyStruct(val, decision)
	default:
		if val.CanAddr() {
			applyFieldPolicyValue(val.Addr(), decision)
		}
	}
}

func applyFieldPolicyStruct(val reflect.Value, decision resolvedFieldPolicy) {
	if val.Kind() != reflect.Struct {
		return
	}
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || field.Tag.Get(TAG_CRUD) == "-" {
			continue
		}
		fieldName := jsonFieldName(field)
		fieldValue := val.Field(i)
		if !fieldValue.CanSet() && fieldValue.CanAddr() {
			fieldValue = fieldValue.Addr()
		}
		if !decision.allowsField(fieldName) {
			zeroReflectValue(fieldValue)
			continue
		}
		if mask := decision.maskFor(fieldName); mask != nil {
			applyMaskValue(fieldValue, mask)
		}
	}
}

func zeroReflectValue(val reflect.Value) {
	if !val.IsValid() {
		return
	}
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return
		}
		if val.Elem().CanSet() {
			val.Elem().Set(reflect.Zero(val.Elem().Type()))
		}
		if val.CanSet() {
			val.Set(reflect.Zero(val.Type()))
		}
		return
	}
	if val.CanSet() {
		val.Set(reflect.Zero(val.Type()))
	}
}

func applyMaskValue(val reflect.Value, mask FieldMaskFunc) {
	if !val.IsValid() || !val.CanSet() || mask == nil {
		return
	}
	current := val.Interface()
	masked := mask(current)
	if masked == nil {
		val.Set(reflect.Zero(val.Type()))
		return
	}
	maskedValue := reflect.ValueOf(masked)
	if !maskedValue.Type().AssignableTo(val.Type()) {
		if maskedValue.Type().ConvertibleTo(val.Type()) {
			maskedValue = maskedValue.Convert(val.Type())
		} else {
			return
		}
	}
	val.Set(maskedValue)
}
