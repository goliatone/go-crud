package crud

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	goerrors "github.com/goliatone/go-errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

// Real HTTP complements app.Test coverage; the fixture owns only its listener.
// Application-managed drain and stream ownership belong to the host modules.
func TestFiberAdapterMatrixLiveCRUD(t *testing.T) {
	for _, name := range []string{"native-v2", "native-v3", "generic-v2", "generic-v3"} {
		t.Run(name, func(t *testing.T) {
			sqlDB, err := sql.Open("sqlite3", ":memory:")
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			db := bun.NewDB(sqlDB, sqlitedialect.New())
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			require.NoError(t, createSchema(t.Context(), db))
			f := newAdapterFixture(name)
			NewController(newTestUserRepository(db), WithDeserializer(testUserDeserializer),
				WithScopeGuard[*TestUser](func(c Context, _ CrudOperation) (ActorContext, ScopeFilter, error) {
					if c.UserContext().Value(adapterTestContextKey{}) != "upstream" {
						t.Error("native middleware context lost over HTTP")
					}
					if h, ok := c.(headerProvider); ok && h.Header("X-Deny") == "yes" {
						return ActorContext{}, ScopeFilter{}, goerrors.New("denied", goerrors.CategoryAuthz).WithCode(http.StatusForbidden).WithTextCode("FORBIDDEN")
					}
					return ActorContext{ActorID: "actor"}, ScopeFilter{}, nil
				})).RegisterRoutes(f.routes)
			f.init()

			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() { done <- f.listen(ln) }()
			transport := &http.Transport{DisableKeepAlives: true}
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			t.Cleanup(func() {
				transport.CloseIdleConnections()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := f.shutdown(ctx); err != nil {
					t.Errorf("fixture shutdown: %v", err)
				}
				_ = ln.Close()
				select {
				case err := <-done:
					if err != nil {
						t.Errorf("fixture listener: %v", err)
					}
				case <-ctx.Done():
					t.Error("fixture listener did not join")
				}
			})
			request := func(method, path, body string, deny bool, status int) ([]byte, http.Header) {
				t.Helper()
				req, err := http.NewRequestWithContext(t.Context(), method, "http://"+ln.Addr().String()+"/api"+path, strings.NewReader(body))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				if deny {
					req.Header.Set("X-Deny", "yes")
				}
				resp, err := client.Do(req)
				require.NoError(t, err)
				data, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				require.Equal(t, status, resp.StatusCode, string(data))
				return data, resp.Header
			}

			data, _ := request(http.MethodPost, "/test-user", `{"name":"Ada","email":"ada@example.test","password":"secret"}`, false, http.StatusCreated)
			var created TestUser
			require.NoError(t, json.Unmarshal(data, &created))
			require.NotEqual(t, uuid.Nil, created.ID)
			require.Equal(t, "Ada", created.Name)
			require.NotContains(t, string(data), "secret")
			data, _ = request(http.MethodGet, "/test-user/"+created.ID.String(), "", false, http.StatusOK)
			var read APIResponse[TestUser]
			require.NoError(t, json.Unmarshal(data, &read))
			require.Equal(t, created.ID, read.Data.ID)
			require.Equal(t, "Ada", read.Data.Name)

			data, headers := request(http.MethodPost, "/test-user", `{"name":"Denied","email":"denied@example.test"}`, true, http.StatusForbidden)
			require.Equal(t, "application/problem+json", headers.Get("Content-Type"))
			var problem goerrors.ErrorResponse
			require.NoError(t, json.Unmarshal(data, &problem))
			require.NotNil(t, problem.Error)
			require.Equal(t, "FORBIDDEN", problem.Error.TextCode)
			count, err := db.NewSelect().Model((*TestUser)(nil)).Count(t.Context())
			require.NoError(t, err)
			require.Equal(t, 1, count, "denied request must not write")

			data, _ = request(http.MethodGet, "/test-user/schema", "", false, http.StatusOK)
			var schema map[string]any
			require.NoError(t, json.Unmarshal(data, &schema))
			require.NotEmpty(t, schema["openapi"])
			require.NotEmpty(t, schema["paths"])
			request(http.MethodDelete, "/test-user/"+created.ID.String(), "", false, http.StatusNoContent)
			request(http.MethodGet, "/test-user/"+created.ID.String(), "", false, http.StatusNotFound)
		})
	}
}
