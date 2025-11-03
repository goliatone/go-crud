package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-router"
	"github.com/stretchr/testify/require"
)

func TestAuthorSchemaIncludesHeadquartersComponent(t *testing.T) {
	db := setupDatabase()
	defer db.Close()

	repos := registerRepositories(db)
	require.NoError(t, migrateSchema(db))
	require.NoError(t, seedDatabase(context.Background(), db, repos))

	app := router.NewFiberAdapter(func(*fiber.App) *fiber.App {
		return fiber.New()
	})

	api := app.Router().Group("/api")
	adapter := crud.NewGoRouterAdapter(api)

	crud.NewController(repos.Publishers).RegisterRoutes(adapter)
	crud.NewController(repos.Headquarters).RegisterRoutes(adapter)
	crud.NewController(repos.Authors).RegisterRoutes(adapter)
	crud.NewController(repos.AuthorProfiles).RegisterRoutes(adapter)
	crud.NewController(repos.Books).RegisterRoutes(adapter)
	crud.NewController(repos.Chapters).RegisterRoutes(adapter)
	crud.NewController(repos.Tags).RegisterRoutes(adapter)

	app.Init()

	req := httptest.NewRequest(http.MethodGet, "/api/author/schema", nil)
	resp, err := app.WrappedRouter().Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var doc map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&doc))

	components, ok := doc["components"].(map[string]any)
	require.True(t, ok, "components section missing")

	schemas, ok := components["schemas"].(map[string]any)
	require.True(t, ok, "schemas section missing")

	require.Contains(t, schemas, "headquarters", "expected headquarters schema component to be present")
	require.Contains(t, schemas, "chapter", "expected chapter schema component to be present")
	require.Contains(t, schemas, "author-profile", "expected author-profile schema component to be present")
}
