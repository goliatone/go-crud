package crud

import (
	"fmt"
	"net/http"
	"reflect"

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
	routeMethods        map[CrudOperation]string
	routePaths          map[CrudOperation]string
	routeNames          map[CrudOperation]string
}

// NewController creates a new Controller with functional options.
func NewController[T any](repo repository.Repository[T], opts ...Option[T]) *Controller[T] {
	var t T
	controller := &Controller[T]{
		Repo:          repo,
		deserializer:  DefaultDeserializer[T],
		deserialiMany: DefaultDeserializerMany[T],
		resp:          NewDefaultResponseHandler[T](),
		service:       nil,
		resourceType:  reflect.TypeOf(t),
		logger:        &defaultLogger{},
		routeConfig:   DefaultRouteConfig(),
		routeMethods:  make(map[CrudOperation]string),
		routePaths:    make(map[CrudOperation]string),
		routeNames:    make(map[CrudOperation]string),
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

	registerQueryConfig(controller.resourceType, controller.fieldMapProvider)

	return controller
}

func (c *Controller[T]) RegisterRoutes(r Router) {
	resource, resources := GetResourceName(c.resourceType)

	c.resource = resource

	metadata := router.GetResourceMetadata(c.resourceType)
	routeMeta := map[string]router.RouteDefinition{}
	if metadata != nil {
		for _, def := range metadata.Routes {
			key := fmt.Sprintf("%s %s", string(def.Method), def.Path)
			routeMeta[key] = def
		}
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

func (c *Controller[T]) newHookContext(ctx Context, op CrudOperation) HookContext {
	return HookContext{
		Context:  ctx,
		Metadata: c.hookMetadata(op),
	}
}

func (c *Controller[T]) runHook(ctx Context, op CrudOperation, hook HookFunc[T], record T) error {
	if hook == nil || isNil(record) {
		return nil
	}
	return hook(c.newHookContext(ctx, op), record)
}

func (c *Controller[T]) runBatchHook(ctx Context, op CrudOperation, hook HookBatchFunc[T], records []T) error {
	if hook == nil || len(records) == 0 {
		return nil
	}
	return hook(c.newHookContext(ctx, op), records)
}

func (c *Controller[T]) Schema(ctx Context) error {
	meta := c.GetMetadata()
	if meta.Name == "" {
		return ctx.SendStatus(http.StatusNotFound)
	}

	aggregator := router.NewMetadataAggregator()
	aggregator.AddProvider(c)
	aggregator.Compile()

	doc := aggregator.GenerateOpenAPI()
	if len(doc) == 0 {
		return ctx.SendStatus(http.StatusNoContent)
	}

	return ctx.JSON(doc)
}

// Show supports different query string parameters:
// GET /user?include=Company,Profile
// GET /user?select=id,age,email
func (c *Controller[T]) Show(ctx Context) error {
	criteria, filters, err := BuildQueryCriteriaWithLogger[T](ctx, OpList, c.logger, c.queryLoggingEnabled)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}

	id := ctx.Params("id")
	record, err := c.service.Show(ctx, id, criteria)
	if err != nil {
		return c.resp.OnError(ctx, &NotFoundError{err}, OpRead)
	}
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
	criteria, filters, err := BuildQueryCriteriaWithLogger[T](ctx, OpList, c.logger, c.queryLoggingEnabled)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}
	records, count, err := c.service.Index(ctx, criteria)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}

	filters.Count = count

	return c.resp.OnList(ctx, records, OpList, filters)
}

func (c *Controller[T]) Create(ctx Context) error {
	record, err := c.deserializer(OpCreate, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpCreate)
	}

	if err := c.runHook(ctx, OpCreate, c.hooks.BeforeCreate, record); err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}

	createdRecord, err := c.service.Create(ctx, record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}

	if err := c.runHook(ctx, OpCreate, c.hooks.AfterCreate, createdRecord); err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}
	return c.resp.OnData(ctx, createdRecord, OpCreate)
}

func (c *Controller[T]) CreateBatch(ctx Context) error {
	records, err := c.deserialiMany(OpCreateBatch, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpCreateBatch)
	}

	if err := c.runBatchHook(ctx, OpCreateBatch, c.hooks.BeforeCreateBatch, records); err != nil {
		return c.resp.OnError(ctx, err, OpCreateBatch)
	}

	createdRecords, err := c.service.CreateBatch(ctx, records)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreateBatch)
	}

	if err := c.runBatchHook(ctx, OpCreateBatch, c.hooks.AfterCreateBatch, createdRecords); err != nil {
		return c.resp.OnError(ctx, err, OpCreateBatch)
	}

	return c.resp.OnList(ctx, createdRecords, OpCreateBatch, &Filters{
		Count:     len(createdRecords),
		Operation: string(OpCreateBatch),
	})
}

func (c *Controller[T]) Update(ctx Context) error {
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

	if err := c.runHook(ctx, OpUpdate, c.hooks.BeforeUpdate, record); err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}

	updatedRecord, err := c.service.Update(ctx, record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}

	if err := c.runHook(ctx, OpUpdate, c.hooks.AfterUpdate, updatedRecord); err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}
	return c.resp.OnData(ctx, updatedRecord, OpUpdate)
}

func (c *Controller[T]) UpdateBatch(ctx Context) error {
	records, err := c.deserialiMany(OpUpdateBatch, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpUpdateBatch)
	}

	if err := c.runBatchHook(ctx, OpUpdateBatch, c.hooks.BeforeUpdateBatch, records); err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}

	updatedRecords, err := c.service.UpdateBatch(ctx, records)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}

	if err := c.runBatchHook(ctx, OpUpdateBatch, c.hooks.AfterUpdateBatch, updatedRecords); err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}

	return c.resp.OnList(ctx, updatedRecords, OpUpdateBatch, &Filters{
		Count:     len(updatedRecords),
		Operation: string(OpUpdateBatch),
	})
}

func (c *Controller[T]) Delete(ctx Context) error {
	id := ctx.Params("id")
	record, err := c.service.Show(ctx, id, nil)
	if err != nil {
		return c.resp.OnError(ctx, &NotFoundError{err}, OpDelete)
	}

	if err := c.runHook(ctx, OpDelete, c.hooks.BeforeDelete, record); err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	err = c.service.Delete(ctx, record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	if err := c.runHook(ctx, OpDelete, c.hooks.AfterDelete, record); err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	return c.resp.OnEmpty(ctx, OpDelete)
}

func (c *Controller[T]) DeleteBatch(ctx Context) error {
	records, err := c.deserialiMany(OpDeleteBatch, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpDeleteBatch)
	}

	if err := c.runBatchHook(ctx, OpDeleteBatch, c.hooks.BeforeDeleteBatch, records); err != nil {
		return c.resp.OnError(ctx, err, OpDeleteBatch)
	}

	err = c.service.DeleteBatch(ctx, records)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDeleteBatch)
	}

	if err := c.runBatchHook(ctx, OpDeleteBatch, c.hooks.AfterDeleteBatch, records); err != nil {
		return c.resp.OnError(ctx, err, OpDeleteBatch)
	}

	return c.resp.OnEmpty(ctx, OpDeleteBatch)
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
