package crud

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fiber2 "github.com/gofiber/fiber/v2"
	fiber3 "github.com/gofiber/fiber/v3"
	goerrors "github.com/goliatone/go-errors"
	router "github.com/goliatone/go-router"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type adapterTestContextKey struct{}

type adapterFixture struct {
	routes  Router
	request func(*http.Request) (*http.Response, error)
	init    func()
}

// Each path uses its actual native middleware and controller registration.
func newAdapterFixture(name string) adapterFixture {
	seed := func() context.Context {
		return context.WithValue(context.Background(), adapterTestContextKey{}, "upstream")
	}
	if name == "native-v2" {
		app := fiber2.New()
		app.Use(func(c *fiber2.Ctx) error { c.SetUserContext(seed()); return c.Next() })
		return adapterFixture{NewFiberAdapter(app.Group("/api")), func(r *http.Request) (*http.Response, error) { return app.Test(r) }, func() {}}
	}
	if name == "native-v3" {
		app := fiber3.New()
		app.Use(func(c fiber3.Ctx) error { c.SetContext(seed()); return c.Next() })
		return adapterFixture{NewFiberV3Adapter(app.Group("/api")), func(r *http.Request) (*http.Response, error) { return app.Test(r) }, func() {}}
	}
	if name == "generic-v2" {
		srv := router.NewFiberAdapter(func(app *fiber2.App) *fiber2.App {
			app.Use(func(c *fiber2.Ctx) error { c.SetUserContext(seed()); return c.Next() })
			return app
		})
		return adapterFixture{NewGoRouterAdapter(srv.Router().Group("/api")), func(r *http.Request) (*http.Response, error) { return srv.WrappedRouter().Test(r) }, srv.Init}
	}
	srv := router.NewFiberV3Adapter(func(app *fiber3.App) *fiber3.App {
		app.Use(func(c fiber3.Ctx) error { c.SetContext(seed()); return c.Next() })
		return app
	})
	return adapterFixture{NewGoRouterAdapter(srv.Router().Group("/api")), func(r *http.Request) (*http.Response, error) { return srv.WrappedRouter().Test(r) }, srv.Init}
}

func TestFiberAdapterMatrixTranslation(t *testing.T) {
	for _, name := range []string{"native-v2", "native-v3", "generic-v2", "generic-v3"} {
		t.Run(name, func(t *testing.T) {
			f := newAdapterFixture(name)
			handler := func(c Context) error {
				// Avoid FailNow in Fiber's request goroutine.
				if c.Params("id") != "42" || c.Params("missing", "fallback") != "fallback" {
					t.Error("parameter translation")
				}
				if c.Query("empty", "fallback") != "fallback" || c.QueryInt("bad", 17) != 17 || c.QueryInt("n") != 9 {
					t.Error("query defaults")
				}
				if got := c.QueryValues("tag"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
					t.Errorf("repeated query: %v", got)
				}
				if c.UserContext().Value(adapterTestContextKey{}) != "upstream" {
					t.Error("upstream context lost")
				}
				if setter, ok := c.(userContextSetter); ok {
					setter.SetUserContext(ContextWithActor(c.UserContext(), ActorContext{ActorID: "actor"}))
				} else {
					t.Error("adapter does not support context updates")
				}
				if ActorFromContext(c.UserContext()).ActorID != "actor" {
					t.Error("context setter lost")
				}
				var input struct {
					Name string `json:"name" form:"name"`
				}
				if err := c.BodyParser(&input); err != nil {
					return err
				}
				headers, ok := c.(headerProvider)
				if !ok || input.Name != "Ada" || len(c.Body()) == 0 || headers.Header("X-Probe") != "present" {
					t.Error("body/header translation")
				}
				return c.Status(http.StatusAccepted).JSON(input, " application/vnd.crud+json ")
			}
			for _, register := range []func(string, func(Context) error) RouterRouteInfo{f.routes.Post, f.routes.Put, f.routes.Patch} {
				register("/record/:id", handler).Name("record")
			}
			f.routes.Get("/record/:id", func(c Context) error { return c.JSON(map[string]string{"id": c.Params("id")}) }).Name("read")
			f.routes.Delete("/record/:id", func(c Context) error { return c.SendStatus(http.StatusNoContent) }).Name("delete")
			f.init()
			for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
				for _, content := range []struct{ kind, body string }{{"application/json", `{"name":"Ada"}`}, {"application/x-www-form-urlencoded", "name=Ada"}} {
					req := httptest.NewRequestWithContext(t.Context(), method, "/api/record/42?tag=a&tag=b&empty=&bad=no&n=9", strings.NewReader(content.body))
					req.Header.Set("Content-Type", content.kind)
					req.Header.Set("X-Probe", "present")
					resp, err := f.request(req)
					require.NoError(t, err)
					require.Equal(t, http.StatusAccepted, resp.StatusCode)
					require.Equal(t, "application/vnd.crud+json", resp.Header.Get("Content-Type"))
					var data map[string]string
					require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
					require.NoError(t, resp.Body.Close())
					require.Equal(t, "Ada", data["name"])
				}
			}
			for _, probe := range []struct {
				method string
				status int
			}{{http.MethodGet, 200}, {http.MethodDelete, 204}} {
				resp, err := f.request(httptest.NewRequestWithContext(t.Context(), probe.method, "/api/record/42", nil))
				require.NoError(t, err)
				require.Equal(t, probe.status, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
			}
		})
	}
}

