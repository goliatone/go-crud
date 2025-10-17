package crud

import (
	"reflect"
	"strings"

	"github.com/goliatone/go-router"
)

const (
	TAG_CRUD         = "crud"
	TAG_BUN          = "bun"
	TAG_JSON         = "json"
	TAG_KEY_RESOURCE = "resource"
)

var operatorMap = DefaultOperatorMap()

type FieldMapProvider func(reflect.Type) map[string]string

func DefaultOperatorMap() map[string]string {
	return map[string]string{
		"eq":    "=",
		"ne":    "<>",
		"gt":    ">",
		"lt":    "<",
		"gte":   ">=",
		"lte":   "<=",
		"ilike": "ILIKE",
		"like":  "LIKE",
		"and":   "and",
		"or":    "or",
	}
}

func SetOperatorMap(om map[string]string) {
	operatorMap = om
}

type Option[T any] func(*Controller[T])

// WithDeserializer sets a custom deserializer for the Controller.
func WithDeserializer[T any](d func(CrudOperation, Context) (T, error)) Option[T] {
	return func(c *Controller[T]) {
		c.deserializer = d
	}
}

func WithResponseHandler[T any](handler ResponseHandler[T]) Option[T] {
	return func(c *Controller[T]) {
		c.resp = handler
	}
}

func WithLogger[T any](logger Logger) Option[T] {
	return func(c *Controller[T]) {
		c.logger = logger
	}
}

func WithFieldMapProvider[T any](provider FieldMapProvider) Option[T] {
	return func(c *Controller[T]) {
		c.fieldMapProvider = provider
	}
}

// DefaultDeserializer provides a generic deserializer.
func DefaultDeserializer[T any](op CrudOperation, ctx Context) (T, error) {
	var record T
	if err := ctx.BodyParser(&record); err != nil {
		return record, err
	}
	return record, nil
}

// DefaultDeserializerMany provides a generic deserializer.
func DefaultDeserializerMany[T any](op CrudOperation, ctx Context) ([]T, error) {
	var records []T
	if err := ctx.BodyParser(&records); err != nil {
		return records, err
	}
	return records, nil
}

// GetResourceName returns the singular and plural resource names for type T.
// It first checks for a 'crud:"resource:..."' tag on any embedded fields.
// If found, it uses the specified resource name. Otherwise, it derives the name from the type's name.
func GetResourceName(typ reflect.Type) (string, string) {
	return router.GetResourceName(typ)
}

func GetResourceTitle(typ reflect.Type) (string, string) {
	return router.GetResourceTitle(typ)
}

func parseFieldOperator(param string) (field string, operator string) {
	operator = "="
	field = strings.TrimSpace(param)
	var exists bool
	// TODO: support different formats, e.g. field[$operator]=value
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
	meta := getRelationMetadataForType(typeOf[T]())
	if meta == nil {
		return map[string]string{}
	}
	return cloneStringMap(meta.fields)
}
