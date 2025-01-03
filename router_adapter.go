package crud

import (
	"context"
	"net/http"

	"github.com/goliatone/go-router"
)

// goRouterAdapter wraps a router.Router[T] to implement crud.Router
type goRouterAdapter[T any] struct {
	r router.Router[T]
}

// NewGoRouterAdapter creates a new crud.Router that uses a router.Router[T]
// This follows the same pattern as the existing NewFiberAdapter
func NewGoRouterAdapter[T any](r router.Router[T]) Router {
	return &goRouterAdapter[T]{r: r}
}

func (ra *goRouterAdapter[T]) Get(path string, handler func(Context) error) RouterRouteInfo {
	return &routerRouteInfoAdapter{ri: ra.r.Get(path, ra.wrap(handler))}
}

func (ra *goRouterAdapter[T]) Post(path string, handler func(Context) error) RouterRouteInfo {
	return &routerRouteInfoAdapter{ri: ra.r.Post(path, ra.wrap(handler))}
}

func (ra *goRouterAdapter[T]) Put(path string, handler func(Context) error) RouterRouteInfo {
	return &routerRouteInfoAdapter{ri: ra.r.Put(path, ra.wrap(handler))}
}

func (ra *goRouterAdapter[T]) Delete(path string, handler func(Context) error) RouterRouteInfo {
	return &routerRouteInfoAdapter{ri: ra.r.Delete(path, ra.wrap(handler))}
}

// wrap converts a crud.Context handler to a router.HandlerFunc
func (ra *goRouterAdapter[T]) wrap(h func(Context) error) router.HandlerFunc {
	return func(rc router.Context) error {
		return h(&contextAdapter{c: rc})
	}
}

// routerRouteInfoAdapter implements crud.RouterRouteInfo
type routerRouteInfoAdapter struct {
	ri router.RouteInfo
}

func (ria *routerRouteInfoAdapter) Name(n string) RouterRouteInfo {
	ria.ri.Name(n)
	return ria
}

// contextAdapter wraps a router.Context to implement crud.Context
type contextAdapter struct {
	c      router.Context
	status int
}

// Request interface implementation
func (ca *contextAdapter) UserContext() context.Context {
	return ca.c.Context()
}

func (ca *contextAdapter) Params(key string, defaultValue ...string) string {
	def := ""
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	return ca.c.Param(key, def)
}

func (ca *contextAdapter) Body() []byte {
	return ca.c.Body()
}

func (ca *contextAdapter) BodyParser(out interface{}) error {
	return ca.c.Bind(out)
}

func (ca *contextAdapter) Query(key string, defaultValue ...string) string {
	def := ""
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	return ca.c.Query(key, def)
}

func (ca *contextAdapter) QueryInt(key string, defaultValue ...int) int {
	def := 0
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	return ca.c.GetInt(key, def)
}

func (ca *contextAdapter) Queries() map[string]string {
	return ca.c.Queries()
}

// Response interface implementation
func (ca *contextAdapter) Status(status int) Response {
	ca.status = status
	ca.c.Status(status)
	return ca
}

func (ca *contextAdapter) JSON(data interface{}, ctype ...string) error {
	if ca.status == 0 {
		ca.status = http.StatusOK
	}
	return ca.c.JSON(ca.status, data)
}

func (ca *contextAdapter) SendStatus(status int) error {
	return ca.c.NoContent(status)
}