func TestFiberAdapterMatrixController(t *testing.T) {
	for _, name := range []string{"native-v2", "native-v3", "generic-v2", "generic-v3"} {
		t.Run(name, func(t *testing.T) {
			sqlDB, err := sql.Open("sqlite3", ":memory:")
			require.NoError(t, err)
			db := bun.NewDB(sqlDB, sqlitedialect.New())
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			require.NoError(t, createSchema(context.Background(), db))
			f := newAdapterFixture(name)
			writes := 0
			ctrl := NewController(newTestUserRepository(db),
				WithDeserializer(testUserDeserializer),
				WithScopeGuard[*TestUser](func(c Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
					if c.UserContext().Value(adapterTestContextKey{}) != "upstream" {
						t.Error("guard lost current context")
					}
					if headers, ok := c.(headerProvider); !ok {
						t.Error("adapter does not support headers")
					} else if headers.Header("X-Deny") == "yes" {
						return ActorContext{}, ScopeFilter{}, goerrors.New("denied", goerrors.CategoryAuthz).WithCode(http.StatusForbidden).WithTextCode("FORBIDDEN")
					}
					return ActorContext{ActorID: "actor", TenantID: "tenant"}, ScopeFilter{Labels: map[string]string{"tenant": "tenant"}}, nil
				}),
				WithServiceFuncs(ServiceFuncs[*TestUser]{Create: func(c Context, u *TestUser) (*TestUser, error) {
					if ActorFromContext(c.UserContext()).ActorID != "actor" || ScopeFromContext(c.UserContext()).Labels["tenant"] != "tenant" {
						t.Error("service lost actor/scope")
					}
					if RequestIDFromContext(c.UserContext()) != "req" || CorrelationIDFromContext(c.UserContext()) != "corr" {
						t.Error("service lost identifiers")
					}
					if u.Name == "" {
						return nil, goerrors.New("name required", goerrors.CategoryValidation).WithCode(http.StatusBadRequest).WithTextCode("VALIDATION_ERROR")
					}
					writes++
					return u, nil
				}}))
			ctrl.RegisterRoutes(f.routes)
			f.init()
			for _, probe := range []struct {
				body, deny     string
				status, writes int
				textCode       string
			}{
				{`{"name":"Ada","email":"ada@example.test"}`, "", 201, 1, ""},
				{`{"email":"missing@example.test"}`, "", 400, 1, "VALIDATION_ERROR"},
				{`{"name":"Denied"}`, "yes", 403, 1, "FORBIDDEN"},
			} {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/test-user", strings.NewReader(probe.body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Deny", probe.deny)
				req.Header.Set("X-Request-ID", "req")
				req.Header.Set("X-Correlation-ID", "corr")
				resp, err := f.request(req)
				require.NoError(t, err)
				require.Equal(t, probe.status, resp.StatusCode)
				if probe.textCode != "" {
					require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
					var payload goerrors.ErrorResponse
					require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
					require.NotNil(t, payload.Error)
					require.Equal(t, probe.textCode, payload.Error.TextCode)
				}
				require.NoError(t, resp.Body.Close())
				require.Equal(t, probe.writes, writes)
			}
		})
	}
}
