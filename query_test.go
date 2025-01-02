package crud

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/goliatone/go-print"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
	jsonData  interface{}
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
func (m *mockContext) BodyParser(out interface{}) error {
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
func (m *mockContext) JSON(data interface{}, ctype ...string) error {
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

func (m *mockContext) GetJSONData() interface{} {
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

	// Create test users
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

			// Create a new query and apply the criteria
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

func TestParseRelation(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedField string
		expectedOp    string
		expectedValue string
	}{
		{
			name:          "simple equals filter",
			input:         "Profile.status=active",
			expectedField: "status",
			expectedOp:    "=",
			expectedValue: "active",
		},
		{
			name:          "greater than filter",
			input:         "Profile.points__gte=1000",
			expectedField: "points",
			expectedOp:    ">=",
			expectedValue: "1000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseRelation(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, "Profile", info.name)

			if len(info.filters) > 0 {
				filter := info.filters[0]
				assert.Equal(t, tt.expectedField, filter.field)
				assert.Equal(t, tt.expectedOp, filter.operator)
				assert.Equal(t, tt.expectedValue, filter.value)
			}
		})
	}
}

func TestBuildQueryCriteria_Filters(t *testing.T) {
	SetOperatorMap(DefaultOperatorMap())

	tests := []struct {
		name            string
		queryParams     map[string]string
		operation       CrudOperation
		validateFilters func(*testing.T, *Filters)
	}{
		{
			name: "pagination filters",
			queryParams: map[string]string{
				"limit":  "10",
				"offset": "20",
			},
			operation: OpList,
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
			validateFilters: func(t *testing.T, filters *Filters) {
				assert.Equal(t, DefaultLimit, filters.Limit)
				assert.Equal(t, DefaultOffset, filters.Offset)
				assert.Empty(t, filters.Fields)
				assert.Empty(t, filters.Order)
				assert.Contains(t, filters.Include, "Profile")

				// Skip relations test for now until we implement it
				t.Skip("Relations filtering not yet implemented")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newMockContextWithQuery(tt.queryParams)
			_, filters, err := BuildQueryCriteria[TestUser](ctx, tt.operation)

			assert.NoError(t, err)
			assert.NotNil(t, filters)

			fmt.Println("========")
			fmt.Println(print.MaybePrettyJSON(filters))
			fmt.Println("========")

			if tt.validateFilters != nil {
				tt.validateFilters(t, filters)
			}
		})
	}
}
