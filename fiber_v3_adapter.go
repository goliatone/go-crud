package crud

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// fiberV3Adapter wraps a fiber.Router so that it satisfies crud.Router.
// This allows us to register routes in the crud.Controller.
type fiberV3Adapter struct {
	r fiber.Router
}

// NewFiberV3Adapter adapts a native Fiber v3 app or group to the CRUD router contract.
func NewFiberV3Adapter(r fiber.Router) Router {
	return &fiberV3Adapter{r: r}
}

func (ra *fiberV3Adapter) Get(path string, handler func(Context) error) RouterRouteInfo {
	return &fiberV3RouteInfoAdapter{ri: ra.r.Get(path, ra.wrap(handler))}
}

func (ra *fiberV3Adapter) Post(path string, handler func(Context) error) RouterRouteInfo {
	return &fiberV3RouteInfoAdapter{ri: ra.r.Post(path, ra.wrap(handler))}
}

func (ra *fiberV3Adapter) Put(path string, handler func(Context) error) RouterRouteInfo {
	return &fiberV3RouteInfoAdapter{ri: ra.r.Put(path, ra.wrap(handler))}
}

func (ra *fiberV3Adapter) Patch(path string, handler func(Context) error) RouterRouteInfo {
	return &fiberV3RouteInfoAdapter{ri: ra.r.Patch(path, ra.wrap(handler))}
}

func (ra *fiberV3Adapter) Delete(path string, handler func(Context) error) RouterRouteInfo {
	return &fiberV3RouteInfoAdapter{ri: ra.r.Delete(path, ra.wrap(handler))}
}

func (ra *fiberV3Adapter) wrap(h func(Context) error) func(fiber.Ctx) error {
	return func(rc fiber.Ctx) error {
		return h(&fiberV3ContextAdapter{c: rc})
	}
}

type fiberV3RouteInfoAdapter struct {
	ri fiber.Router
}

func (ria *fiberV3RouteInfoAdapter) Name(n string) RouterRouteInfo {
	ria.ri.Name(n)
	return ria
}

// fiberV3ContextAdapter translates the native v3 request and response contracts.
type fiberV3ContextAdapter struct {
	c          fiber.Ctx
	statusCode int
}

func (ca *fiberV3ContextAdapter) UserContext() context.Context {
	return ca.c.Context()
}

func (ca *fiberV3ContextAdapter) SetUserContext(ctx context.Context) {
	ca.c.SetContext(ctx)
}

func (ca *fiberV3ContextAdapter) Params(key string, defaultValue ...string) string {
	val := ca.c.Params(key)
	if val == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return val
}

func (ca *fiberV3ContextAdapter) Body() []byte {
	return ca.c.Body()
}

func (ca *fiberV3ContextAdapter) BodyParser(out any) error {
	return ca.c.Bind().Body(out)
}

func (ca *fiberV3ContextAdapter) Query(key string, defaultValue ...string) string {
	val := ca.c.Query(key)
	if val == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return val
}

func (ca *fiberV3ContextAdapter) QueryValues(key string) []string {
	args := ca.c.Request().URI().QueryArgs()
	values := []string{}
	for k, v := range args.All() {
		if string(k) == key {
			values = append(values, string(v))
		}
	}
	return values
}

func (ca *fiberV3ContextAdapter) QueryInt(key string, defaultValue ...int) int {
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

func (ca *fiberV3ContextAdapter) Queries() map[string]string {
	return ca.c.Queries()
}

func (ca *fiberV3ContextAdapter) Header(key string) string {
	return ca.c.Get(key)
}

func (ca *fiberV3ContextAdapter) Status(status int) Response {
	ca.statusCode = status
	ca.c.Status(status)
	return ca
}

func (ca *fiberV3ContextAdapter) JSON(data any, ctype ...string) error {
	if ca.statusCode == 0 {
		ca.statusCode = http.StatusOK
	}
	var contentType string
	if len(ctype) > 0 {
		contentType = strings.TrimSpace(ctype[0])
	}
	ca.c.Status(ca.statusCode)
	if err := ca.c.JSON(data); err != nil {
		return err
	}
	if contentType != "" {
		ca.c.Set(fiber.HeaderContentType, contentType)
	}
	return nil
}

func (ca *fiberV3ContextAdapter) SendStatus(status int) error {
	ca.statusCode = status
	return ca.c.SendStatus(status)
}

var _ Router = (*fiberV3Adapter)(nil)
var _ Context = (*fiberV3ContextAdapter)(nil)
