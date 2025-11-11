package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-crud"
	goerrors "github.com/goliatone/go-errors"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"
)

func buildExampleTestServer(t *testing.T, options ...crud.Option[*User]) (router.Server[*fiber.App], repository.Repository[*User]) {
	t.Helper()

	db := setupDatabase()
	t.Cleanup(func() {
		_ = db.Close()
	})

	handlers := repository.ModelHandlers[*User]{
		NewRecord: func() *User {
			return &User{}
		},
		GetID: func(u *User) uuid.UUID {
			return u.ID
		},
		SetID: func(u *User, id uuid.UUID) {
			u.ID = id
		},
		GetIdentifier: func() string {
			return "Email"
		},
		GetIdentifierValue: func(u *User) string {
			return u.Email
		},
	}

	repo := repository.NewRepository(db, handlers)
	seedDatabase(repo)

	app := router.NewFiberAdapter(func(*fiber.App) *fiber.App {
		return fiber.New(fiber.Config{
			AppName:           "go-crud Web Demo (test)",
			EnablePrintRoutes: false,
		})
	})

	api := app.Router().Group("/api")
	apiAdapter := crud.NewGoRouterAdapter(api)

	controller := crud.NewController(repo, options...)
	controller.RegisterRoutes(apiAdapter)

	app.Init()

	return app, repo
}

func TestWebExampleSchemaIncludesSharedParameters(t *testing.T) {
	app, _ := buildExampleTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/user/schema", nil)
	resp, err := app.WrappedRouter().Test(req)
	if err != nil {
		t.Fatalf("failed to perform schema request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}

	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("failed to decode schema response: %v", err)
	}

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("components section missing: %v", doc)
	}

	params, ok := components["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected shared parameter components")
	}

	checkNumberParam := func(name string, expectedDefault float64) {
		param, ok := params[name].(map[string]any)
		if !ok {
			t.Fatalf("missing %s parameter", name)
		}
		schema, ok := param["schema"].(map[string]any)
		if !ok {
			t.Fatalf("missing schema for %s parameter", name)
		}
		if schema["type"] != "integer" {
			t.Fatalf("unexpected schema type for %s parameter: %v", name, schema["type"])
		}
		if schema["default"] != expectedDefault {
			t.Fatalf("unexpected default for %s parameter: %v", name, schema["default"])
		}
	}

	checkStringParam := func(name string) {
		param, ok := params[name].(map[string]any)
		if !ok {
			t.Fatalf("missing %s parameter", name)
		}
		schema, ok := param["schema"].(map[string]any)
		if !ok {
			t.Fatalf("missing schema for %s parameter", name)
		}
		if schema["type"] != "string" {
			t.Fatalf("unexpected schema type for %s parameter: %v", name, schema["type"])
		}
	}

	checkNumberParam("Limit", 25)
	checkNumberParam("Offset", 0)
	checkStringParam("Include")
	checkStringParam("Select")
	checkStringParam("Order")

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths section missing")
	}

	var listPath map[string]any
	for _, candidate := range []string{"/api/users", "/users"} {
		if path, exists := paths[candidate]; exists {
			if typed, ok := path.(map[string]any); ok {
				listPath = typed
				break
			}
		}
	}
	if listPath == nil {
		t.Fatalf("list path metadata missing")
	}

	getOperation, ok := listPath["get"].(map[string]any)
	if !ok {
		t.Fatalf("GET operation metadata missing")
	}

	rawParams, ok := getOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("list GET parameters missing")
	}

	expectedRefs := map[string]bool{
		"#/components/parameters/Limit":   false,
		"#/components/parameters/Offset":  false,
		"#/components/parameters/Include": false,
		"#/components/parameters/Select":  false,
		"#/components/parameters/Order":   false,
	}

	for _, raw := range rawParams {
		if param, ok := raw.(map[string]any); ok {
			if ref, ok := param["$ref"].(string); ok {
				if _, exists := expectedRefs[ref]; exists {
					expectedRefs[ref] = true
				}
			}
		}
	}

	for ref, seen := range expectedRefs {
		if !seen {
			t.Fatalf("expected parameter reference %s in list GET operation", ref)
		}
	}
}

