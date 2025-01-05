package crud

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/ettle/strcase"
	"github.com/gertd/go-pluralize"
)

const (
	TAG_CRUD         = "crud"
	TAG_BUN          = "bun"
	TAG_JSON         = "json"
	TAG_KEY_RESOURCE = "resource"
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
func GetResourceName[T any]() (string, string) {
	var t T
	typ := reflect.TypeOf(t)

	// If T is a pointer, get the element type
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	resourceName := ""
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		crudTag := field.Tag.Get(TAG_CRUD)
		if crudTag == "" {
			continue
		}

		// Parse the crud tag, expecting format 'resource:user'
		parts := strings.SplitN(crudTag, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if key == TAG_KEY_RESOURCE && value != "" {
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

func GetResourceTitle[T any]() (string, string) {
	resourceName, pluralName := GetResourceName[T]()
	name := strcase.ToCase(resourceName, strcase.TitleCase, ' ')
	names := strcase.ToCase(pluralName, strcase.TitleCase, ' ')
	return name, names
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
	var t T
	typ := reflect.TypeOf(t)
	// If T is a pointer, get the element type
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	fields := make(map[string]string)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		crudTag := field.Tag.Get(TAG_CRUD)
		if crudTag == "-" {
			continue
		}

		// Get the bun tag to get the column name
		bunTag := field.Tag.Get(TAG_BUN)
		var columnName string
		if bunTag != "" {
			parts := strings.Split(bunTag, ",")
			columnName = parts[0]
		} else {
			// Use the field name converted to snake_case
			columnName = strcase.ToSnake(field.Name)
		}

		jsonTag := field.Tag.Get(TAG_JSON)
		if jsonTag != "" {
			jsonTag = strings.Split(jsonTag, ",")[0] // remove options
		} else {
			jsonTag = strcase.ToSnake(field.Name)
		}

		fields[jsonTag] = columnName
	}
	return fields
}
