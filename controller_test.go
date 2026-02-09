package crud

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	goerrors "github.com/goliatone/go-errors"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/extra/bundebug"

	"github.com/goliatone/go-crud/pkg/activity"
	"github.com/goliatone/go-repository-bun"
)

type TestUserProfile struct {
	bun.BaseModel `bun:"table:test_user_profiles,alias:p"`

	ID        uuid.UUID `bun:"id,pk,notnull" json:"id"`
	UserID    uuid.UUID `bun:"user_id,notnull" json:"user_id"`
	Bio       string    `bun:"bio" json:"bio"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

type TestUser struct {
	bun.BaseModel `bun:"table:test_users,alias:u"`

	ID        uuid.UUID          `bun:"id,pk,notnull" json:"id"`
	Name      string             `bun:"name,notnull" json:"name" crud:"label"`
	Email     string             `bun:"email,notnull,unique" json:"email"`
	Age       int                `bun:"age" json:"age"`
	Password  string             `bun:"password,notnull" json:"-" crud:"-"`
	CreatedAt time.Time          `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time          `bun:"updated_at,notnull" json:"updated_at"`
	Profiles  []*TestUserProfile `bun:"rel:has-many,join:id=user_id" json:"profiles,omitempty"`
}

type optionResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type testNotificationEmitter struct {
	events []NotificationEvent
	ctxs   []context.Context
}

func (e *testNotificationEmitter) SendNotification(ctx context.Context, event NotificationEvent) error {
	e.ctxs = append(e.ctxs, ctx)
	e.events = append(e.events, event)
	return nil
}

func newTestUserRepository(db *bun.DB) repository.Repository[*TestUser] {
	handlers := repository.ModelHandlers[*TestUser]{
		NewRecord: func() *TestUser {
			return &TestUser{}
		},
		GetID: func(record *TestUser) uuid.UUID {
			return record.ID
		},
		SetID: func(record *TestUser, id uuid.UUID) {
			record.ID = id
		},
		GetIdentifier: func() string {
			return "Email"
		},
	}
	return repository.NewRepository(db, handlers)
}

func testUserDeserializer(op CrudOperation, ctx Context) (*TestUser, error) {
	var user TestUser
	if err := ctx.BodyParser(&user); err != nil {
		return nil, err
	}
	// Additional validation can be added here
	return &user, nil
}

func setupApp(t *testing.T, options ...Option[*TestUser]) (*fiber.App, *bun.DB) {
	// Initialize the Fiber app
	app := fiber.New()

	// Set up the database (in-memory SQLite for testing)
	// Use shared cache to ensure all connections see the same database
	sqldb, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	if os.Getenv("TEST_SQL_DEBUG") != "" {
		db.AddQueryHook(bundebug.NewQueryHook(
			bundebug.WithVerbose(true),
		))
	}

	// Create tables
	ctx := context.Background()
	if err := createSchema(ctx, db); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Initialize the repository and controller
	repo := newTestUserRepository(db)
	opts := append([]Option[*TestUser]{WithDeserializer(testUserDeserializer)}, options...)
	controller := NewController[*TestUser](repo, opts...)

	// Register routes
	router := NewFiberAdapter(app)
	controller.RegisterRoutes(router)

	return app, db
}

func setupAppWithHooks(t *testing.T, hooks LifecycleHooks[*TestUser]) (*fiber.App, repository.Repository[*TestUser], *bun.DB) {
	app := fiber.New()

	sqldb, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	if os.Getenv("TEST_SQL_DEBUG") != "" {
		db.AddQueryHook(bundebug.NewQueryHook(
			bundebug.WithVerbose(true),
		))
	}

	ctx := context.Background()
	if err := createSchema(ctx, db); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	repo := newTestUserRepository(db)
	controller := NewController[*TestUser](repo,
		WithDeserializer(testUserDeserializer),
		WithLifecycleHooks[*TestUser](hooks),
	)

	router := NewFiberAdapter(app)
	controller.RegisterRoutes(router)

	return app, repo, db
}

