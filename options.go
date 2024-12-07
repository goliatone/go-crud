package crud

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/ettle/strcase"
	"github.com/gofiber/fiber/v2"
)

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

// Add a new option for setting the response handler
func WithResponseHandler[T any](handler ResponseHandler[T]) Option[T] {
	return func(c *Controller[T]) {
		c.resp = handler
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

const (
	TAG_CRUD = "crud"
	TAG_BUN  = "bun"
	TAG_JSON = "json"
)

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
			continue // skip this field
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
