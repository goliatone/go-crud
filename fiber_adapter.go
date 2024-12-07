package crud

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// fiberAdapter wraps a fiber.Router so that it satisfies crud.Router.
// This allows us to register routes in the crud.Controller.
type fiberAdapter struct {
	r fiber.Router
}

// NewFiberAdapter creates a new crud.Router that uses a router.Router[any]
func NewFiberAdapter(r fiber.Router) Router {
	return &fiberAdapter{r: r}
}

func (ra *fiberAdapter) Get(path string, handler func(Context) error) RouterRouteInfo {
	return &routeInfoAdapter{ri: ra.r.Get(path, ra.wrap(handler))}
}
func (ra *fiberAdapter) Post(path string, handler func(Context) error) RouterRouteInfo {
	return &routeInfoAdapter{ri: ra.r.Post(path, ra.wrap(handler))}
}
func (ra *fiberAdapter) Put(path string, handler func(Context) error) RouterRouteInfo {
	return &routeInfoAdapter{ri: ra.r.Put(path, ra.wrap(handler))}
}
func (ra *fiberAdapter) Delete(path string, handler func(Context) error) RouterRouteInfo {
	return &routeInfoAdapter{ri: ra.r.Delete(path, ra.wrap(handler))}
}

func (ra *fiberAdapter) wrap(h func(Context) error) func(*fiber.Ctx) error {
	return func(rc *fiber.Ctx) error {
		return h(&crudAdapter{c: rc})
	}
}

type routeInfoAdapter struct {
	ri fiber.Router
}

func (ria *routeInfoAdapter) Name(n string) RouterRouteInfo {
	ria.ri.Name(n)
	return ria
}

// crudAdapter wraps a router.Context to implement crud.Context
type crudAdapter struct {
	c          *fiber.Ctx
	statusCode int
}

func (ca *crudAdapter) UserContext() context.Context {
	return ca.c.UserContext()
}

func (ca *crudAdapter) Params(key string, defaultValue ...string) string {
	val := ca.c.Params(key)
	if val == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return val
}

func (ca *crudAdapter) BodyParser(out interface{}) error {
	return ca.c.BodyParser(out)
}

func (ca *crudAdapter) Query(key string, defaultValue ...string) string {
	val := ca.c.Query(key)
	if val == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return val
}

func (ca *crudAdapter) QueryInt(key string, defaultValue ...int) int {
	val := ca.Query(key)
	if val == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	i, err := strconv.Atoi(val)
	if err != nil && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return i
}

func (ca *crudAdapter) Queries() map[string]string {
	return ca.c.Queries()
}

func (ca *crudAdapter) Status(status int) Response {
	ca.statusCode = status
	ca.c.Status(status)
	return ca
}

func (ca *crudAdapter) JSON(data interface{}, ctype ...string) error {
	if ca.statusCode == 0 {
		ca.statusCode = http.StatusOK
	}
	ca.c.Status(ca.statusCode)
	return ca.c.JSON(data)
}

func (ca *crudAdapter) SendStatus(status int) error {
	ca.statusCode = status
	return ca.c.SendStatus(status)
}
