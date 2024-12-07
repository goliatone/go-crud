package crud

import (
	"fmt"
	"strings"

	"github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// CrudOperation defines the type for CRUD operations.
type CrudOperation string

const (
	OpCreate CrudOperation = "create"
	OpRead   CrudOperation = "read"
	OpList   CrudOperation = "list"
	OpUpdate CrudOperation = "update"
	OpDelete CrudOperation = "delete"
)

// Controller handles CRUD operations for a given model.
type Controller[T any] struct {
	Repo         repository.Repository[T]
	deserializer func(op CrudOperation, ctx Context) (T, error)
	resp         ResponseHandler[T]
	resource     string
}

// NewController creates a new Controller with functional options.
func NewController[T any](repo repository.Repository[T], opts ...Option[T]) *Controller[T] {
	controller := &Controller[T]{
		Repo:         repo,
		deserializer: DefaultDeserializer[T],
		resp:         NewDefaultResponseHandler[T](),
	}

	for _, opt := range opts {
		opt(controller)
	}
	return controller
}

func (c *Controller[T]) RegisterRoutes(r Router) {
	resource, resources := GetResourceName[T]()

	c.resource = resource

	r.Get(fmt.Sprintf("/%s/:id", resource), c.Show).
		Name(fmt.Sprintf("%s:%s", resource, OpRead))

	r.Get(fmt.Sprintf("/%s", resources), c.Index).
		Name(fmt.Sprintf("%s:%s", resource, OpList))

	r.Post(fmt.Sprintf("/%s", resource), c.Create).
		Name(fmt.Sprintf("%s:%s", resource, OpCreate))

	r.Put(fmt.Sprintf("/%s/:id", resource), c.Update).
		Name(fmt.Sprintf("%s:%s", resource, OpUpdate))

	r.Delete(fmt.Sprintf("/%s/:id", resource), c.Delete).
		Name(fmt.Sprintf("%s:%s", resource, OpDelete))
}

func (c *Controller[T]) Show(ctx Context) error {
	id := ctx.Params("id")
	record, err := c.Repo.GetByID(ctx.UserContext(), id)
	if err != nil {
		return c.resp.OnError(ctx, &NotFoundError{err}, OpRead)
	}
	return c.resp.OnData(ctx, record, OpRead)
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
	// Parse known query parameters
	limit := ctx.QueryInt("limit", 25)
	offset := ctx.QueryInt("offset", 0)
	order := ctx.Query("order")
	selectFields := ctx.Query("select")
	include := ctx.Query("include")

	filters := &Filters{
		Limit:  limit,
		Offset: offset,
	}

	var criteria []repository.SelectCriteria

	criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Limit(limit).Offset(offset)
	})

	allowedFieldsMap := getAllowedFields[T]()

	// Select fields
	if selectFields != "" {
		fields := strings.Split(selectFields, ",")
		var columns []string
		for _, field := range fields {
			columnName, ok := allowedFieldsMap[field]
			if !ok {
				//TODO: log info
				continue // skip, unknown fields!
			}
			columns = append(columns, columnName)
		}
		if len(columns) > 0 {
			criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Column(columns...)
			})
			filters.Fields = columns
		}
	}

	if order != "" {
		orders := strings.Split(order, ",")
		criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, o := range orders {
				parts := strings.Fields(strings.TrimSpace(o))
				if len(parts) > 0 {
					field := parts[0]
					direction := ""
					if len(parts) > 1 {
						direction = parts[1]
					}
					// Check if field is allowed
					columnName, ok := allowedFieldsMap[field]
					if ok {
						// Build order clause
						orderClause := columnName
						if direction != "" {
							orderClause += " " + direction
						}
						q = q.Order(orderClause)
						filters.Order = append(filters.Order, Order{
							Field: orderClause,
							Dir:   direction,
						})
					}
				}
			}
			return q
		})
	}

	// Include relations
	if include != "" {
		relations := strings.Split(include, ",")
		//TODO: Need to map user -> User e,g, table name to Class name
		filters.Include = append(filters.Include, relations...)
		criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, relation := range relations {
				q = q.Relation(relation)
			}
			return q
		})
	}

	// Build where conditions from other query parameters
	excludeParams := map[string]bool{
		"limit":   true,
		"offset":  true,
		"order":   true,
		"select":  true,
		"include": true,
	}

	queryParams := ctx.Queries()

	// For each parameter, if it's not in excludeParams, add a where condition
	criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
		for param, values := range queryParams {
			if excludeParams[param] {
				continue
			}

			// TODO: we could check that if we are in sqlite that we support the operator, e.g. ilike
			field, operator := parseFieldOperator(param)

			// Check if field is allowed and get column name
			columnName, ok := allowedFieldsMap[field]
			if !ok {
				continue // skip, not allowed TODO: Log
			}

			whereGroup := func(q *bun.SelectQuery) *bun.SelectQuery {
				for i, value := range strings.Split(values, ",") {
					value = strings.TrimSpace(value)
					if value == "" {
						continue
					}
					if i == 0 {
						q = q.Where(fmt.Sprintf("%s = ?", columnName), value)
					} else {
						q = q.WhereOr(fmt.Sprintf("%s = ?", columnName), value)
					}
				}
				return q
			}

			switch strings.ToLower(operator) {
			case "and":
				q = q.WhereGroup(" AND ", whereGroup)
			case "or":
				q = q.WhereGroup(" OR ", whereGroup)
			default:
				// Existing operators
				for _, value := range strings.Split(values, ",") {
					value = strings.TrimSpace(value)
					if value == "" {
						continue
					}
					q = q.Where(fmt.Sprintf("%s %s ?", columnName, operator), value)
				}
			}
		}
		return q
	})

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

	c.Repo.SetID(record, id)

	updatedRecord, err := c.Repo.Update(ctx.UserContext(), record)
	if err != nil {
		return c.resp.OnError(ctx, err, OpUpdate)
	}
	return c.resp.OnData(ctx, updatedRecord, OpUpdate)
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
