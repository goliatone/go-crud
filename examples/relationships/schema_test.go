package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorSchemaIncludesHeadquartersComponent(t *testing.T) {
	db := setupDatabase()
	defer db.Close()

	repos := registerRepositories(db)
	require.NoError(t, migrateSchema(db))
	require.NoError(t, seedDatabase(context.Background(), db, repos))

	app := newServer(repos)

	app.Init()

	req := httptest.NewRequest(http.MethodGet, "/api/author/schema", nil)
	resp, err := app.WrappedRouter().Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
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
