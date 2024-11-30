package crud

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/ettle/strcase"
	"github.com/gertd/go-pluralize"
	"github.com/gofiber/fiber/v2"
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

var pluralizer = pluralize.NewClient()

var operatorMap = DefaultOperatorMap()

func DefaultOperatorMap() map[string]string {
	return map[string]string{
		"eq":    "=",
		"ne":    "<>",
		"gt":    ">",
		"lt":    "<",
		"gte":   ">=",
		"lte":   "<=",
		"ilike": "ILIKE", // If we are using sqlite it does not support ILIKE
		"like":  "LIKE",
		"and":   "and",
		"or":    "or",
	}
}

func SetOperatorMap(om map[string]string) {
	operatorMap = om
}

// Option defines a functional option for Controller.
type Option[T any] func(*Controller[T])

// WithDeserializer sets a custom deserializer for the Controller.
func WithDeserializer[T any](d func(CrudOperation, *fiber.Ctx) (T, error)) Option[T] {
	return func(c *Controller[T]) {
		c.deserializer = d
	}
}

// DefaultDeserializer provides a generic deserializer.
func DefaultDeserializer[T any](op CrudOperation, ctx *fiber.Ctx) (T, error) {
	var record T
	v := reflect.ValueOf(&record).Elem()

	if v.Kind() == reflect.Ptr && v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}

	if err := ctx.BodyParser(record); err != nil {
		return record, err
	}
	return record, nil
}

// Controller handles CRUD operations for a given model.
type Controller[T any] struct {
	Repo         repository.Repository[T]
	deserializer func(op CrudOperation, ctx *fiber.Ctx) (T, error)
}

// NewController creates a new Controller with functional options.
func NewController[T any](repo repository.Repository[T], opts ...Option[T]) *Controller[T] {
	controller := &Controller[T]{
		Repo:         repo,
		deserializer: DefaultDeserializer[T],
	}

	for _, opt := range opts {
		opt(controller)
	}
	return controller
}

func (c *Controller[T]) RegisterRoutes(app fiber.Router) {
	resource, resources := GetResourceName[T]()

	app.Get(fmt.Sprintf("/%s/:id", resource), c.GetOne).
		Name(fmt.Sprintf("%s:%s", resource, OpRead))

	app.Get(fmt.Sprintf("/%s", resources), c.List).
		Name(fmt.Sprintf("%s:%s", resource, OpList))

	app.Post(fmt.Sprintf("/%s", resource), c.Create).
		Name(fmt.Sprintf("%s:%s", resource, OpCreate))

	app.Put(fmt.Sprintf("/%s/:id", resource), c.Update).
		Name(fmt.Sprintf("%s:%s", resource, OpUpdate))

	app.Delete(fmt.Sprintf("/%s/:id", resource), c.Delete).
		Name(fmt.Sprintf("%s:%s", resource, OpDelete))
}

func (c *Controller[T]) GetOne(ctx *fiber.Ctx) error {
	id := ctx.Params("id")
	record, err := c.Repo.GetByID(ctx.UserContext(), id)
	if err != nil {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Record not found"})
	}
	return ctx.JSON(record)
}

