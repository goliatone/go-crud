package crud

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

// mockContext implements the Request interface.
type mockContext struct {
	userCtx     context.Context
	paramsMap   map[string]string
	queryMap    map[string]string
	requestBody []byte
	status      int
	queries     url.Values
	// Optionally store a JSON payload or other data for testing
	jsonData  any
	sentError error
}

// Constructor for convenience
func newMockRequest() *mockContext {
	return &mockContext{
		userCtx:   context.Background(),
		paramsMap: make(map[string]string),
		queryMap:  make(map[string]string),
		status:    200,
	}
}

// UserContext returns the context.
func (m *mockContext) UserContext() context.Context {
	if m.userCtx == nil {
		return context.Background()
	}
	return m.userCtx
}

// Params returns a URL parameter by key, or defaultValue if provided and the key does not exist.
func (m *mockContext) Params(key string, defaultValue ...string) string {
	val, ok := m.paramsMap[key]
	if !ok && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return val
}

// BodyParser unmarshals the JSON in requestBody into out.
func (m *mockContext) BodyParser(out any) error {
	if len(m.requestBody) == 0 {
		return nil
	}
	return json.Unmarshal(m.requestBody, out)
}

// Query returns a query parameter by key, or defaultValue if provided and the key does not exist.
func (m *mockContext) Query(key string, defaultValue ...string) string {
	val, ok := m.queryMap[key]
	if !ok && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return val
}