func TestWebExampleOptionsRespectsPagination(t *testing.T) {
	app, _ := buildExampleTestServer(t)

	makeRequest := func(values url.Values) []map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/api/users?"+values.Encode(), nil)
		resp, err := app.WrappedRouter().Test(req)
		if err != nil {
			t.Fatalf("failed to perform options request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code: %d", resp.StatusCode)
		}

		var payload []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode options payload: %v", err)
		}
		return payload
	}

	baseValues := url.Values{}
	baseValues.Set("format", "options")
	baseValues.Set("order", "name asc,created_at desc")

	allOptions := makeRequest(baseValues)
	if len(allOptions) < 2 {
		t.Fatalf("expected seeded dataset to contain at least two option entries")
	}

	cloneValues := func(src url.Values) url.Values {
		dest := make(url.Values, len(src))
		for k, vals := range src {
			dest[k] = append([]string(nil), vals...)
		}
		return dest
	}

	firstValues := cloneValues(baseValues)
	firstValues.Set("limit", "1")
	firstValues.Set("offset", "0")
	firstPage := makeRequest(firstValues)
	if len(firstPage) == 0 {
		t.Fatalf("expected first page to contain at least one option")
	}
	firstLabel, _ := firstPage[0]["label"].(string)
	if firstLabel == "" {
		t.Fatalf("expected non-empty label on first page")
	}
	if firstLabel != allOptions[0]["label"] {
		t.Fatalf("expected first page label %q to match first overall option %q", firstLabel, allOptions[0]["label"])
	}

	secondValues := cloneValues(baseValues)
	secondValues.Set("limit", "1")
	secondValues.Set("offset", "1")
	secondPage := makeRequest(secondValues)
	if len(secondPage) == 0 {
		t.Fatalf("expected second page to contain at least one option")
	}
	secondLabel, _ := secondPage[0]["label"].(string)
	if secondLabel == "" {
		t.Fatalf("expected non-empty label on second page")
	}
	if secondLabel != allOptions[1]["label"] {
		t.Fatalf("expected second page label %q to match overall second option %q", secondLabel, allOptions[1]["label"])
	}
}

func TestWebExampleProblemJSONErrorResponse(t *testing.T) {
	app, _ := buildExampleTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/user/"+uuid.New().String(), nil)
	resp, err := app.WrappedRouter().Test(req)
	if err != nil {
		t.Fatalf("failed to perform request: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got == "" || !strings.Contains(got, "application/problem+json") {
		t.Fatalf("expected application/problem+json content type, got %q", got)
	}

	var problem goerrors.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("failed to decode problem response: %v", err)
	}
	if problem.Error == nil {
		t.Fatalf("expected error payload in problem response")
	}
	if problem.Error.Category != goerrors.CategoryNotFound {
		t.Fatalf("unexpected category: %s", problem.Error.Category)
	}
}

func TestWebExampleLegacyErrorResponse(t *testing.T) {
	app, _ := buildExampleTestServer(t, crud.WithErrorEncoder[*User](crud.LegacyJSONErrorEncoder()))

	req := httptest.NewRequest(http.MethodGet, "/api/user/"+uuid.New().String(), nil)
	resp, err := app.WrappedRouter().Test(req)
	if err != nil {
		t.Fatalf("failed to perform request: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got == "" || !strings.Contains(got, "application/json") {
		t.Fatalf("expected legacy application/json content type, got %q", got)
	}

	var legacy crud.APIResponse[*User]
	if err := json.NewDecoder(resp.Body).Decode(&legacy); err != nil {
		t.Fatalf("failed to decode legacy error response: %v", err)
	}
	if legacy.Success {
		t.Fatalf("expected success=false in legacy response")
	}
	if legacy.Error == "" {
		t.Fatalf("expected error message in legacy response")
	}
}