// List supports different query string parameters:
// GET /users?limit=10&offset=20
// GET /users?order=name asc,created_at desc
// GET /users?select=id,name,email
// GET /users?include=company,profile
// GET /users?name__ilike=John&age__gte=30
// GET /users?name__and=John,Jack
// GET /users?name__or=John,Jack
func (c *Controller[T]) List(ctx *fiber.Ctx) error {
	// Parse known query parameters
	limit := ctx.QueryInt("limit", 25)
	offset := ctx.QueryInt("offset", 0)
	order := ctx.Query("order")
	selectFields := ctx.Query("select")
	include := ctx.Query("include")

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
				continue // skip unknown fields
			}
			columns = append(columns, columnName)
		}
		if len(columns) > 0 {
			criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Column(columns...)
			})
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
					}
				}
			}
			return q
		})
	}

	// Include relations
	if include != "" {
		relations := strings.Split(include, ",")
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

	// Get all query parameters
	queryParams := ctx.Queries()

	// For each parameter, if it's not in excludeParams, add a where condition
	criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
		for param, values := range queryParams {
			if excludeParams[param] {
				continue
			}

			field, operator := parseFieldOperator(param)

			// Check if field is allowed and get column name
			columnName, ok := allowedFieldsMap[field]
			if !ok {
				continue // skip fields that are not allowed
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

			switch operator {
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

	records, _, err := c.Repo.List(ctx.UserContext(), criteria...)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve records"})
	}
	return ctx.JSON(records)
}

func (c *Controller[T]) Create(ctx *fiber.Ctx) error {
	record, err := c.deserializer(OpCreate, ctx)
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	createdRecord, err := c.Repo.Create(ctx.UserContext(), record)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create record"})
	}
	return ctx.Status(fiber.StatusCreated).JSON(createdRecord)
}

func (c *Controller[T]) Update(ctx *fiber.Ctx) error {
	idStr := ctx.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid ID format"})
	}

	record, err := c.deserializer(OpUpdate, ctx)
	if err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	c.Repo.SetID(record, id)

	updatedRecord, err := c.Repo.Update(ctx.UserContext(), record)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to update record: %s", err),
		})
	}
	return ctx.JSON(updatedRecord)
}

func (c *Controller[T]) Delete(ctx *fiber.Ctx) error {
	id := ctx.Params("id")
	record, err := c.Repo.GetByID(ctx.UserContext(), id)
	if err != nil {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Record not found"})
	}
	err = c.Repo.Delete(ctx.UserContext(), record)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete record"})
	}
	return ctx.SendStatus(fiber.StatusNoContent)
}

// GetResourceName returns the singular and plural resource names for type T.
// It first checks for a 'crud:"resource:..."' tag on any embedded fields.
// If found, it uses the specified resource name. Otherwise, it derives the name from the type's name.
func GetResourceName[T any]() (string, string) {
	var t T
	typ := reflect.TypeOf(t)

	// If T is a pointer, get the element type
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	// Initialize resourceName as empty
	resourceName := ""

	// Iterate over all fields to find the 'crud' tag
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		crudTag := field.Tag.Get("crud")
		if crudTag == "" {
			continue
		}

		// Parse the crud tag, expecting format 'resource:user'
		parts := strings.SplitN(crudTag, ":", 2)
		if len(parts) != 2 {
			continue // Invalid format, skip
		}

		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if key == "resource" && value != "" {
			resourceName = value
			break
		}
	}

	if resourceName == "" {
		// No 'crud' tag found, derive from type name
		typeName := typ.Name()
		resourceName = toKebabCase(typeName)
	}

	singular := pluralizer.Singular(resourceName)
	plural := pluralizer.Plural(resourceName)

	return singular, plural
}

func toKebabCase(s string) string {
	runes := []rune(s)
	var result []rune

	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) || (unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
					result = append(result, '-')
				}
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, r)
		}
	}

	return string(result)
}

func parseFieldOperator(param string) (field string, operator string) {
	operator = "="
	field = param
	var exists bool

	// Check if param contains "__" to separate field and operator
	if strings.Contains(param, "__") {
		parts := strings.SplitN(param, "__", 2)
		field = parts[0]
		op := parts[1]
		operator, exists = operatorMap[op]
		if !exists {
			operator = "="
		}
	}
	return
}

func getAllowedFields[T any]() map[string]string {
	var t T
	typ := reflect.TypeOf(t)
	// If T is a pointer, get the element type
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	fields := make(map[string]string)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		crudTag := field.Tag.Get("crud")
		if crudTag == "-" {
			continue // skip this field
		}

		// Get the bun tag to get the column name
		bunTag := field.Tag.Get("bun")
		var columnName string
		if bunTag != "" {
			parts := strings.Split(bunTag, ",")
			columnName = parts[0]
		} else {
			// Use the field name converted to snake_case
			columnName = strcase.ToSnake(field.Name)
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag != "" {
			jsonTag = strings.Split(jsonTag, ",")[0] // remove options
		} else {
			jsonTag = strcase.ToSnake(field.Name)
		}

		fields[jsonTag] = columnName
	}
	return fields
}