func createSchema(ctx context.Context, db *bun.DB) error {
	models := []any{
		(*TestUser)(nil),
		(*TestUserProfile)(nil),
	}

	for _, model := range models {
		if _, err := db.NewCreateTable().Model(model).IfNotExists().Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func insertTestUsers(t *testing.T, db *bun.DB, users ...*TestUser) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, user := range users {
		if user.ID == uuid.Nil {
			user.ID = uuid.New()
		}
		if user.CreatedAt.IsZero() {
			user.CreatedAt = now
		}
		if user.UpdatedAt.IsZero() {
			user.UpdatedAt = now
		}
		if user.Email == "" {
			user.Email = fmt.Sprintf("user-%s@example.com", user.ID.String())
		}
		_, err := db.NewInsert().Model(user).Exec(ctx)
		require.NoError(t, err)
	}
}

func TestController_Schema_ReturnsOpenAPIDocument(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/test-user/schema", nil)
	req.Header.Set("Accept", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.Equal(t, "3.0.3", doc["openapi"])

	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok, "paths section missing")
	_, hasListPath := paths["/test-users"]
	assert.True(t, hasListPath, "expected list path in OpenAPI document")

	components, ok := doc["components"].(map[string]any)
	require.True(t, ok, "components section missing")

	schemas, ok := components["schemas"].(map[string]any)
	require.True(t, ok, "schemas section missing")

	entitySchema, ok := schemas["test-user"].(map[string]any)
	require.True(t, ok, "test-user schema missing")
	assert.Equal(t, "object", entitySchema["type"])

	labelField, ok := entitySchema["x-formgen-label-field"].(string)
	require.True(t, ok, "expected x-formgen-label-field extension")
	assert.Equal(t, "name", labelField)

	requiredFields, ok := entitySchema["required"].([]any)
	require.True(t, ok, "required list missing")
	assert.Contains(t, requiredFields, "id")
	assert.Contains(t, requiredFields, "email")

	props, ok := entitySchema["properties"].(map[string]any)
	require.True(t, ok, "properties map missing")

	idProp, ok := props["id"].(map[string]any)
	require.True(t, ok, "id property missing")
	assert.Equal(t, "string", idProp["type"])

	relationExt, ok := entitySchema["x-formgen-relations"].(map[string]any)
	require.True(t, ok, "expected x-formgen-relations extension")

	includes, includesOK := relationExt["includes"].([]any)
	require.True(t, includesOK, "expected includes slice in relation metadata")
	assert.Contains(t, includes, "profiles")

	tree, treeOK := relationExt["tree"].(map[string]any)
	require.True(t, treeOK, "expected relation tree metadata")
	assert.Equal(t, "testuser", tree["name"])

	componentsParams, ok := components["parameters"].(map[string]any)
	require.True(t, ok, "expected shared parameter components")

	checkNumberParam := func(name, description string, expectedDefault float64) {
		param, ok := componentsParams[name].(map[string]any)
		require.Truef(t, ok, "expected %s parameter component", name)
		assert.Equalf(t, strings.ToLower(name), param["name"], "unexpected %s name", name)
		assert.Equalf(t, "query", param["in"], "unexpected %s location", name)
		assert.Equalf(t, description, param["description"], "unexpected %s description", name)

		schema, ok := param["schema"].(map[string]any)
		require.Truef(t, ok, "expected %s schema definition", name)
		assert.Equalf(t, "integer", schema["type"], "unexpected %s schema type", name)
		assert.Equalf(t, expectedDefault, schema["default"], "unexpected %s default", name)
	}

	checkStringParam := func(name, description string) {
		param, ok := componentsParams[name].(map[string]any)
		require.Truef(t, ok, "expected %s parameter component", name)
		assert.Equalf(t, strings.ToLower(name), param["name"], "unexpected %s name", name)
		assert.Equalf(t, "query", param["in"], "unexpected %s location", name)
		assert.Equalf(t, description, param["description"], "unexpected %s description", name)

		schema, ok := param["schema"].(map[string]any)
		require.Truef(t, ok, "expected %s schema definition", name)
		assert.Equalf(t, "string", schema["type"], "unexpected %s schema type", name)
	}

	checkNumberParam("Limit", "Maximum number of records to return (default 25)", 25)
	checkNumberParam("Offset", "Number of records to skip before starting to return results (default 0)", 0)
	checkStringParam("Include", "Related resources to include, comma separated (e.g. Company,Profile)")
	checkStringParam("Select", "Fields to include in the response, comma separated (e.g. id,name,email)")
	checkStringParam("Order", "Sort order, comma separated with direction (e.g. name asc,created_at desc)")

	listPath, ok := paths["/test-users"].(map[string]any)
	require.True(t, ok, "expected list path metadata")

	getOperation, ok := listPath["get"].(map[string]any)
	require.True(t, ok, "expected GET operation metadata for list path")

	rawParams, ok := getOperation["parameters"].([]any)
	require.True(t, ok, "expected parameters array on list GET operation")

	expectedRefs := map[string]bool{
		"#/components/parameters/Limit":   false,
		"#/components/parameters/Offset":  false,
		"#/components/parameters/Include": false,
		"#/components/parameters/Select":  false,
		"#/components/parameters/Order":   false,
	}

	for _, p := range rawParams {
		param, ok := p.(map[string]any)
		require.True(t, ok, "unexpected parameter value in GET operation")
		if ref, ok := param["$ref"].(string); ok {
			if _, exists := expectedRefs[ref]; exists {
				expectedRefs[ref] = true
			}
		}
	}

	for ref, seen := range expectedRefs {
		assert.Truef(t, seen, "expected to find parameter reference %s on GET operation", ref)
	}
}

func TestController_SchemaIncludesAdminExtensions(t *testing.T) {
	resetSchemaRegistry()

	action := Action[*TestUser]{
		Name:   "Deactivate",
		Method: http.MethodPost,
		Target: ActionTargetResource,
		Handler: func(actx ActionContext[*TestUser]) error {
			return actx.Status(http.StatusAccepted).JSON(fiber.Map{"ok": true})
		},
	}

	scope := AdminScopeMetadata{
		Level:       "tenant",
		Description: "Requires tenant scope",
		Claims:      []string{"users:write"},
	}
	menu := AdminMenuMetadata{
		Group: "Directory",
		Label: "Test Users",
		Icon:  "user",
		Order: 10,
	}
	rowFilters := []RowFilterHint{
		{Field: "tenant_id", Operator: "=", Description: "Matches actor tenant"},
	}

	app, db := setupApp(t,
		WithActions(action),
		WithAdminScopeMetadata[*TestUser](scope),
		WithAdminMenuMetadata[*TestUser](menu),
		WithRowFilterHints[*TestUser](rowFilters...),
	)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/test-user/schema", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var doc map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&doc))

	components := doc["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)

	var schema map[string]any
	for _, value := range schemas {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, hasActions := entry["x-admin-actions"]; hasActions {
			schema = entry
			break
		}
	}
	require.NotNil(t, schema, "expected schema with x-admin-actions")

	scopeExt, ok := schema["x-admin-scope"].(map[string]any)
	require.True(t, ok, "expected x-admin-scope extension")
	assert.Equal(t, "tenant", scopeExt["level"])
	assert.Contains(t, scopeExt["claims"], "users:write")

	menuExt, ok := schema["x-admin-menu"].(map[string]any)
	require.True(t, ok, "expected x-admin-menu extension")
	assert.Equal(t, "Directory", menuExt["group"])
	assert.Equal(t, float64(10), menuExt["order"])

	rowExt, ok := schema["x-admin-row-filters"].([]any)
	require.True(t, ok, "expected x-admin-row-filters extension")
	require.Len(t, rowExt, 1)

	actions, ok := schema["x-admin-actions"].([]any)
	require.True(t, ok, "expected x-admin-actions array")
	require.Len(t, actions, 1)
	firstAction, ok := actions[0].(map[string]any)
	require.True(t, ok, "action descriptor should be an object")
	assert.Equal(t, "deactivate", firstAction["slug"])
	assert.Equal(t, "/test-user/:id/actions/deactivate", firstAction["path"])
}

func TestSchemaRegistryStoresEntries(t *testing.T) {
	resetSchemaRegistry()

	app, db := setupApp(t)
	defer db.Close()
	_ = app

	entries := ListSchemas()
	require.NotEmpty(t, entries, "expected registry to contain schema entries")

	found := false
	for _, entry := range entries {
		if entry.Resource == "test-user" {
			found = true
			assert.NotNil(t, entry.Document["openapi"])
		}
	}
	assert.True(t, found, "expected test-user schema to be registered")
}

func TestController_RelationMetadataMatchesSchema(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	ctx := context.Background()
	repo := newTestUserRepository(db)
	user := &TestUser{
		ID:        uuid.New(),
		Name:      "Schema Relation User",
		Email:     "relation.user@example.com",
		Password:  "secret",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := repo.Create(ctx, user)
	require.NoError(t, err)

	profile := &TestUserProfile{
		ID:        uuid.New(),
		UserID:    user.ID,
		Bio:       "Remote",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err = db.NewInsert().Model(profile).Exec(ctx)
	require.NoError(t, err)

	schemaReq := httptest.NewRequest(http.MethodGet, "/test-user/schema", nil)
	schemaResp, err := app.Test(schemaReq, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, schemaResp.StatusCode)

	var schemaDoc map[string]any
	require.NoError(t, json.NewDecoder(schemaResp.Body).Decode(&schemaDoc))

	components := schemaDoc["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	entitySchema := schemas["test-user"].(map[string]any)
	relationExt := entitySchema["x-formgen-relations"].(map[string]any)

	includesAny, includesOK := relationExt["includes"].([]any)
	require.True(t, includesOK)
	includeSet := make(map[string]struct{}, len(includesAny))
	for _, v := range includesAny {
		if s, ok := v.(string); ok {
			includeSet[strings.ToLower(s)] = struct{}{}
		}
	}

	relationsAny, relationsOK := relationExt["relations"].([]any)
	schemaRelations := make(map[string][]RelationFilter, len(relationsAny))
	if relationsOK {
		for _, rel := range relationsAny {
			relMap, ok := rel.(map[string]any)
			if !ok {
				continue
			}
			name, _ := relMap["name"].(string)
			filtersAny, _ := relMap["filters"].([]any)
			var filters []RelationFilter
			for _, filter := range filtersAny {
				if fm, ok := filter.(map[string]any); ok {
					filters = append(filters, RelationFilter{
						Field:    stringify(fm["field"]),
						Operator: stringify(fm["operator"]),
						Value:    stringify(fm["value"]),
					})
				}
			}
			if name != "" {
				schemaRelations[strings.ToLower(name)] = filters
			}
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/test-users?include=profiles.bio__eq=Remote", nil)
	listResp, err := app.Test(listReq, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var listResponse APIListResponse[TestUser]
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listResponse))
	require.NotNil(t, listResponse.Meta)
	require.NotEmpty(t, listResponse.Meta.Include)

	for _, relation := range listResponse.Meta.Relations {
		_, ok := includeSet[strings.ToLower(relation.Name)]
		assert.Truef(t, ok, "relation %s missing from schema includes", relation.Name)

		expectedFilters, hasSchemaFilters := schemaRelations[strings.ToLower(relation.Name)]
		if hasSchemaFilters && len(expectedFilters) > 0 {
			assert.Equal(t, expectedFilters, relation.Filters)
		}
	}
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func printBody(t *testing.T, resp *http.Response) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	fmt.Printf("Response Body: %s\n", string(bodyBytes))
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
}

func TestController_GetUser_NotFound(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	req := httptest.NewRequest("GET", fmt.Sprintf("/test-user/%s", uuid.New().String()), nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json")

	var response goerrors.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if assert.NotNil(t, response.Error) {
		assert.Equal(t, goerrors.CategoryNotFound, response.Error.Category)
		assert.Equal(t, http.StatusNotFound, response.Error.Code)
		assert.Equal(t, "NOT_FOUND", response.Error.TextCode)
		assert.NotEmpty(t, response.Error.Message)
		if assert.NotNil(t, response.Error.Metadata) {
			assert.Equal(t, string(OpRead), response.Error.Metadata["operation"])
		}
	}
}

func TestController_GetUser_NotFound_LegacyEncoder(t *testing.T) {
	app, db := setupApp(t, WithErrorEncoder[*TestUser](LegacyJSONErrorEncoder()))
	defer db.Close()

	req := httptest.NewRequest("GET", fmt.Sprintf("/test-user/%s", uuid.New().String()), nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var legacy APIResponse[TestUser]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&legacy))
	assert.False(t, legacy.Success)
	assert.NotEmpty(t, legacy.Error)
}

func TestController_CreateUser(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	user := map[string]any{
		"name":     "John Doe",
		"email":    "john.doe@example.com",
		"password": "secret",
	}

	body, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("Failed to marshal user: %v", err)
	}

	req := httptest.NewRequest("POST", "/test-user", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdUser TestUser
	if err := json.NewDecoder(resp.Body).Decode(&createdUser); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.Equal(t, user["name"], createdUser.Name)
	assert.Equal(t, user["email"], createdUser.Email)
	assert.Empty(t, createdUser.Password)
	assert.NotEmpty(t, createdUser.ID)
}

func TestController_Create_DelegatesToService(t *testing.T) {
	var called bool

	opt := func(ctrl *Controller[*TestUser]) {
		base := NewRepositoryService(ctrl.Repo)
		overrides := ServiceFuncs[*TestUser]{
			Create: func(ctx Context, record *TestUser) (*TestUser, error) {
				called = true
				record.Name = record.Name + " via service"
				return base.Create(ctx, record)
			},
		}
		ctrl.service = ComposeService(base, overrides)
	}

	app, db := setupApp(t, opt)
	defer db.Close()

	user := map[string]any{
		"name":     "Service User",
		"email":    "service.user@example.com",
		"password": "secret",
	}

	body, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("Failed to marshal user: %v", err)
	}

	req := httptest.NewRequest("POST", "/test-user", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.True(t, called, "expected service override to be invoked")

	var createdUser TestUser
	if err := json.NewDecoder(resp.Body).Decode(&createdUser); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.Equal(t, "Service User via service", createdUser.Name)

	repo := newTestUserRepository(db)
	saved, err := repo.GetByID(context.Background(), createdUser.ID.String())
	if err != nil {
		t.Fatalf("Failed to fetch created user: %v", err)
	}
	assert.Equal(t, "Service User via service", saved.Name)
}

func TestController_GetUser(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	// Create a user in the database
	ctx := context.Background()
	repo := newTestUserRepository(db)
	user := &TestUser{
		ID:        uuid.New(),
		Name:      "Jane Doe",
		Email:     "jane.doe@example.com",
		Password:  "secret",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := repo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/test-user/%s", user.ID.String()), nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response APIResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, response.Success)
	assert.Equal(t, user.Name, response.Data.Name)
	assert.Equal(t, user.Email, response.Data.Email)
	assert.Empty(t, response.Data.Password)
}

func TestController_ListUsers(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	// Create multiple users
	ctx := context.Background()
	repo := newTestUserRepository(db)
	for i := 1; i <= 5; i++ {
		user := &TestUser{
			ID:        uuid.New(),
			Name:      fmt.Sprintf("User %d", i),
			Email:     fmt.Sprintf("user%d@example.com", i),
			Password:  "secret",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_, err := repo.Create(ctx, user)
		if err != nil {
			t.Fatalf("Failed to create user %d: %v", i, err)
		}
	}

	req := httptest.NewRequest("GET", "/test-users", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	// Debug: Print response body if status is not OK
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Response status: %d, body: %s", resp.StatusCode, string(body))
		resp.Body = io.NopCloser(bytes.NewReader(body)) // Reset body for further reading
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response APIListResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, response.Success)
	assert.Len(t, response.Data, 5)
	if assert.NotNil(t, response.Meta) {
		assert.Equal(t, 5, response.Meta.Count)
	}

	for _, user := range response.Data {
		assert.NotEmpty(t, user.Name)
		assert.NotEmpty(t, user.Email)
		assert.Empty(t, user.Password)
	}
}

func TestController_ListUsers_FormatOptions(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	ctx := context.Background()
	repo := newTestUserRepository(db)
	expected := make(map[string]string)

	for i := 1; i <= 3; i++ {
		user := &TestUser{
			ID:        uuid.New(),
			Name:      fmt.Sprintf("Option %d", i),
			Email:     fmt.Sprintf("option%d@example.com", i),
			Password:  "secret",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if _, err := repo.Create(ctx, user); err != nil {
			t.Fatalf("Failed to create user %d: %v", i, err)
		}
		expected[user.ID.String()] = user.Name
	}

	req := httptest.NewRequest("GET", "/test-users?format=options", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var options []optionResponse
	if err := json.NewDecoder(resp.Body).Decode(&options); err != nil {
		t.Fatalf("Failed to decode options response: %v", err)
	}

	assert.Len(t, options, len(expected))
	for _, opt := range options {
		label, ok := expected[opt.Value]
		assert.True(t, ok, "unexpected option value %s", opt.Value)
		assert.Equal(t, label, opt.Label)
	}
}

func TestController_Index_DelegatesToService(t *testing.T) {
	var called bool

	opt := func(ctrl *Controller[*TestUser]) {
		base := NewRepositoryService(ctrl.Repo)
		overrides := ServiceFuncs[*TestUser]{
			Index: func(ctx Context, criteria []repository.SelectCriteria) ([]*TestUser, int, error) {
				called = true
				return base.Index(ctx, criteria)
			},
		}
		ctrl.service = ComposeService(base, overrides)
	}

	app, db := setupApp(t, opt)
	defer db.Close()

	ctx := context.Background()
	repo := newTestUserRepository(db)
	user := &TestUser{
		ID:        uuid.New(),
		Name:      "Index User",
		Email:     "index.user@example.com",
		Password:  "secret",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if _, err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	req := httptest.NewRequest("GET", "/test-users", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, called, "expected service override to be invoked for index")
}

func TestController_CreateBatch_FormatOptions(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	now := time.Now().UTC()
	payload := []map[string]any{
		{
			"id":         uuid.New().String(),
			"name":       "Batch Option 1",
			"email":      "batch-option-1@example.com",
			"created_at": now.Format(time.RFC3339),
			"updated_at": now.Format(time.RFC3339),
		},
		{
			"id":         uuid.New().String(),
			"name":       "Batch Option 2",
			"email":      "batch-option-2@example.com",
			"created_at": now.Format(time.RFC3339),
			"updated_at": now.Format(time.RFC3339),
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/test-user/batch?format=options", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var options []optionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&options))
	assert.Len(t, options, len(payload))

	expected := make(map[string]string, len(payload))
	for _, item := range payload {
		id := item["id"].(string)
		name := item["name"].(string)
		expected[id] = name
	}

	for _, opt := range options {
		label, ok := expected[opt.Value]
		assert.True(t, ok, "unexpected option value %s", opt.Value)
		assert.Equal(t, label, opt.Label)
	}
}

func TestController_DeleteBatch_AcceptsIDArray(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	ctx := context.Background()
	repo := newTestUserRepository(db)
	user1 := &TestUser{
		ID:        uuid.New(),
		Name:      "Delete Batch 1",
		Email:     "delete-batch-1@example.com",
		Password:  "secret",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	user2 := &TestUser{
		ID:        uuid.New(),
		Name:      "Delete Batch 2",
		Email:     "delete-batch-2@example.com",
		Password:  "secret",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := repo.Create(ctx, user1)
	require.NoError(t, err)
	_, err = repo.Create(ctx, user2)
	require.NoError(t, err)

	ids := []string{user1.ID.String(), user2.ID.String()}
	body, err := json.Marshal(ids)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/test-user/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	_, err = repo.GetByID(ctx, user1.ID.String())
	assert.Error(t, err)
	_, err = repo.GetByID(ctx, user2.ID.String())
	assert.Error(t, err)
}

func TestController_UpdateUser(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	// Create a user
	ctx := context.Background()
	repo := newTestUserRepository(db)
	user := &TestUser{
		ID:        uuid.New(),
		Name:      "Old Name",
		Email:     "update@example.com",
		Password:  "secret",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := repo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Prepare update data
	updatedData := map[string]any{
		"name": "New Name",
	}
	body, err := json.Marshal(updatedData)
	if err != nil {
		t.Fatalf("Failed to marshal updated data: %v", err)
	}

	req := httptest.NewRequest("PUT", fmt.Sprintf("/test-user/%s", user.ID.String()), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response APIResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, response.Success)
	assert.Equal(t, "New Name", response.Data.Name)
	assert.Equal(t, user.Email, response.Data.Email)
}

func TestController_DeleteUser(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	// Create a user
	ctx := context.Background()
	repo := newTestUserRepository(db)
	user := &TestUser{
		ID:        uuid.New(),
		Name:      "To Be Deleted",
		Email:     "delete@example.com",
		Password:  "secret",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := repo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/test-user/%s", user.ID.String()), nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify that the user is deleted
	_, err = repo.GetByID(ctx, user.ID.String())
	assert.Error(t, err)
}

func TestController_ListUsers_NoFilters(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	// Create users with different names and emails
	ctx := context.Background()
	repo := newTestUserRepository(db)
	users := []TestUser{
		{ID: uuid.New(), Name: "Alice", Email: "alice@example.com", Password: "secret", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: uuid.New(), Name: "Bob", Email: "bob@example.com", Password: "secret", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: uuid.New(), Name: "Charlie", Email: "charlie@example.com", Password: "secret", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	for i := range users {
		_, err := repo.Create(ctx, &users[i])
		if err != nil {
			t.Fatalf("Failed to create user %s: %v", users[i].Name, err)
		}
	}

	// List all users without filters
	req := httptest.NewRequest("GET", "/test-users", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response APIListResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, response.Success)
	assert.Len(t, response.Data, 3)
	if assert.NotNil(t, response.Meta) {
		assert.Equal(t, 3, response.Meta.Count)
	}

	// Optional: Verify each user exists
	expectedNames := []string{"Alice", "Bob", "Charlie"}
	for _, name := range expectedNames {
		found := false
		for _, user := range response.Data {
			if user.Name == name {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected user %s to be present", name)
	}
}

func TestController_ListUsers_WithFilters(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	// Create users with different names and emails
	ctx := context.Background()
	repo := newTestUserRepository(db)
	users := []TestUser{
		{ID: uuid.New(), Name: "Alice", Email: "alice@example.com", Password: "secret", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: uuid.New(), Name: "Bob", Email: "bob@example.com", Password: "secret", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: uuid.New(), Name: "Charlie", Email: "charlie@example.com", Password: "secret", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	for i := range users {
		_, err := repo.Create(ctx, &users[i])
		if err != nil {
			t.Fatalf("Failed to create user %s: %v", users[i].Name, err)
		}
	}

	// Intermediate check: List all users without filters
	reqAll := httptest.NewRequest("GET", "/test-users", nil)
	reqAll.Header.Set("Content-Type", "application/json")

	respAll, err := app.Test(reqAll, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	// Debug: Print response body if status is not OK
	if respAll.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respAll.Body)
		t.Logf("Response status: %d, body: %s", respAll.StatusCode, string(body))
		respAll.Body = io.NopCloser(bytes.NewReader(body)) // Reset body for further reading
	}

	assert.Equal(t, http.StatusOK, respAll.StatusCode)

	var allUsersResponse APIListResponse[TestUser]
	if err := json.NewDecoder(respAll.Body).Decode(&allUsersResponse); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, allUsersResponse.Success)
	assert.Len(t, allUsersResponse.Data, 3)
	if assert.NotNil(t, allUsersResponse.Meta) {
		assert.Equal(t, 3, allUsersResponse.Meta.Count)
	}

	// Now, filter users where name is 'Bob'
	req := httptest.NewRequest("GET", "/test-users?name=Bob", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	// Debug: Print response body if status is not OK
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Response status: %d, body: %s", resp.StatusCode, string(body))
		resp.Body = io.NopCloser(bytes.NewReader(body)) // Reset body for further reading
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var filteredResponse APIListResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&filteredResponse); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, filteredResponse.Success)
	if assert.Len(t, filteredResponse.Data, 1) {
		assert.Equal(t, "Bob", filteredResponse.Data[0].Name)
	}
	if assert.NotNil(t, filteredResponse.Meta) {
		assert.Equal(t, 1, filteredResponse.Meta.Count)
	}
}

func TestController_ListUsers_WithExhaustiveFilters(t *testing.T) {
	SetOperatorMap(map[string]string{
		"eq":    "=",
		"ne":    "<>",
		"gt":    ">",
		"lt":    "<",
		"gte":   ">=",
		"lte":   "<=",
		"ilike": "LIKE", // Adjusted for SQLite
		"like":  "LIKE",
		"and":   "and",
		"or":    "or",
	})
	defer SetOperatorMap(DefaultOperatorMap())

	app, db := setupApp(t)
	defer db.Close()

	ctx := context.Background()
	repo := newTestUserRepository(db)

	baseTime := time.Now().Truncate(time.Second)
	t.Logf("Base time: %v", baseTime)

	users := []TestUser{
		{
			ID:        uuid.New(),
			Name:      "Alice",
			Email:     "alice@example.com",
			Password:  "secret",
			Age:       30,
			CreatedAt: baseTime.Add(-48 * time.Hour),
			UpdatedAt: baseTime.Add(-24 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Bob",
			Email:     "bob@example.com",
			Password:  "secret",
			Age:       25,
			CreatedAt: baseTime.Add(-72 * time.Hour),
			UpdatedAt: baseTime.Add(-36 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Charlie",
			Email:     "charlie@sample.com",
			Password:  "secret",
			Age:       35,
			CreatedAt: baseTime.Add(-23 * time.Hour), // Just inside 24hr window
			UpdatedAt: baseTime.Add(-12 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "David",
			Email:     "david@example.com",
			Password:  "secret",
			Age:       40,
			CreatedAt: baseTime.Add(-96 * time.Hour),
			UpdatedAt: baseTime.Add(-48 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Eve",
			Email:     "eve@sample.com",
			Password:  "secret",
			Age:       28,
			CreatedAt: baseTime.Add(-12 * time.Hour),
			UpdatedAt: baseTime.Add(-6 * time.Hour),
		},
	}

	t.Log("User creation times (UTC):")
	for _, u := range users {
		t.Logf("%s: %v", u.Name, u.CreatedAt.UTC())
	}

	for i := range users {
		_, err := repo.Create(ctx, &users[i])
		if err != nil {
			t.Fatalf("Failed to create user %s: %v", users[i].Name, err)
		}
	}

	// Define test cases
	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedNames  []string
		expectedStatus int
	}{
		{
			name:           "Filter by name equals 'Bob'",
			query:          "?name=Bob",
			expectedCount:  1,
			expectedNames:  []string{"Bob"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by email ilike 'example.com'",
			query:          "?email__ilike=%example.com%",
			expectedCount:  3, // Alice, Bob, David
			expectedNames:  []string{"Alice", "Bob", "David"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by age greater than 30",
			query:          "?age__gt=30",
			expectedCount:  2, // Charlie (35), David (40)
			expectedNames:  []string{"Charlie", "David"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by age greater than or equal to 30",
			query:          "?age__gte=30",
			expectedCount:  3, // Alice (30), Charlie (35), David (40)
			expectedNames:  []string{"Alice", "Charlie", "David"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by age less than 30",
			query:          "?age__lt=30",
			expectedCount:  2, // Bob (25), Eve (28)
			expectedNames:  []string{"Bob", "Eve"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by age less than or equal to 30",
			query:          "?age__lte=30",
			expectedCount:  3, // Alice (30), Bob (25), Eve (28)
			expectedNames:  []string{"Alice", "Bob", "Eve"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by name like 'A%'",
			query:          "?name__like=A%",
			expectedCount:  1, // Alice
			expectedNames:  []string{"Alice"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by email not equal 'eve@sample.com'",
			query:          "?email__ne=eve@sample.com",
			expectedCount:  4, // All except Eve
			expectedNames:  []string{"Alice", "Bob", "Charlie", "David"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter with AND operator on age between 25 and 35",
			query:          "?age__gte=25&age__lte=35",
			expectedCount:  4, // Alice (30), Bob (25), Charlie (35), Eve (28)
			expectedNames:  []string{"Alice", "Bob", "Charlie", "Eve"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter with OR operator on name",
			query:          "?name__or=Alice,Charlie",
			expectedCount:  2, // Alice and Charlie
			expectedNames:  []string{"Alice", "Charlie"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Multiple filters: age >=30 and email ilike 'example.com'",
			query:          "?age__gte=30&email__ilike=%example.com%",
			expectedCount:  2, // Alice (30, example.com), David (40, example.com)
			expectedNames:  []string{"Alice", "David"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Multiple filters with OR: name__or=Alice,Bob&age__lt=35",
			query:          "?name__or=Alice,Bob&age__lt=35",
			expectedCount:  2, // Alice (30), Bob (25)
			expectedNames:  []string{"Alice", "Bob"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by created_at greater than specific time",
			query:          "", // Will set dynamically
			expectedCount:  2,  // Charlie and Eve
			expectedNames:  []string{"Charlie", "Eve"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unknown operator defaults to equals",
			query:          "?name__unknown=Eve",
			expectedCount:  1, // Eve
			expectedNames:  []string{"Eve"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Empty operator defaults to equals",
			query:          "?name__=Alice",
			expectedCount:  1, // Alice
			expectedNames:  []string{"Alice"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter with non-existing field",
			query:          "?nonexistent=foo",
			expectedCount:  5, // No filter applied
			expectedNames:  []string{"Alice", "Bob", "Charlie", "David", "Eve"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter with multiple operators on same field",
			query:          "?age__gte=25&age__lte=35",
			expectedCount:  4, // Alice (30), Bob (25), Charlie (35), Eve (28)
			expectedNames:  []string{"Alice", "Bob", "Charlie", "Eve"},
			expectedStatus: http.StatusOK,
		},
	}

	for i, tt := range tests {
		if tt.name == "Filter by created_at greater than specific time" {
			timeThreshold := baseTime.Add(-24 * time.Hour).UTC()
			t.Logf("Time threshold (UTC): %v", timeThreshold)

			// Format time for SQLite using standard format
			timeStr := timeThreshold.Format("2006-01-02 15:04:05")
			tests[i].query = "?created_at__gte=" + url.QueryEscape(timeStr)

			t.Logf("Query string: %s", tests[i].query)
			t.Logf("Looking for records after: %s", timeStr)

			tests[i].expectedCount = 2 // Charlie and Eve
			tests[i].expectedNames = []string{"Charlie", "Eve"}
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/test-users" + tt.query
			req := httptest.NewRequest("GET", url, nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("Failed to perform request: %v", err)
			}

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			var response APIListResponse[TestUser]
			if resp.StatusCode == http.StatusOK {
				if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				assert.True(t, response.Success)
				assert.Equal(t, tt.expectedCount, len(response.Data))
				assert.Equal(t, tt.expectedCount, response.Meta.Count)

				var names []string
				for _, user := range response.Data {
					names = append(names, user.Name)
				}

				assert.ElementsMatch(t, tt.expectedNames, names)
			} else {
				var errorResponse goerrors.ErrorResponse
				if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}
				if assert.NotNil(t, errorResponse.Error) {
					assert.NotEmpty(t, errorResponse.Error.Message)
				}
				t.Fatalf("Unexpected error response: %+v", errorResponse)
			}
		})
	}

	// Edge case tests
	t.Run("Filter with empty value", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test-users?name__eq=", nil)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("Failed to perform request: %v", err)
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response APIListResponse[TestUser]
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		assert.True(t, response.Success)
		// TODO: Maybe this should be assert.Len(t, response.Data, 0)
		assert.Len(t, response.Data, 5)
		if assert.NotNil(t, response.Meta) {
			assert.Equal(t, 5, response.Meta.Count)
		}
	})

	t.Run("Filter with multiple OR operators on different fields", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test-users?name__or=Alice,Bob&email__or=charlie@sample.com", nil)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("Failed to perform request: %v", err)
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response APIListResponse[TestUser]
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		assert.True(t, response.Success)
		expectedNames := []string{"Alice", "Bob", "Charlie"}
		assert.Equal(t, 3, len(response.Data))
		if assert.NotNil(t, response.Meta) {
			assert.Equal(t, 3, response.Meta.Count)
		}
		for _, user := range response.Data {
			assert.Contains(t, expectedNames, user.Name)
		}
	})

	t.Run("Filter with invalid UUID in parameter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test-users?id__eq=invalid-uuid", nil)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("Failed to perform request: %v", err)
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response APIListResponse[TestUser]
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		assert.True(t, response.Success)
		assert.Len(t, response.Data, 0)
		if assert.NotNil(t, response.Meta) {
			assert.Equal(t, 0, response.Meta.Count)
		}
	})

	t.Run("Filter with JSON injection attempt", func(t *testing.T) {
		injection := "Bobby Tables'; DROP TABLE users;--"
		encodedInjection := url.QueryEscape(injection)

		reqURL := "/test-users?name__or=" + encodedInjection
		req := httptest.NewRequest("GET", reqURL, nil)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("Failed to perform request: %v", err)
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response APIListResponse[TestUser]
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		assert.True(t, response.Success)
		assert.Len(t, response.Data, 0)
		if assert.NotNil(t, response.Meta) {
			assert.Equal(t, 0, response.Meta.Count)
		}
	})
}

func TestController_ListUsers_WithPagination(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	// Create 30 users
	ctx := context.Background()
	repo := newTestUserRepository(db)
	for i := 1; i <= 30; i++ {
		user := &TestUser{
			ID:        uuid.New(),
			Name:      fmt.Sprintf("User %d", i),
			Email:     fmt.Sprintf("user%d@example.com", i),
			Password:  "secret",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_, err := repo.Create(ctx, user)
		if err != nil {
			t.Fatalf("Failed to create user %d: %v", i, err)
		}
	}

	// Request with limit and offset
	req := httptest.NewRequest("GET", "/test-users?limit=10&offset=10", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResponse APIListResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&listResponse); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.Len(t, listResponse.Data, 10)
	assert.Equal(t, "User 11", listResponse.Data[0].Name)
	assert.Equal(t, "User 20", listResponse.Data[9].Name)
}

func TestController_ListUsers_AdjustsOutOfRangeOffset(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	ctx := context.Background()
	repo := newTestUserRepository(db)
	for i := 1; i <= 6; i++ {
		user := &TestUser{
			ID:        uuid.New(),
			Name:      fmt.Sprintf("User %d", i),
			Email:     fmt.Sprintf("user%d@example.com", i),
			Password:  "secret",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_, err := repo.Create(ctx, user)
		if err != nil {
			t.Fatalf("Failed to create user %d: %v", i, err)
		}
	}

	req := httptest.NewRequest("GET", "/test-users?limit=10&offset=20", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response APIListResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, response.Success)
	assert.Len(t, response.Data, 6)
	if assert.NotNil(t, response.Meta) {
		assert.Equal(t, 6, response.Meta.Count)
		assert.Equal(t, 10, response.Meta.Limit)
		assert.Equal(t, 0, response.Meta.Offset)
		assert.Equal(t, 1, response.Meta.Page)
		assert.True(t, response.Meta.Adjusted)
	}
}

func TestController_ListUsers_AdjustsOffsetWhenEmpty(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	req := httptest.NewRequest("GET", "/test-users?limit=10&offset=20", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response APIListResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, response.Success)
	assert.Len(t, response.Data, 0)
	if assert.NotNil(t, response.Meta) {
		assert.Equal(t, 0, response.Meta.Count)
		assert.Equal(t, 10, response.Meta.Limit)
		assert.Equal(t, 0, response.Meta.Offset)
		assert.Equal(t, 1, response.Meta.Page)
		assert.True(t, response.Meta.Adjusted)
	}
}

func TestController_UnauthorizedFieldAccess(t *testing.T) {
	app, db := setupApp(t)
	defer db.Close()

	// Attempt to filter by 'password' field, which is not allowed
	req := httptest.NewRequest("GET", "/test-users?password=secret", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResponse APIListResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&listResponse); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Since 'password' is not an allowed field, the filter should not be applied
	// Assuming there are no users, the list should be empty
	assert.Len(t, listResponse.Data, 0)
}

type OrderItem struct{}
type Person struct{}
type Category struct{}
type ModelWithTag struct {
	bun.BaseModel `bun:"table:companies,alias:cmp" crud:"resource:tag"`
}

func TestGetResourceName(t *testing.T) {
	testCases := []struct {
		name             string
		expectedSingular string
		expectedPlural   string
	}{
		{
			name:             "TestUser",
			expectedSingular: "test-user",
			expectedPlural:   "test-users",
		},
		{
			name:             "OrderItem",
			expectedSingular: "order-item",
			expectedPlural:   "order-items",
		},
		{
			name:             "Person",
			expectedSingular: "person",
			expectedPlural:   "people",
		},
		{
			name:             "Category",
			expectedSingular: "category",
			expectedPlural:   "categories",
		},
		{
			name:             "ModelWithTag",
			expectedSingular: "tag",
			expectedPlural:   "tags",
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // Optional: Run tests in parallel

			switch tc.name {
			case "TestUser":
				singular, plural := GetResourceName(reflect.TypeOf(TestUser{}))
				assert.Equal(t, tc.expectedSingular, singular)
				assert.Equal(t, tc.expectedPlural, plural)
			case "OrderItem":
				singular, plural := GetResourceName(reflect.TypeOf(OrderItem{}))
				assert.Equal(t, tc.expectedSingular, singular)
				assert.Equal(t, tc.expectedPlural, plural)
			case "Person":
				singular, plural := GetResourceName(reflect.TypeOf(Person{}))
				assert.Equal(t, tc.expectedSingular, singular)
				assert.Equal(t, tc.expectedPlural, plural)
			case "Category":
				singular, plural := GetResourceName(reflect.TypeOf(Category{}))
				assert.Equal(t, tc.expectedSingular, singular)
				assert.Equal(t, tc.expectedPlural, plural)
			case "ModelWithTag":
				singular, plural := GetResourceName(reflect.TypeOf(ModelWithTag{}))
				assert.Equal(t, tc.expectedSingular, singular)
				assert.Equal(t, tc.expectedPlural, plural)
			default:
				t.Fatalf("Unknown test case: %s", tc.name)
			}
		})
	}
}

func TestRegisterRoutes(t *testing.T) {
	app := fiber.New()
	router := NewFiberAdapter(app)

	sqldb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqldb.Close()

	db := bun.NewDB(sqldb, sqlitedialect.New())

	ctx := context.Background()
	if err := createSchema(ctx, db); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	repo := newTestUserRepository(db)
	controller := NewController(repo, WithDeserializer(testUserDeserializer))

	controller.RegisterRoutes(router)

	singular, plural := GetResourceName(reflect.TypeOf(TestUser{}))

	// Expected routes
	expectedRoutes := []struct {
		Name   string
		Method string
		Path   string
	}{
		{
			Name:   fmt.Sprintf("%s:%s", singular, OpRead),
			Method: "GET",
			Path:   fmt.Sprintf("/%s/:id", singular),
		},
		{
			Name:   fmt.Sprintf("%s:%s", singular, OpList),
			Method: "GET",
			Path:   fmt.Sprintf("/%s", plural),
		},
		{
			Name:   fmt.Sprintf("%s:%s", singular, OpCreate),
			Method: "POST",
			Path:   fmt.Sprintf("/%s", singular),
		},
		{
			Name:   fmt.Sprintf("%s:%s", singular, OpUpdate),
			Method: "PUT",
			Path:   fmt.Sprintf("/%s/:id", singular),
		},
		{
			Name:   fmt.Sprintf("%s:%s", singular, OpDelete),
			Method: "DELETE",
			Path:   fmt.Sprintf("/%s/:id", singular),
		},
	}

	for _, expected := range expectedRoutes {
		route := app.GetRoute(expected.Name)
		if assert.NotNil(t, route, "Route %s should be registered", expected.Name) {
			assert.Equal(t, expected.Method, route.Method, "Method for route %s should be %s", expected.Name, expected.Method)
			assert.Equal(t, expected.Path, route.Path, "Path for route %s should be %s", expected.Name, expected.Path)
		} else {
			t.Errorf("Route %s not found", expected.Name)
		}
	}
}

func TestRegisterRoutesWithBatchRouteSegment(t *testing.T) {
	app := fiber.New()
	router := NewFiberAdapter(app)

	sqldb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqldb.Close()

	db := bun.NewDB(sqldb, sqlitedialect.New())

	ctx := context.Background()
	if err := createSchema(ctx, db); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	repo := newTestUserRepository(db)
	controller := NewController(
		repo,
		WithDeserializer(testUserDeserializer),
		WithBatchRouteSegment[*TestUser]("bulk"),
	)

	controller.RegisterRoutes(router)

	singular, _ := GetResourceName(reflect.TypeOf(TestUser{}))

	expectedRoutes := []struct {
		Name   string
		Method string
		Path   string
	}{
		{
			Name:   fmt.Sprintf("%s:%s", singular, OpCreateBatch),
			Method: "POST",
			Path:   fmt.Sprintf("/%s/bulk", singular),
		},
		{
			Name:   fmt.Sprintf("%s:%s", singular, OpUpdateBatch),
			Method: "PUT",
			Path:   fmt.Sprintf("/%s/bulk", singular),
		},
		{
			Name:   fmt.Sprintf("%s:%s", singular, OpDeleteBatch),
			Method: "DELETE",
			Path:   fmt.Sprintf("/%s/bulk", singular),
		},
	}

	for _, expected := range expectedRoutes {
		route := app.GetRoute(expected.Name)
		if assert.NotNil(t, route, "Route %s should be registered", expected.Name) {
			assert.Equal(t, expected.Method, route.Method, "Method for route %s should be %s", expected.Name, expected.Method)
			assert.Equal(t, expected.Path, route.Path, "Path for route %s should be %s", expected.Name, expected.Path)
		} else {
			t.Errorf("Route %s not found", expected.Name)
		}
	}
}

func TestRegisterRoutesWithDisabledOperation(t *testing.T) {
	app := fiber.New()
	router := NewFiberAdapter(app)

	sqldb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqldb.Close()

	db := bun.NewDB(sqldb, sqlitedialect.New())

	ctx := context.Background()
	if err := createSchema(ctx, db); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	repo := newTestUserRepository(db)
	controller := NewController(
		repo,
		WithDeserializer(testUserDeserializer),
		WithRouteConfig[*TestUser](RouteConfig{
			Operations: map[CrudOperation]RouteOptions{
				OpDelete:      {Enabled: BoolPtr(false)},
				OpDeleteBatch: {Enabled: BoolPtr(false)},
			},
		}),
	)

	controller.RegisterRoutes(router)

	singular, _ := GetResourceName(reflect.TypeOf(TestUser{}))

	deleteRoute := fmt.Sprintf("%s:%s", singular, OpDelete)
	assert.False(t, fiberRouteExists(app, deleteRoute), "delete route should not be registered when disabled")

	deleteBatchRoute := fmt.Sprintf("%s:%s", singular, OpDeleteBatch)
	assert.False(t, fiberRouteExists(app, deleteBatchRoute), "delete batch route should not be registered when disabled")
}

func TestRegisterRoutesWithMethodOverride(t *testing.T) {
	app := fiber.New()
	router := NewFiberAdapter(app)

	sqldb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer sqldb.Close()

	db := bun.NewDB(sqldb, sqlitedialect.New())

	ctx := context.Background()
	if err := createSchema(ctx, db); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	repo := newTestUserRepository(db)
	controller := NewController(
		repo,
		WithDeserializer(testUserDeserializer),
		WithRouteConfig[*TestUser](RouteConfig{
			Operations: map[CrudOperation]RouteOptions{
				OpUpdate: {Method: http.MethodPatch},
			},
		}),
	)

	controller.RegisterRoutes(router)

	singular, _ := GetResourceName(reflect.TypeOf(TestUser{}))

	updateRoute := fmt.Sprintf("%s:%s", singular, OpUpdate)
	route := app.GetRoute(updateRoute)
	if assert.NotNil(t, route, "update route should be registered") {
		assert.Equal(t, http.MethodPatch, route.Method, "update route should use overridden method")
	}
}

func fiberRouteExists(app *fiber.App, name string) bool {
	for _, route := range app.GetRoutes() {
		if route.Name == name {
			return true
		}
	}
	return false
}

func TestLifecycleHooks_Create(t *testing.T) {
	var beforeCalled, afterCalled bool
	var beforeMeta, afterMeta HookMetadata

	hooks := LifecycleHooks[*TestUser]{
		BeforeCreate: []HookFunc[*TestUser]{func(hctx HookContext, user *TestUser) error {
			beforeCalled = true
			beforeMeta = hctx.Metadata
			user.Name = "mutated-before"
			user.Age = 21
			return nil
		}},
		AfterCreate: []HookFunc[*TestUser]{func(hctx HookContext, user *TestUser) error {
			afterCalled = true
			afterMeta = hctx.Metadata
			user.Age = 99
			return nil
		}},
	}

	app, repo, db := setupAppWithHooks(t, hooks)
	defer db.Close()

	body := `{"name":"original","email":"hooks@example.com","password":"secret","age":10}`
	req := httptest.NewRequest(http.MethodPost, "/test-user", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var created TestUser
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.True(t, beforeCalled, "before hook should run")
	assert.True(t, afterCalled, "after hook should run")

	assert.Equal(t, OpCreate, beforeMeta.Operation)
	assert.Equal(t, OpCreate, afterMeta.Operation)
	assert.Equal(t, beforeMeta, afterMeta)
	assert.Equal(t, http.MethodPost, beforeMeta.Method)
	assert.Equal(t, "/test-user", beforeMeta.Path)
	assert.Equal(t, "test-user:create", beforeMeta.RouteName)

	assert.Equal(t, "mutated-before", created.Name)
	assert.Equal(t, 99, created.Age)

	saved, err := repo.GetByID(context.Background(), created.ID.String())
	if err != nil {
		t.Fatalf("Failed to load saved record: %v", err)
	}
	assert.Equal(t, 21, saved.Age)
	assert.Equal(t, "mutated-before", saved.Name)
}

func TestControllerWithService_UsesServiceHooksAndMetadata(t *testing.T) {
	app := fiber.New()

	sqldb, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	defer db.Close()

	ctx := context.Background()
	require.NoError(t, createSchema(ctx, db))

	repo := newTestUserRepository(db)

	var beforeMeta, afterMeta HookMetadata
	hooks := LifecycleHooks[*TestUser]{
		BeforeCreate: []HookFunc[*TestUser]{func(hctx HookContext, user *TestUser) error {
			beforeMeta = hctx.Metadata
			user.Name = "service-before"
			return nil
		}},
		AfterCreate: []HookFunc[*TestUser]{func(hctx HookContext, user *TestUser) error {
			afterMeta = hctx.Metadata
			user.Age = 77
			return nil
		}},
	}

	service := NewService(ServiceConfig[*TestUser]{
		Repository: repo,
		Hooks:      hooks,
	})

	controller := NewControllerWithService(repo, service, WithDeserializer(testUserDeserializer))

	router := NewFiberAdapter(app)
	controller.RegisterRoutes(router)

	body := `{"name":"original","email":"hooks-service@example.com","password":"secret","age":10}`
	req := httptest.NewRequest(http.MethodPost, "/test-user", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created TestUser
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	assert.Equal(t, "service-before", created.Name)
	assert.Equal(t, 77, created.Age)

	assert.Equal(t, OpCreate, beforeMeta.Operation)
	assert.Equal(t, beforeMeta, afterMeta)
	assert.Equal(t, http.MethodPost, beforeMeta.Method)
	assert.Equal(t, "/test-user", beforeMeta.Path)
	assert.Equal(t, "test-user:create", beforeMeta.RouteName)
}

type contextFactoryWrapper struct {
	Context
	userCtx context.Context
}

func (w *contextFactoryWrapper) UserContext() context.Context {
	if w.userCtx == nil {
		return context.Background()
	}
	return w.userCtx
}

func (w *contextFactoryWrapper) SetUserContext(ctx context.Context) {
	w.userCtx = ctx
	if setter, ok := w.Context.(userContextSetter); ok {
		setter.SetUserContext(ctx)
	}
}

type contextFactoryCheckService struct {
	next Service[*TestUser]
	t    *testing.T
	key  any
	seen map[CrudOperation]int
}

func (s *contextFactoryCheckService) check(ctx Context) {
	s.t.Helper()
	value := ctx.UserContext().Value(s.key)
	require.Equal(s.t, "factory", value)
	meta := HookMetadataFromContext(ctx.UserContext())
	if meta.Operation != "" {
		s.seen[meta.Operation]++
	}
}

func (s *contextFactoryCheckService) Create(ctx Context, record *TestUser) (*TestUser, error) {
	s.check(ctx)
	return s.next.Create(ctx, record)
}

func (s *contextFactoryCheckService) CreateBatch(ctx Context, records []*TestUser) ([]*TestUser, error) {
	s.check(ctx)
	return s.next.CreateBatch(ctx, records)
}

func (s *contextFactoryCheckService) Update(ctx Context, record *TestUser) (*TestUser, error) {
	s.check(ctx)
	return s.next.Update(ctx, record)
}

func (s *contextFactoryCheckService) UpdateBatch(ctx Context, records []*TestUser) ([]*TestUser, error) {
	s.check(ctx)
	return s.next.UpdateBatch(ctx, records)
}

func (s *contextFactoryCheckService) Delete(ctx Context, record *TestUser) error {
	s.check(ctx)
	return s.next.Delete(ctx, record)
}

func (s *contextFactoryCheckService) DeleteBatch(ctx Context, records []*TestUser) error {
	s.check(ctx)
	return s.next.DeleteBatch(ctx, records)
}

func (s *contextFactoryCheckService) Index(ctx Context, criteria []repository.SelectCriteria) ([]*TestUser, int, error) {
	s.check(ctx)
	return s.next.Index(ctx, criteria)
}

func (s *contextFactoryCheckService) Show(ctx Context, id string, criteria []repository.SelectCriteria) (*TestUser, error) {
	s.check(ctx)
	return s.next.Show(ctx, id, criteria)
}

func TestController_ContextFactoryRunsForAllOperations(t *testing.T) {
	type ctxKey struct{}

	var factoryCalls int
	factory := func(ctx Context) Context {
		factoryCalls++
		userCtx := context.WithValue(ctx.UserContext(), ctxKey{}, "factory")
		return &contextFactoryWrapper{
			Context: ctx,
			userCtx: userCtx,
		}
	}

	var checker *contextFactoryCheckService
	app, db := setupApp(t,
		WithContextFactory[*TestUser](factory),
		WithCommandService[*TestUser](func(defaults Service[*TestUser]) Service[*TestUser] {
			checker = &contextFactoryCheckService{
				next: defaults,
				t:    t,
				key:  ctxKey{},
				seen: make(map[CrudOperation]int),
			}
			return checker
		}),
	)
	defer db.Close()

	userOne := &TestUser{
		ID:       uuid.New(),
		Name:     "User One",
		Email:    "user-one@example.com",
		Password: "secret",
		Age:      21,
	}
	userTwo := &TestUser{
		ID:       uuid.New(),
		Name:     "User Two",
		Email:    "user-two@example.com",
		Password: "secret",
		Age:      25,
	}
	insertTestUsers(t, db, userOne, userTwo)

	createBody := `{"name":"Created","email":"created@example.com","password":"secret","age":30}`
	req := httptest.NewRequest(http.MethodPost, "/test-user", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created TestUser
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	req = httptest.NewRequest(http.MethodGet, "/test-users", nil)
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test-user/%s", userOne.ID.String()), nil)
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	updateBody := `{"name":"Updated","email":"updated@example.com","password":"secret","age":31}`
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/test-user/%s", userOne.ID.String()), strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	updateBatchPayload := []map[string]any{
		{
			"id":       userOne.ID.String(),
			"name":     "Batch One",
			"email":    "batch-one@example.com",
			"password": "secret",
			"age":      40,
		},
		{
			"id":       userTwo.ID.String(),
			"name":     "Batch Two",
			"email":    "batch-two@example.com",
			"password": "secret",
			"age":      41,
		},
	}
	buf, err := json.Marshal(updateBatchPayload)
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPut, "/test-user/batch", bytes.NewBuffer(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	createBatchPayload := []map[string]any{
		{
			"name":     "Batch Create One",
			"email":    "batch-create-one@example.com",
			"password": "secret",
			"age":      18,
		},
		{
			"name":     "Batch Create Two",
			"email":    "batch-create-two@example.com",
			"password": "secret",
			"age":      19,
		},
	}
	buf, err = json.Marshal(createBatchPayload)
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/test-user/batch", bytes.NewBuffer(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var createdBatch APIListResponse[TestUser]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createdBatch))
	require.Len(t, createdBatch.Data, 2)

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/test-user/%s", created.ID.String()), nil)
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	deleteBatchPayload := []map[string]any{
		{"id": createdBatch.Data[0].ID.String()},
		{"id": createdBatch.Data[1].ID.String()},
	}
	buf, err = json.Marshal(deleteBatchPayload)
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodDelete, "/test-user/batch", bytes.NewBuffer(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	expectedOps := []CrudOperation{
		OpCreate,
		OpCreateBatch,
		OpRead,
		OpList,
		OpUpdate,
		OpUpdateBatch,
		OpDelete,
		OpDeleteBatch,
	}
	for _, op := range expectedOps {
		assert.Greater(t, checker.seen[op], 0, "expected context factory for %s", op)
	}
	assert.Equal(t, len(expectedOps), factoryCalls)
}

func Test_ParseFieldOperator(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedField string
		expectedOp    string
	}{
		{
			name:          "Ilike operator",
			input:         "name__ilike",
			expectedField: "name",
			expectedOp:    "ILIKE",
		},
		{
			name:          "Gte operator",
			input:         "age__gte",
			expectedField: "age",
			expectedOp:    ">=",
		},
		{
			name:          "No operator",
			input:         "age",
			expectedField: "age",
			expectedOp:    "=",
		},
		{
			name:          "Unknown operator defaults to '='",
			input:         "status__unknown",
			expectedField: "status",
			expectedOp:    "=",
		},
		{
			name:          "Multiple '__' in input",
			input:         "user_name__like",
			expectedField: "user_name",
			expectedOp:    "LIKE",
		},
		{
			name:          "Empty operator",
			input:         "email__",
			expectedField: "email",
			expectedOp:    "=",
		},
		{
			name:          "Empty field and operator",
			input:         "__",
			expectedField: "",
			expectedOp:    "=",
		},
		{
			name:          "Only operator",
			input:         "__eq",
			expectedField: "",
			expectedOp:    "=",
		},
		{
			name:          "And operator",
			input:         "name__and",
			expectedField: "name",
			expectedOp:    "and",
		},
		{
			name:          "Or operator",
			input:         "name__or",
			expectedField: "name",
			expectedOp:    "or",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, op := parseFieldOperator(tt.input)
			assert.Equal(t, tt.expectedField, field)
			assert.Equal(t, tt.expectedOp, op)
		})
	}
}

func Test_ParseFieldOperator_Custom(t *testing.T) {
	defer SetOperatorMap(DefaultOperatorMap())

	SetOperatorMap(map[string]string{
		"$eq":    "=",
		"$ne":    "<>",
		"$gt":    ">",
		"$lt":    "<",
		"$gte":   ">=",
		"$lte":   "<=",
		"$ilike": "ILIKE",
		"$like":  "LIKE",
		"$and":   "and",
		"$or":    "or",
	})

	tests := []struct {
		name          string
		input         string
		expectedField string
		expectedOp    string
	}{
		{
			name:          "Ilike operator",
			input:         "name__$ilike",
			expectedField: "name",
			expectedOp:    "ILIKE",
		},
		{
			name:          "Gte operator",
			input:         "age__$gte",
			expectedField: "age",
			expectedOp:    ">=",
		},
		{
			name:          "No operator",
			input:         "age",
			expectedField: "age",
			expectedOp:    "=",
		},
		{
			name:          "Canonical operator remains supported",
			input:         "name__ilike",
			expectedField: "name",
			expectedOp:    "ILIKE",
		},
		{
			name:          "Unknown operator defaults to '='",
			input:         "status__$unknown",
			expectedField: "status",
			expectedOp:    "=",
		},
		{
			name:          "Multiple '__' in input",
			input:         "user_name__$like",
			expectedField: "user_name",
			expectedOp:    "LIKE",
		},
		{
			name:          "Empty operator",
			input:         "email__",
			expectedField: "email",
			expectedOp:    "=",
		},
		{
			name:          "Empty field and operator",
			input:         "__",
			expectedField: "",
			expectedOp:    "=",
		},
		{
			name:          "Only operator",
			input:         "__$eq",
			expectedField: "",
			expectedOp:    "=",
		},
		{
			name:          "And operator",
			input:         "name__$and",
			expectedField: "name",
			expectedOp:    "and",
		},
		{
			name:          "Or operator",
			input:         "name__$or",
			expectedField: "name",
			expectedOp:    "or",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, op := parseFieldOperator(tt.input)
			assert.Equal(t, tt.expectedField, field)
			assert.Equal(t, tt.expectedOp, op)
		})
	}

}

func TestController_WithScopeGuardFiltersList(t *testing.T) {
	guard := func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
		scope := ScopeFilter{}
		scope.AddColumnFilter("name", "=", "Alice Johnson")
		return ActorContext{ActorID: "actor-list", TenantID: "tenant-alpha"}, scope, nil
	}

	app, db := setupApp(t, WithScopeGuard[*TestUser](guard))
	defer db.Close()

	insertTestUsers(t, db,
		&TestUser{Name: "Alice Johnson", Email: "alice@example.com"},
		&TestUser{Name: "Bob Smith", Email: "bob@example.com"},
	)

	req := httptest.NewRequest(http.MethodGet, "/test-users", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	data, ok := payload["data"].([]any)
	require.True(t, ok, "expected data array in response")
	require.Len(t, data, 1)
	row, ok := data[0].(map[string]any)
	require.True(t, ok, "expected map for row data")
	assert.Equal(t, "Alice Johnson", row["name"])
}

func TestController_WithScopeGuardBlocksDeleteOutsideScope(t *testing.T) {
	var (
		alice = &TestUser{Name: "Alice Johnson", Email: "alice.scope@example.com"}
		bob   = &TestUser{Name: "Bob Smith", Email: "bob.scope@example.com"}
	)

	guard := func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
		scope := ScopeFilter{}
		scope.AddColumnFilter("name", "=", alice.Name)
		return ActorContext{ActorID: "actor-delete"}, scope, nil
	}

	app, db := setupApp(t, WithScopeGuard[*TestUser](guard))
	defer db.Close()

	insertTestUsers(t, db, alice, bob)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/test-user/%s", bob.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/test-user/%s", alice.ID), nil)
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestController_HookContextIncludesGuardMetadata(t *testing.T) {
	var captured HookContext

	hooks := LifecycleHooks[*TestUser]{
		BeforeCreate: []HookFunc[*TestUser]{func(hctx HookContext, record *TestUser) error {
			captured = hctx
			return nil
		}},
	}

	guard := func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
		scope := ScopeFilter{}
		scope.AddColumnFilter("name", "=", "Hooked User")
		actor := ActorContext{
			ActorID:        "actor-hooks",
			TenantID:       "tenant-guard",
			OrganizationID: "org-guard",
		}
		return actor, scope, nil
	}

	app, db := setupApp(t, WithLifecycleHooks(hooks), WithScopeGuard[*TestUser](guard))
	defer db.Close()

	payload := map[string]any{
		"name":  "Hooked User",
		"email": "hooked@example.com",
		"age":   30,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/test-user", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-123")
	req.Header.Set("X-Correlation-ID", "corr-456")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, captured.Context)
	assert.Equal(t, "actor-hooks", captured.Actor.ActorID)
	assert.Equal(t, "tenant-guard", captured.Actor.TenantID)
	assert.Equal(t, "org-guard", captured.Actor.OrganizationID)
	require.Len(t, captured.Scope.ColumnFilters, 1)
	assert.Equal(t, "name", captured.Scope.ColumnFilters[0].Column)
	assert.Equal(t, []string{"Hooked User"}, captured.Scope.ColumnFilters[0].Values)
	assert.Equal(t, "req-123", captured.RequestID)
	assert.Equal(t, "corr-456", captured.CorrelationID)
}

func TestController_ScopeGuardErrorSurfaces(t *testing.T) {
	guardErr := goerrors.New("not authorized", goerrors.CategoryAuthz).
		WithCode(http.StatusForbidden).
		WithTextCode("FORBIDDEN")
	guard := func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
		return ActorContext{}, ScopeFilter{}, guardErr
	}

	app, db := setupApp(t, WithScopeGuard[*TestUser](guard))
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/test-users", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	var payload goerrors.ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	if assert.NotNil(t, payload.Error) {
		assert.Equal(t, goerrors.CategoryAuthz, payload.Error.Category)
		assert.Equal(t, http.StatusForbidden, payload.Error.Code)
		assert.Equal(t, "not authorized", payload.Error.Message)
	}
}

func TestController_WithCommandServiceOverridesCreate(t *testing.T) {
	var commandCalled bool

	commandFactory := func(defaults Service[*TestUser]) Service[*TestUser] {
		return ComposeService(defaults, ServiceFuncs[*TestUser]{
			Create: func(ctx Context, record *TestUser) (*TestUser, error) {
				commandCalled = true
				record.Name = "command:" + record.Name
				return defaults.Create(ctx, record)
			},
		})
	}

	app, db := setupApp(t, WithCommandService(commandFactory))
	defer db.Close()

	body := `{"name":"original","email":"cmd@example.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/test-user", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.True(t, commandCalled, "command service should intercept create")

	var created TestUser
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	assert.Equal(t, "command:original", created.Name)
}

func TestController_ActionResourceRouteExecutesHandler(t *testing.T) {
	var invoked bool
	var guardOps []CrudOperation

	action := Action[*TestUser]{
		Name:   "Deactivate",
		Method: http.MethodPost,
		Target: ActionTargetResource,
		Handler: func(actx ActionContext[*TestUser]) error {
			invoked = true
			assert.Equal(t, CrudOperation("action:deactivate"), actx.Operation)
			return actx.Status(http.StatusAccepted).JSON(fiber.Map{"ok": true})
		},
	}

	guard := func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
		guardOps = append(guardOps, op)
		return ActorContext{ActorID: "actor-action"}, ScopeFilter{}, nil
	}

	app, db := setupApp(t,
		WithActions(action),
		WithScopeGuard[*TestUser](guard),
	)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/test-user/%s/actions/deactivate", uuid.New()), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.True(t, invoked, "action handler should run")
	assert.Equal(t, []CrudOperation{CrudOperation("action:deactivate")}, guardOps)
}

func TestController_ActionCollectionRouteExecutesHandler(t *testing.T) {
	var invoked bool

	action := Action[*TestUser]{
		Name:   "SyncAll",
		Method: http.MethodPost,
		Target: ActionTargetCollection,
		Handler: func(actx ActionContext[*TestUser]) error {
			invoked = true
			return actx.Status(http.StatusOK).JSON(fiber.Map{"synced": true})
		},
	}

	app, db := setupApp(t, WithActions(action))
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/test-users/actions/sync-all", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, invoked)
}

func TestSendNotificationHelperEmitsEvents(t *testing.T) {
	emitter := &testNotificationEmitter{}
	hooks := LifecycleHooks[*TestUser]{
		AfterUpdate: []HookFunc[*TestUser]{func(hctx HookContext, user *TestUser) error {
			return SendNotification(hctx, ActivityPhaseAfter, user,
				WithNotificationChannel("email"),
				WithNotificationTemplate("user-updated"),
				WithNotificationRecipients("ops@example.com"),
			)
		}},
	}

	app, db := setupApp(t,
		WithLifecycleHooks(hooks),
		WithNotificationEmitter[*TestUser](emitter),
	)
	defer db.Close()

	user := &TestUser{Name: "Notify User", Email: "notify@example.com"}
	insertTestUsers(t, db, user)

	body := fmt.Sprintf(`{"name":"Notify Updated","email":"%s","password":"secret"}`, user.Email)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/test-user/%s", user.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Len(t, emitter.events, 1)
	event := emitter.events[0]
	assert.Equal(t, OpUpdate, event.Operation)
	assert.Equal(t, ActivityPhaseAfter, event.Phase)
	assert.Equal(t, "email", event.Channel)
	assert.Equal(t, "user-updated", event.Template)
	require.Equal(t, []string{"ops@example.com"}, event.Recipients)
	require.Len(t, event.Records, 1)
	record, ok := event.Records[0].(*TestUser)
	require.True(t, ok)
	assert.Equal(t, "Notify Updated", record.Name)
}

func TestActivityHooksEmitterEmitsOnCreate(t *testing.T) {
	capture := &activity.CaptureHook{}
	app, db := setupApp(t,
		WithActivityHooks[*TestUser](activity.Hooks{capture}, activity.Config{Enabled: true}),
	)
	defer db.Close()

	body := `{"name":"Emit Activity","email":"emit-hooks@example.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/test-user", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "activity-hooks-req")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	require.Len(t, capture.Events, 1)
	event := capture.Events[0]
	assert.Equal(t, "crud.test-user.create", event.Verb)
	assert.Equal(t, "test-user", event.ObjectType)
	assert.NotEmpty(t, event.ObjectID)
	assert.Equal(t, "crud", event.Channel)
	meta := event.Metadata
	require.NotNil(t, meta)
	assert.Equal(t, "test-user:create", meta["route_name"])
	assert.Equal(t, "/test-user", meta["route_path"])
	assert.Equal(t, http.MethodPost, meta["method"])
	assert.Equal(t, "activity-hooks-req", meta["request_id"])
	assert.Equal(t, 1, meta["batch_size"])
	assert.Equal(t, 0, meta["batch_index"])
	assert.NotContains(t, meta, "error")
}

func TestActivityHooksEmitterEmitsOnFailure(t *testing.T) {
	capture := &activity.CaptureHook{}
	app, db := setupApp(t,
		WithActivityHooks[*TestUser](activity.Hooks{capture}, activity.Config{Enabled: true}),
	)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPut, "/test-user/not-a-uuid", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "activity-hooks-fail")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.NotEqual(t, http.StatusOK, resp.StatusCode)

	require.Len(t, capture.Events, 1)
	event := capture.Events[0]
	assert.Equal(t, "crud.test-user.update.failed", event.Verb)
	assert.Equal(t, "test-user", event.ObjectType)
	assert.NotEmpty(t, event.ObjectID)
	meta := event.Metadata
	require.NotNil(t, meta)
	assert.Equal(t, "test-user:update", meta["route_name"])
	assert.Equal(t, "/test-user/:id", meta["route_path"])
	assert.Equal(t, http.MethodPut, meta["method"])
	assert.Equal(t, "activity-hooks-fail", meta["request_id"])
	assert.Contains(t, meta, "error")
}

func TestActivityHooksEmitterDisabled(t *testing.T) {
	capture := &activity.CaptureHook{}
	app, db := setupApp(t,
		WithActivityHooks[*TestUser](activity.Hooks{capture}, activity.Config{Enabled: false}),
	)
	defer db.Close()

	body := `{"name":"Emit Activity","email":"emit-hooks-disabled@example.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/test-user", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	require.Empty(t, capture.Events)
}

func TestController_FieldPolicyRestrictsListFields(t *testing.T) {
	provider := func(req FieldPolicyRequest[*TestUser]) (FieldPolicy, error) {
		if req.Operation == OpList {
			return FieldPolicy{
				Name:  "list:name-only",
				Allow: []string{"id", "name"},
			}, nil
		}
		return FieldPolicy{}, nil
	}

	app, db := setupApp(t, WithFieldPolicyProvider(provider))
	defer db.Close()

	insertTestUsers(t, db, &TestUser{Name: "Policy User", Email: "policy@example.com"})

	req := httptest.NewRequest(http.MethodGet, "/test-users?select=id,name,email", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	data := payload["data"].([]any)
	require.Len(t, data, 1)
	row := data[0].(map[string]any)
	assert.Equal(t, "Policy User", row["name"])
	assert.Equal(t, "", row["email"], "email should be blank when denied by field policy")
}

func TestController_FieldPolicyRowFilterAppliesToUpdate(t *testing.T) {
	filter := ScopeFilter{}
	filter.AddColumnFilter("name", "=", "Allowed User")
	provider := func(req FieldPolicyRequest[*TestUser]) (FieldPolicy, error) {
		if req.Operation == OpUpdate {
			return FieldPolicy{
				Name:      "update:allowed-user",
				RowFilter: filter,
			}, nil
		}
		return FieldPolicy{}, nil
	}

	app, db := setupApp(t, WithFieldPolicyProvider(provider))
	defer db.Close()

	allowed := &TestUser{Name: "Allowed User", Email: "allowed@example.com"}
	blocked := &TestUser{Name: "Blocked User", Email: "blocked@example.com"}
	insertTestUsers(t, db, allowed, blocked)

	body := `{"name":"Blocked Updated","email":"blocked@example.com"}`
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/test-user/%s", blocked.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "row filter should block update outside policy")
}

func TestController_FieldPolicyMasksShowFields(t *testing.T) {
	provider := func(req FieldPolicyRequest[*TestUser]) (FieldPolicy, error) {
		if req.Operation == OpRead {
			mask := ScopeFilter{}
			return FieldPolicy{
				Name: "show:mask",
				Deny: []string{"age"},
				Mask: map[string]FieldMaskFunc{
					"email": func(value any) any {
						return "hidden@example.com"
					},
				},
				RowFilter: mask,
			}, nil
		}
		return FieldPolicy{}, nil
	}

	app, db := setupApp(t, WithFieldPolicyProvider(provider))
	defer db.Close()

	user := &TestUser{Name: "Mask User", Email: "mask@example.com", Age: 42}
	insertTestUsers(t, db, user)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test-user/%s", user.ID), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	data := payload["data"].(map[string]any)
	assert.Equal(t, "hidden@example.com", data["email"])
	assert.Equal(t, float64(0), data["age"])
}
