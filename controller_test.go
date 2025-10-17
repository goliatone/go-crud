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
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/extra/bundebug"

	"github.com/goliatone/go-repository-bun"
)

type TestUser struct {
	bun.BaseModel `bun:"table:test_users,alias:u"`

	ID        uuid.UUID `bun:"id,pk,notnull" json:"id"`
	Name      string    `bun:"name,notnull" json:"name"`
	Email     string    `bun:"email,notnull,unique" json:"email"`
	Age       int       `bun:"age" json:"age"`
	Password  string    `bun:"password,notnull" json:"-" crud:"-"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
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

func setupApp(t *testing.T) (*fiber.App, *bun.DB) {
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
	controller := NewController[*TestUser](repo, WithDeserializer(testUserDeserializer))

	// Register routes
	router := NewFiberAdapter(app)
	controller.RegisterRoutes(router)

	return app, db
}

func createSchema(ctx context.Context, db *bun.DB) error {
	models := []any{
		(*TestUser)(nil),
	}

	for _, model := range models {
		if _, err := db.NewCreateTable().Model(model).IfNotExists().Exec(ctx); err != nil {
			return err
		}
	}
	return nil
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

	var response APIResponse[TestUser]
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	assert.False(t, response.Success)
	assert.NotEmpty(t, response.Error)
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
				var errorResponse APIResponse[any]
				if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}
				assert.False(t, errorResponse.Success)
				assert.NotEmpty(t, errorResponse.Error)
				t.Fatalf("Unexpected error response: %v", errorResponse.Error)
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
