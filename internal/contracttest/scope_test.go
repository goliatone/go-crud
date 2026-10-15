package contracttest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	crud "github.com/goliatone/go-crud"
	goerrors "github.com/goliatone/go-errors"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type ScopedResource struct {
	bun.BaseModel `bun:"table:scope_resources,alias:r"`
	ID            uuid.UUID `bun:"id,pk" json:"id"`
	GroupID       string    `bun:"group_id" json:"group_id"`
	Value         string    `bun:"value" json:"value"`
}

type queryCounter struct{ count atomic.Int64 }

func (h *queryCounter) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	h.count.Add(1)
	return ctx
}
func (*queryCounter) AfterQuery(context.Context, *bun.QueryEvent) {}

type scopeFixture struct {
	db      *bun.DB
	repo    repository.Repository[*ScopedResource]
	a, b    *ScopedResource
	queries *queryCounter
}

func newScopeFixture(t *testing.T, required bool) *scopeFixture {
	t.Helper()
	sqldb, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	f := &scopeFixture{db: db, queries: &queryCounter{},
		a: &ScopedResource{ID: uuid.New(), GroupID: "A", Value: "opaque-a"},
		b: &ScopedResource{ID: uuid.New(), GroupID: "B", Value: "opaque-b"},
	}
	db.AddQueryHook(f.queries)
	_, err = db.NewCreateTable().Model((*ScopedResource)(nil)).Exec(context.Background())
	require.NoError(t, err)
	f.repo = repository.NewRepository(db, repository.ModelHandlers[*ScopedResource]{
		NewRecord: func() *ScopedResource { return &ScopedResource{} },
		GetID:     func(r *ScopedResource) uuid.UUID { return r.ID },
		SetID:     func(r *ScopedResource, id uuid.UUID) { r.ID = id },
	})
	_, err = f.repo.CreateMany(context.Background(), []*ScopedResource{f.a, f.b})
	require.NoError(t, err)
	if required {
		f.repo.RegisterScope("group", repository.ScopeByFieldRequired("group", "group_id"))
		require.NoError(t, f.repo.SetScopeDefaults(repository.ScopeDefaults{
			Select: []string{"group"}, Update: []string{"group"}, Delete: []string{"group"},
		}))
	}
	return f
}

// This is synthetic host policy, not an IAM implementation: selected Group must
// match the actor's configured membership, and missing actor/scope fails closed.
func configuredGuard(c crud.Context, _ crud.CrudOperation) (crud.ActorContext, crud.ScopeFilter, error) {
	headers, ok := c.(interface{ Header(string) string })
	actor, group := "", ""
	if ok {
		actor, group = headers.Header("X-Actor"), headers.Header("X-Group")
	}
	if group == "" || (actor != "user-a" || group != "A") && (actor != "user-b" || group != "B") {
		return crud.ActorContext{}, crud.ScopeFilter{}, goerrors.New("scope denied", goerrors.CategoryAuthz).WithCode(http.StatusForbidden)
	}
	scope := crud.ScopeFilter{}
	scope.AddColumnFilter("group_id", "=", group)
	return crud.ActorContext{ActorID: actor}, scope, nil
}

func (f *scopeFixture) app() *fiber.App {
	app := fiber.New()
	crud.NewController(f.repo, crud.WithScopeGuard[*ScopedResource](configuredGuard)).RegisterRoutes(crud.NewFiberAdapter(app))
	return app
}

func scopeRequest(t *testing.T, app *fiber.App, method, path, actor, group string, body any) *http.Response {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor", actor)
	req.Header.Set("X-Group", group)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	return resp
}

func (f *scopeFixture) unchanged(t *testing.T) {
	t.Helper()
	var rows []*ScopedResource
	require.NoError(t, f.db.NewSelect().Model(&rows).Order("group_id ASC").Scan(context.Background()))
	require.Len(t, rows, 2)
	require.Equal(t, "A", rows[0].GroupID)
	require.Equal(t, "opaque-a", rows[0].Value)
	require.Equal(t, "B", rows[1].GroupID)
	require.Equal(t, "opaque-b", rows[1].Value)
}

