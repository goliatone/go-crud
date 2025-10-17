package crud

import (
	"maps"
	"net/http"
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

type RouteOptions struct {
	Enabled *bool
	Method  string
}

type RouteConfig struct {
	Operations map[CrudOperation]RouteOptions
}

func DefaultRouteConfig() RouteConfig {
	return RouteConfig{}
}

func BoolPtr(v bool) *bool {
	return &v
}

func (rc RouteConfig) merge(other RouteConfig) RouteConfig {
	switch {
	case len(rc.Operations) == 0 && len(other.Operations) == 0:
		return RouteConfig{}
	case len(rc.Operations) == 0:
		return RouteConfig{Operations: cloneRouteOptionsMap(other.Operations)}
	case len(other.Operations) == 0:
		return RouteConfig{Operations: cloneRouteOptionsMap(rc.Operations)}
	default:
		merged := make(map[CrudOperation]RouteOptions, len(rc.Operations)+len(other.Operations))
		maps.Copy(merged, rc.Operations)
		maps.Copy(merged, other.Operations)
		return RouteConfig{Operations: merged}
	}
}

func cloneRouteOptionsMap(in map[CrudOperation]RouteOptions) map[CrudOperation]RouteOptions {
	if len(in) == 0 {
		return nil
	}
	out := make(map[CrudOperation]RouteOptions, len(in))
	maps.Copy(out, in)
	return out
}

func (rc RouteConfig) resolve(op CrudOperation, defaultMethod string) (bool, string) {
	method := defaultMethod
	enabled := true

	if len(method) == 0 {
		method = http.MethodGet
	}

	if rc.Operations != nil {
		if opt, ok := rc.Operations[op]; ok {
			if opt.Enabled != nil {
				enabled = *opt.Enabled
			}
			if opt.Method != "" {
				method = strings.ToUpper(opt.Method)
			}
		}
	}

	if method == "" {
		method = defaultMethod
	}

	if method == "" {
		method = http.MethodGet
	}

	return enabled, strings.ToUpper(method)
}

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

func WithQueryLogging[T any](enabled bool) Option[T] {
	return func(c *Controller[T]) {
		c.queryLoggingEnabled = enabled
	}
}

func WithFieldMapProvider[T any](provider FieldMapProvider) Option[T] {
	return func(c *Controller[T]) {
		c.fieldMapProvider = provider
	}
}

func WithRouteConfig[T any](config RouteConfig) Option[T] {
	return func(c *Controller[T]) {
		c.routeConfig = c.routeConfig.merge(config)
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