// QueryInt returns a query parameter parsed as int, or defaultValue if provided and parsing fails.
func (m *mockContext) QueryInt(key string, defaultValue ...int) int {
	val, ok := m.queryMap[key]
	if !ok {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return i
}

// Queries returns all query parameters as a map (single-value).
func (m *mockContext) Queries() map[string]string {
	// Return a copy to avoid mutation
	out := make(map[string]string, len(m.queryMap))
	for k, v := range m.queryMap {
		out[k] = v
	}
	return out
}

// Body returns the raw request body as bytes.
func (m *mockContext) Body() []byte {
	return m.requestBody
}

// Status sets the HTTP status code and returns itself for chaining.
func (m *mockContext) Status(status int) Response {
	m.status = status
	return m
}

// JSON sets the response data (and optionally you could store the contentType).
func (m *mockContext) JSON(data any, ctype ...string) error {
	m.jsonData = data
	return nil
}

// SendStatus sets the status and returns an error if you want to simulate
// some framework error. We'll just store the status.
func (m *mockContext) SendStatus(status int) error {
	m.status = status
	return nil
}

// Optionally, you can add getters for test assertions:
func (m *mockContext) GetStatus() int {
	return m.status
}

func (m *mockContext) GetJSONData() any {
	return m.jsonData
}

// TestModel represents a model with all possible field types for testing
type TestModel struct {
	ID       int     `json:"id" bun:"id"`
	Name     string  `json:"name" bun:"name"`
	Age      int     `json:"age" bun:"age"`
	Score    float64 `json:"score" bun:"score"`
	IsActive bool    `json:"is_active" bun:"is_active"`
	Profile  Profile `json:"profile" bun:"rel:has-one"`
	Company  Company `json:"company" bun:"rel:belongs-to"`
}

type Profile struct {
	ID     int    `json:"id" bun:"id"`
	Status string `json:"status" bun:"status"`
	Points int    `json:"points" bun:"points"`
}

type Company struct {
	ID   int    `json:"id" bun:"id"`
	Type string `json:"type" bun:"type"`
}

type Translation struct {
	ID     int    `json:"id" bun:"id"`
	Locale string `json:"locale" bun:"locale"`
	Status string `json:"status" bun:"status"`
}

type Block struct {
	ID           int           `json:"id" bun:"id"`
	Translations []Translation `json:"translations" bun:"rel:has-many"`
}

type Page struct {
	ID     int     `json:"id" bun:"id"`
	Blocks []Block `json:"blocks" bun:"rel:has-many"`
}

// Enhanced mockContext constructor with query parameters
func newMockContextWithQuery(queryParams map[string]string) *mockContext {
	mock := newMockRequest()
	mock.queryMap = queryParams
	return mock
}

// Helper function to set up test DB
func setupTestDB(t *testing.T) *bun.DB {
	sqldb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	return db
}

func TestBuildQueryCriteria(t *testing.T) {
	SetOperatorMap(DefaultOperatorMap())
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	if err := db.ResetModel(ctx, (*TestUser)(nil)); err != nil {
		t.Fatal(err)
	}

	users := []TestUser{
		{
			ID:        uuid.New(),
			Name:      "Alice",
			Email:     "alice@example.com",
			Age:       30,
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Bob",
			Email:     "bob@example.com",
			Age:       25,
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Charlie",
			Email:     "charlie@example.com",
			Age:       35,
			CreatedAt: time.Now(),
		},
	}

	for _, user := range users {
		_, err := db.NewInsert().Model(&user).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name          string
		queryParams   map[string]string
		operation     CrudOperation
		expectedCount int
		validate      func(*testing.T, *bun.DB, []TestUser)
	}{
		{
			name: "basic pagination",
			queryParams: map[string]string{
				"limit":  "2",
				"offset": "1",
			},
			operation:     OpList,
			expectedCount: 2,
			validate: func(t *testing.T, db *bun.DB, results []TestUser) {
				assert.Len(t, results, 2)
			},
		},
		{
			name: "name equals filter",
			queryParams: map[string]string{
				"name": "Bob",
			},
			operation:     OpList,
			expectedCount: 1,
			validate: func(t *testing.T, db *bun.DB, results []TestUser) {
				assert.Len(t, results, 1)
				assert.Equal(t, "Bob", results[0].Name)
			},
		},
		{
			name: "age greater than filter",
			queryParams: map[string]string{
				"age__gt": "30",
			},
			operation:     OpList,
			expectedCount: 1,
			validate: func(t *testing.T, db *bun.DB, results []TestUser) {
				assert.Len(t, results, 1)
				assert.Equal(t, "Charlie", results[0].Name)
			},
		},
		{
			name: "email LIKE filter",
			queryParams: map[string]string{
				"email__like": "%example.com",
			},
			operation:     OpList,
			expectedCount: 3,
			validate: func(t *testing.T, db *bun.DB, results []TestUser) {
				assert.Len(t, results, 3)
			},
		},
		{
			name: "name OR condition",
			queryParams: map[string]string{
				"name__or": "Alice,Bob",
			},
			operation:     OpList,
			expectedCount: 2,
			validate: func(t *testing.T, db *bun.DB, results []TestUser) {
				assert.Len(t, results, 2)
				names := []string{results[0].Name, results[1].Name}
				assert.Contains(t, names, "Alice")
				assert.Contains(t, names, "Bob")
			},
		},
		{
			name: "age between condition",
			queryParams: map[string]string{
				"age__gte": "25",
				"age__lte": "30",
			},
			operation:     OpList,
			expectedCount: 2,
			validate: func(t *testing.T, db *bun.DB, results []TestUser) {
				assert.Len(t, results, 2)
				for _, user := range results {
					assert.GreaterOrEqual(t, user.Age, 25)
					assert.LessOrEqual(t, user.Age, 30)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newMockContextWithQuery(tt.queryParams)
			criteria, _, err := BuildQueryCriteria[TestUser](ctx, tt.operation)
			assert.NoError(t, err)

			var results []TestUser
			q := db.NewSelect().Model(&results)

			for _, c := range criteria {
				q = c(q)
			}

			err = q.Scan(context.Background())
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedCount, len(results))
			if tt.validate != nil {
				tt.validate(t, db, results)
			}
		})
	}
}

func TestBuildIncludeTree(t *testing.T) {
	meta := getRelationMetadataForType(reflect.TypeOf(Page{}))
	require.NotNil(t, meta)

	nodes, err := buildIncludeTree("Blocks.Translations.locale__eq=es", meta)
	require.NoError(t, err)
	require.NotNil(t, nodes)

	blockNode, ok := nodes["Blocks"]
	require.True(t, ok)
	assert.Equal(t, "Blocks", blockNode.name)

	translationNode, ok := blockNode.children["Translations"]
	require.True(t, ok)
	assert.Equal(t, "Translations", translationNode.name)
	require.Len(t, translationNode.filters, 1)

	filter := translationNode.filters[0]
	assert.Equal(t, "locale", filter.field)
	assert.Equal(t, "=", filter.operator)
	assert.Equal(t, "es", filter.value)
}

func TestFieldMapProviderOverrides(t *testing.T) {
	type Custom struct {
		Title string `json:"title" bun:"title"`
		Slug  string `json:"slug" bun:"slug"`
	}

	provider := func(t reflect.Type) map[string]string {
		if indirectType(t).Name() == "Custom" {
			return map[string]string{
				"display_title": "title",
			}
		}
		return nil
	}

	registerQueryConfig(reflect.TypeOf(Custom{}), provider)

	fields := getAllowedFields[Custom]()
	require.Contains(t, fields, "display_title")
	assert.Equal(t, "title", fields["display_title"])
	require.Contains(t, fields, "slug")
	assert.Equal(t, "slug", fields["slug"])

	ctx := newMockContextWithQuery(map[string]string{
		"select": "display_title",
	})

	_, filters, err := BuildQueryCriteria[Custom](ctx, OpList)
	require.NoError(t, err)
	require.NotNil(t, filters)
	assert.Contains(t, filters.Fields, "title")
}

func TestFieldMapProviderNestedRelations(t *testing.T) {
	type NestedTranslation struct {
		Locale string `json:"locale_code" bun:"locale"`
		Label  string `json:"label" bun:"label"`
	}

	type NestedBlock struct {
		Name         string              `json:"name" bun:"name"`
		Translations []NestedTranslation `json:"translations" bun:"rel:has-many"`
	}

	type NestedPage struct {
		Blocks []NestedBlock `json:"blocks" bun:"rel:has-many"`
	}

	provider := func(t reflect.Type) map[string]string {
		switch indirectType(t).Name() {
		case "NestedTranslation":
			return map[string]string{
				"locale_alias": "locale",
			}
		default:
			return nil
		}
	}

	registerQueryConfig(reflect.TypeOf(NestedPage{}), provider)

	include := "Blocks.Translations.locale_alias__eq=es"
	ctx := newMockContextWithQuery(map[string]string{
		"include": include,
	})

	_, filters, err := BuildQueryCriteria[NestedPage](ctx, OpList)
	require.NoError(t, err)
	require.NotNil(t, filters)
	assert.Contains(t, filters.Include, "Blocks.Translations")
	require.Len(t, filters.Relations, 1)
	assert.Equal(t, "Blocks.Translations", filters.Relations[0].Name)
	require.Len(t, filters.Relations[0].Filters, 1)
	assert.Equal(t, "locale_alias", filters.Relations[0].Filters[0].Field)
	assert.Equal(t, "es", filters.Relations[0].Filters[0].Value)
}

func TestBuildQueryCriteria_Filters(t *testing.T) {
	SetOperatorMap(DefaultOperatorMap())

	tests := []struct {
		name            string
		queryParams     map[string]string
		operation       CrudOperation
		model           string
		validateFilters func(*testing.T, *Filters)
	}{
		{
			name: "pagination filters",
			queryParams: map[string]string{
				"limit":  "10",
				"offset": "20",
			},
			operation: OpList,
			model:     "user",
			validateFilters: func(t *testing.T, filters *Filters) {
				assert.Equal(t, 10, filters.Limit)
				assert.Equal(t, 20, filters.Offset)
				assert.Empty(t, filters.Fields)
				assert.Empty(t, filters.Order)
				assert.Empty(t, filters.Include)
				assert.Empty(t, filters.Relations)
			},
		},
		{
			name: "field selection filters",
			queryParams: map[string]string{
				"select": "id,name,email",
			},
			operation: OpList,
			model:     "user",
			validateFilters: func(t *testing.T, filters *Filters) {
				assert.Equal(t, DefaultLimit, filters.Limit)
				assert.Equal(t, DefaultOffset, filters.Offset)
				assert.ElementsMatch(t, []string{"id", "name", "email"}, filters.Fields)
				assert.Empty(t, filters.Order)
				assert.Empty(t, filters.Include)
				assert.Empty(t, filters.Relations)
			},
		},
		{
			name: "ordering filters",
			queryParams: map[string]string{
				"order": "name asc,age desc",
			},
			operation: OpList,
			model:     "user",
			validateFilters: func(t *testing.T, filters *Filters) {
				assert.Equal(t, DefaultLimit, filters.Limit)
				assert.Equal(t, DefaultOffset, filters.Offset)
				assert.Empty(t, filters.Fields)
				assert.NotNil(t, filters.Order)
				assert.Len(t, filters.Order, 2)
				if len(filters.Order) >= 2 {
					assert.Equal(t, "name", filters.Order[0].Field)
					assert.Equal(t, "ASC", filters.Order[0].Dir)
					assert.Equal(t, "age", filters.Order[1].Field)
					assert.Equal(t, "DESC", filters.Order[1].Dir)
				}
			},
		},
		{
			name: "basic include filters",
			queryParams: map[string]string{
				"include": "Profile,Company",
			},
			operation: OpList,
			model:     "model",
			validateFilters: func(t *testing.T, filters *Filters) {
				assert.Equal(t, DefaultLimit, filters.Limit)
				assert.Equal(t, DefaultOffset, filters.Offset)
				assert.Empty(t, filters.Fields)
				assert.Empty(t, filters.Order)
				assert.ElementsMatch(t, []string{"Profile", "Company"}, filters.Include)
			},
		},
		{
			name: "relation filters",
			queryParams: map[string]string{
				"include": "Profile.status=active,Profile.points__gte=1000",
			},
			operation: OpList,
			model:     "model",
			validateFilters: func(t *testing.T, filters *Filters) {
				assert.Equal(t, DefaultLimit, filters.Limit)
				assert.Equal(t, DefaultOffset, filters.Offset)
				assert.Empty(t, filters.Fields)
				assert.Empty(t, filters.Order)
				assert.Contains(t, filters.Include, "Profile")
				require.Len(t, filters.Relations, 1)
				if len(filters.Relations) > 0 {
					rel := filters.Relations[0]
					assert.Equal(t, "Profile", rel.Name)
					require.Len(t, rel.Filters, 2)
					assert.Equal(t, "status", rel.Filters[0].Field)
					assert.Equal(t, "active", rel.Filters[0].Value)
					assert.Equal(t, "points", rel.Filters[1].Field)
					assert.Equal(t, "1000", rel.Filters[1].Value)
				}
			},
		},
		{
			name: "nested relation with locale filter",
			queryParams: map[string]string{
				"include": "Blocks.Translations.locale__eq=es",
			},
			operation: OpList,
			model:     "page",
			validateFilters: func(t *testing.T, filters *Filters) {
				assert.Contains(t, filters.Include, "Blocks")
				assert.Contains(t, filters.Include, "Blocks.Translations")
				require.Len(t, filters.Relations, 1)
				if len(filters.Relations) > 0 {
					rel := filters.Relations[0]
					assert.Equal(t, "Blocks.Translations", rel.Name)
					require.Len(t, rel.Filters, 1)
					assert.Equal(t, "locale", rel.Filters[0].Field)
					assert.Equal(t, "es", rel.Filters[0].Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newMockContextWithQuery(tt.queryParams)
			var filters *Filters
			var err error
			switch tt.model {
			case "model":
				_, filters, err = BuildQueryCriteria[TestModel](ctx, tt.operation)
			case "page":
				_, filters, err = BuildQueryCriteria[Page](ctx, tt.operation)
			default:
				_, filters, err = BuildQueryCriteria[TestUser](ctx, tt.operation)
			}

			assert.NoError(t, err)
			assert.NotNil(t, filters)

			if tt.validateFilters != nil {
				tt.validateFilters(t, filters)
			}
		})
	}
}