func TestConfiguredCRUDScope(t *testing.T) {
	for _, name := range []string{"own_read", "cross_read", "own_list", "cross_filter", "own_update", "cross_update", "own_bulk_update", "mixed_bulk_update", "cross_delete", "mixed_bulk_delete"} {
		t.Run(name, func(t *testing.T) {
			f := newScopeFixture(t, false) // prove the controller's guard boundary independently
			app := f.app()
			method, path, status := "GET", "/scoped-resource/"+f.b.ID.String(), http.StatusNotFound
			var body any
			switch name {
			case "own_read":
				path, status = "/scoped-resource/"+f.a.ID.String(), http.StatusOK
			case "own_list":
				path, status = "/scoped-resources?limit=1", http.StatusOK
			case "cross_filter":
				path, status = "/scoped-resources?group_id=B", http.StatusOK
			case "own_update":
				method, path, status, body = "PUT", "/scoped-resource/"+f.a.ID.String(), http.StatusOK, map[string]any{"value": "changed"}
			case "cross_update":
				method, body = "PUT", map[string]any{"value": "tampered"}
			case "own_bulk_update":
				method, path, status, body = "PUT", "/scoped-resource/batch", http.StatusOK, []map[string]any{{"id": f.a.ID, "value": "changed"}}
			case "mixed_bulk_update":
				method, path, body = "PUT", "/scoped-resource/batch", []map[string]any{{"id": f.a.ID, "value": "tampered"}, {"id": f.b.ID, "value": "tampered"}}
			case "cross_delete":
				method = "DELETE"
			case "mixed_bulk_delete":
				method, path, body = "DELETE", "/scoped-resource/batch", []string{f.a.ID.String(), f.b.ID.String()}
			}
			resp := scopeRequest(t, app, method, path, "user-a", "A", body)
			if name == "mixed_bulk_delete" {
				f.unchanged(t)
			}
			require.Equal(t, status, resp.StatusCode)
			if name == "own_list" || name == "cross_filter" {
				var result crud.APIListResponse[*ScopedResource]
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
				want := 1
				if name == "cross_filter" {
					want = 0
				}
				require.Len(t, result.Data, want)
				require.NotNil(t, result.Meta)
				require.Equal(t, want, result.Meta.Count)
				for _, row := range result.Data {
					require.Equal(t, "A", row.GroupID)
				}
			}
			if name == "own_update" || name == "own_bulk_update" {
				var value string
				require.NoError(t, f.db.QueryRow("SELECT value FROM scope_resources WHERE id = ?", f.a.ID).Scan(&value))
				require.Equal(t, "changed", value)
			} else {
				f.unchanged(t)
			}
		})
	}
}

func TestConfiguredCRUDGuardFirst(t *testing.T) {
	for _, identity := range []struct{ name, actor, group string }{
		{"missing_scope", "user-a", ""}, {"missing_actor", "", "A"}, {"forged_scope", "user-a", "B"},
	} {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			for _, bulk := range []bool{false, true} {
				t.Run(identity.name+"/"+method+map[bool]string{false: "/single", true: "/bulk"}[bulk], func(t *testing.T) {
					f := newScopeFixture(t, false)
					app := f.app()
					path := "/scoped-resource/" + f.b.ID.String()
					if bulk {
						path = "/scoped-resource/batch"
						if method == "GET" {
							path = "/scoped-resources?name__unknown=x"
						}
					}
					before := f.queries.count.Load()
					resp := scopeRequest(t, app, method, path, identity.actor, identity.group, "invalid body shape")
					require.Equal(t, http.StatusForbidden, resp.StatusCode)
					require.Equal(t, before, f.queries.count.Load(), "denied guard must precede SQL and decoding")
					f.unchanged(t)
				})
			}
		}
	}
}

