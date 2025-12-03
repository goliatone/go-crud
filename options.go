package crud

import (
	"maps"
	"net/http"
	"reflect"
	"strings"

	"github.com/goliatone/go-crud/pkg/activity"
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

// WithErrorEncoder overrides the encoder used when controllers serialize errors.
// When paired with the default response handler, this switches between the
// problem+json encoder and the legacy {success:false,error:string} payloads.
func WithErrorEncoder[T any](encoder ErrorEncoder) Option[T] {
	return func(c *Controller[T]) {
		if encoder == nil {
			return
		}
		if c.resp == nil {
			c.resp = NewDefaultResponseHandler[T]()
		}
		if aware, ok := c.resp.(errorEncoderAware); ok {
			aware.setErrorEncoder(encoder)
			return
		}
		c.resp = &errorEncoderResponseHandler[T]{
			base:    c.resp,
			encoder: encoder,
		}
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

func WithService[T any](service Service[T]) Option[T] {
	return func(c *Controller[T]) {
		c.service = service
	}
}

// WithCommandService composes the default repository-backed service with the
// provided command adapter factory. When the factory is nil the option is a no-op.
func WithCommandService[T any](factory CommandServiceFactory[T]) Option[T] {
	return func(c *Controller[T]) {
		if factory == nil {
			return
		}
		base := c.service
		if base == nil {
			base = NewRepositoryService(c.Repo)
		}
		if wrapped := factory(base); wrapped != nil {
			c.service = wrapped
			return
		}
		c.service = base
	}
}

func WithServiceFuncs[T any](overrides ServiceFuncs[T]) Option[T] {
	return func(c *Controller[T]) {
		base := NewRepositoryService(c.Repo)
		c.service = ComposeService(base, overrides)
	}
}

func WithRouteConfig[T any](config RouteConfig) Option[T] {
	return func(c *Controller[T]) {
		c.routeConfig = c.routeConfig.merge(config)
	}
}

func WithLifecycleHooks[T any](hooks LifecycleHooks[T]) Option[T] {
	return func(c *Controller[T]) {
		c.hooks = hooks
	}
}

// WithActivityHooks configures the controller with the shared activity emitter
// built from pkg/activity hooks and config. Defaults to no-op when hooks are empty.
func WithActivityHooks[T any](hooks activity.Hooks, cfg activity.Config) Option[T] {
	return func(c *Controller[T]) {
		c.activityEmitterHooks = activity.NewEmitter(hooks, cfg)
	}
}

func WithNotificationEmitter[T any](emitter NotificationEmitter) Option[T] {
	return func(c *Controller[T]) {
		c.notificationEmitter = emitter
	}
}

func WithActions[T any](actions ...Action[T]) Option[T] {
	return func(c *Controller[T]) {
		if len(actions) == 0 {
			return
		}
		c.actions = append(c.actions, actions...)
	}
}

func WithAdminScopeMetadata[T any](meta AdminScopeMetadata) Option[T] {
	return func(c *Controller[T]) {
		c.adminScopeMetadata = meta
	}
}

func WithAdminMenuMetadata[T any](meta AdminMenuMetadata) Option[T] {
	return func(c *Controller[T]) {
		c.adminMenuMetadata = meta
	}
}

func WithRowFilterHints[T any](hints ...RowFilterHint) Option[T] {
	return func(c *Controller[T]) {
		c.rowFilterHints = cloneRowFilterHints(hints)
	}
}

func WithScopeGuard[T any](guard ScopeGuardFunc[T]) Option[T] {
	return func(c *Controller[T]) {
		c.scopeGuard = guard
	}
}

func WithRelationMetadataProvider[T any](provider router.RelationMetadataProvider) Option[T] {
	return func(c *Controller[T]) {
		c.relationProvider = provider
	}
}

func WithRelationFilter[T any](filter router.RelationFilterFunc) Option[T] {
	return func(c *Controller[T]) {
		if filter == nil {
			return
		}
		router.RegisterRelationFilter(filter)
		invalidateRelationDescriptorCache()
	}
}

func WithFieldPolicyProvider[T any](provider FieldPolicyProvider[T]) Option[T] {
	return func(c *Controller[T]) {
		c.fieldPolicyProvider = provider
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
