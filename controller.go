package crud

import (
	"fmt"

	"github.com/goliatone/go-repository-bun"
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

// Controller handles CRUD operations for a given model.
type Controller[T any] struct {
	Repo          repository.Repository[T]
	deserializer  func(op CrudOperation, ctx Context) (T, error)
	deserialiMany func(op CrudOperation, ctx Context) ([]T, error)
	resp          ResponseHandler[T]
	resource      string
}

// NewController creates a new Controller with functional options.
func NewController[T any](repo repository.Repository[T], opts ...Option[T]) *Controller[T] {
	controller := &Controller[T]{
		Repo:          repo,
		deserializer:  DefaultDeserializer[T],
		deserialiMany: DefaultDeserializerMany[T],
		resp:          NewDefaultResponseHandler[T](),
	}

	for _, opt := range opts {
		opt(controller)
	}
	return controller
}

func (c *Controller[T]) RegisterRoutes(r Router) {
	resource, resources := GetResourceName[T]()

	c.resource = resource

	r.Get(fmt.Sprintf("/%s/schema", resource), c.Schema).
		Name(fmt.Sprintf("%s:%s", resource, "schema"))

	r.Get(fmt.Sprintf("/%s/:id", resource), c.Show).
		Name(fmt.Sprintf("%s:%s", resource, OpRead))

	r.Get(fmt.Sprintf("/%s", resources), c.Index).
		Name(fmt.Sprintf("%s:%s", resource, OpList))

	r.Post(fmt.Sprintf("/%s/batch", resource), c.CreateBatch).
		Name(fmt.Sprintf("%s:%s", resource, OpCreateBatch))

	r.Post(fmt.Sprintf("/%s", resource), c.Create).
		Name(fmt.Sprintf("%s:%s", resource, OpCreate))

	r.Put(fmt.Sprintf("/%s/batch", resource), c.UpdateBatch).
		Name(fmt.Sprintf("%s:%s", resource, OpUpdateBatch))

	r.Put(fmt.Sprintf("/%s/:id", resource), c.Update).
		Name(fmt.Sprintf("%s:%s", resource, OpUpdate))

	r.Delete(fmt.Sprintf("/%s/batch", resource), c.DeleteBatch).
		Name(fmt.Sprintf("%s:%s", resource, OpDeleteBatch))

	r.Delete(fmt.Sprintf("/%s/:id", resource), c.Delete).
		Name(fmt.Sprintf("%s:%s", resource, OpDelete))
}

func (c *Controller[T]) Schema(ctx Context) error {
	return ctx.JSON(c.GetMetadata())
}

// Show supports different query string parameters:
// GET /user?include=Company,Profile
// GET /user?select=id,age,email
func (c *Controller[T]) Show(ctx Context) error {
	criteria, filters, err := BuildQueryCriteria[T](ctx, OpList)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}

	id := ctx.Params("id")
	record, err := c.Repo.GetByID(ctx.UserContext(), id, criteria...)
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
	criteria, filters, err := BuildQueryCriteria[T](ctx, OpList)
	if err != nil {
		return c.resp.OnError(ctx, err, OpList)
	}
	records, count, err := c.Repo.List(ctx.UserContext(), criteria...)
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

	createdRecord, err := c.Repo.Create(ctx.UserContext(), record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpCreate)
	}
	return c.resp.OnData(ctx, createdRecord, OpCreate)
}

func (c *Controller[T]) CreateBatch(ctx Context) error {
	records, err := c.deserialiMany(OpCreateBatch, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpCreateBatch)
	}
	createdRecords, err := c.Repo.CreateMany(ctx.UserContext(), records)
	if err != nil {
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

	updatedRecord, err := c.Repo.Update(ctx.UserContext(), record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}
	return c.resp.OnData(ctx, updatedRecord, OpUpdate)
}

func (c *Controller[T]) UpdateBatch(ctx Context) error {
	records, err := c.deserialiMany(OpUpdate, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpUpdateBatch)
	}

	updatedRecords, err := c.Repo.UpdateMany(ctx.UserContext(), records)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdateBatch)
	}

	return c.resp.OnList(ctx, updatedRecords, OpUpdateBatch, &Filters{
		Count:     len(updatedRecords),
		Operation: string(OpUpdateBatch),
	})
}

func (c *Controller[T]) Delete(ctx Context) error {
	id := ctx.Params("id")
	record, err := c.Repo.GetByID(ctx.UserContext(), id)
	if err != nil {
		return c.resp.OnError(ctx, &NotFoundError{err}, OpDelete)
	}

	err = c.Repo.Delete(ctx.UserContext(), record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	return c.resp.OnEmpty(ctx, OpDelete)
}

func (c *Controller[T]) DeleteBatch(ctx Context) error {
	records, err := c.deserialiMany(OpUpdate, ctx)
	if err != nil {
		return c.resp.OnError(ctx, &ValidationError{err}, OpUpdateBatch)
	}

	criteria := []repository.DeleteCriteria{}
	for _, record := range records {
		id := c.Repo.Handlers().GetID(record)
		criteria = append(criteria, repository.DeleteByID(id.String()))
	}

	err = c.Repo.DeleteMany(ctx.UserContext(), criteria...)
	if err != nil {
		return c.resp.OnError(ctx, err, OpDelete)
	}

	return c.resp.OnEmpty(ctx, OpDelete)
}
