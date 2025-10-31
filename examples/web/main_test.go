package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"
)

func buildExampleTestServer(t *testing.T) (router.Server[*fiber.App], repository.Repository[*User]) {
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

	controller := crud.NewController(repo)
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

	firstValues := url.Values{}
	firstValues.Set("limit", "1")
	firstValues.Set("offset", "0")
	firstValues.Set("format", "options")
	firstValues.Set("order", "name asc,created_at desc")

	firstPage := makeRequest(firstValues)
	if len(firstPage) != 1 {
		t.Fatalf("expected 1 option, got %d", len(firstPage))
	}
	firstLabel, _ := firstPage[0]["label"].(string)
	if firstLabel == "" {
		t.Fatalf("expected non-empty label on first page")
	}

	secondValues := url.Values{}
	secondValues.Set("limit", "1")
	secondValues.Set("offset", "1")
	secondValues.Set("format", "options")
	secondValues.Set("order", "name asc,created_at desc")

	secondPage := makeRequest(secondValues)
	if len(secondPage) != 1 {
		t.Fatalf("expected 1 option on second page, got %d", len(secondPage))
	}
	secondLabel, _ := secondPage[0]["label"].(string)
	if secondLabel == "" {
		t.Fatalf("expected non-empty label on second page")
	}
	if firstLabel == secondLabel {
		t.Fatalf("expected pagination to advance, labels matched: %s", firstLabel)
	}
}