func TestRequiredRepositoryScope(t *testing.T) {
	for _, group := range []string{"A", "", "missing"} {
		t.Run("group_"+group, func(t *testing.T) {
			f := newScopeFixture(t, true)
			ctx := context.Background()
			if group != "" {
				ctx = repository.WithScopeData(ctx, "group", group)
			}
			rows, count, err := f.repo.List(ctx)
			require.NoError(t, err)
			want := 0
			if group == "A" {
				want = 1
			}
			require.Len(t, rows, want)
			require.Equal(t, want, count)
			_, err = f.repo.GetByID(ctx, f.b.ID.String())
			require.Error(t, err)
			if group == "A" {
				row, err := f.repo.GetByID(ctx, f.a.ID.String())
				require.NoError(t, err)
				require.Equal(t, f.a.ID, row.ID)
			}
			copyB := *f.b
			copyB.Value = "tampered"
			_, err = f.repo.Update(ctx, &copyB)
			require.Error(t, err)
			// Bulk update may succeed with zero affected rows. The SQL invariant,
			// rather than the return value, is authoritative for this API.
			_, err = f.repo.UpdateMany(ctx, []*ScopedResource{&copyB})
			require.NoError(t, err)
			require.NoError(t, f.repo.DeleteWhere(ctx, repository.DeleteByIDs([]string{f.b.ID.String()})))
			f.unchanged(t)
		})
	}
}

func TestUnconfiguredRepositoryIsNotAuthorization(t *testing.T) {
	f := newScopeFixture(t, false)
	row, err := f.repo.GetByID(context.Background(), f.b.ID.String())
	require.NoError(t, err)
	require.Equal(t, "B", row.GroupID)
	// Required SELECT/UPDATE/DELETE scopes do not stamp ownership on insertion.
	f = newScopeFixture(t, true)
	row, err = f.repo.Create(repository.WithScopeData(context.Background(), "group", "A"), &ScopedResource{ID: uuid.New(), GroupID: "B", Value: "opaque-new"})
	require.NoError(t, err)
	require.Equal(t, "B", row.GroupID)
}

func TestFilteredDeleteBatch(t *testing.T) {
	for _, policy := range []bool{false, true} {
		for _, shape := range []string{"ids", "records"} {
			for _, allowed := range []bool{false, true} {
				name := map[bool]string{false: "guard", true: "field_policy"}[policy] + "/" + shape + "/" + map[bool]string{false: "mixed", true: "own"}[allowed]
				t.Run(name, func(t *testing.T) {
					f := newScopeFixture(t, false)
					app := fiber.New()
					options := []crud.Option[*ScopedResource]{crud.WithScopeGuard[*ScopedResource](configuredGuard)}
					if policy {
						options = []crud.Option[*ScopedResource]{crud.WithFieldPolicyProvider[*ScopedResource](func(crud.FieldPolicyRequest[*ScopedResource]) (crud.FieldPolicy, error) {
							scope := crud.ScopeFilter{}
							scope.AddColumnFilter("group_id", "=", "A")
							return crud.FieldPolicy{RowFilter: scope}, nil
						})}
					}
					crud.NewController(f.repo, options...).RegisterRoutes(crud.NewFiberAdapter(app))
					ids, records := []string{f.a.ID.String()}, []*ScopedResource{f.a}
					if !allowed {
						ids = append(ids, f.b.ID.String())
						records = append(records, f.b)
					}
					var body any = ids
					if shape == "records" {
						body = records
					}
					resp := scopeRequest(t, app, "DELETE", "/scoped-resource/batch", "user-a", "A", body)
					if !allowed {
						require.Equal(t, http.StatusNotFound, resp.StatusCode)
						f.unchanged(t)
						return
					}
					require.Equal(t, http.StatusNoContent, resp.StatusCode)
					var rows []*ScopedResource
					require.NoError(t, f.db.NewSelect().Model(&rows).Scan(context.Background()))
					require.Len(t, rows, 1)
					require.Equal(t, f.b.ID, rows[0].ID)
				})
			}
		}
	}
}

func TestUnfilteredDeleteBatchMissingIDRemainsIdempotent(t *testing.T) {
	f := newScopeFixture(t, false)
	app := fiber.New()
	crud.NewController(f.repo).RegisterRoutes(crud.NewFiberAdapter(app))
	resp := scopeRequest(t, app, "DELETE", "/scoped-resource/batch", "", "", []string{uuid.NewString()})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	f.unchanged(t)
}
