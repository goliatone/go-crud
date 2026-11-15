package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-router"
	"github.com/stretchr/testify/require"
)

func TestRelationshipLiveHTTP(t *testing.T) {
	db := setupDatabase()
	defer db.Close()
	repos := registerRepositories(db)
	require.NoError(t, migrateSchema(db))
	require.NoError(t, seedDatabase(context.Background(), db, repos))
	app := newServer(repos)
	run, cancel, done := startObserved(t, app.(router.ManagedServer), 2*time.Second, nil)
	await(t, run.Serving())
	client := &http.Client{Timeout: 3 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	request := func(method, path, payload string, status int) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), method, "http://"+run.Address()+path, bytes.NewBufferString(payload))
		require.NoError(t, err)
		if payload != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, status, resp.StatusCode, "%s %s: %s", method, path, body)
		return body
	}
	var authors crud.APIListResponse[Author]
	require.NoError(t, json.Unmarshal(request("GET", "/api/authors?include=profile,books,publisher", "", 200), &authors))
	require.Len(t, authors.Data, 3)
	var lina *Author
	for i := range authors.Data {
		if authors.Data[i].FullName == "Lina Ortiz" {
			lina = &authors.Data[i]
		}
	}
	require.NotNil(t, lina)
	require.NotNil(t, lina.Profile)
	require.Len(t, lina.Books, 2)
	require.NotNil(t, lina.Publisher)
	var books crud.APIListResponse[Book]
	require.NoError(t, json.Unmarshal(request("GET", "/api/books?include=chapters,publisher", "", 200), &books))
	require.Len(t, books.Data, 3)
	for _, book := range books.Data {
		if book.Title == "Contact Shadows" {
			require.Len(t, book.Chapters, 2)
		}
	}
	var created Tag
	require.NoError(t, json.Unmarshal(request("POST", "/api/tag", `{"name":"Live smoke","category":"test"}`, 201), &created))
	path := "/api/tag/" + created.ID.String()
	request("PUT", path, `{"name":"Updated smoke","category":"test"}`, 200)
	var fetched crud.APIResponse[Tag]
	require.NoError(t, json.Unmarshal(request("GET", path, "", 200), &fetched))
	require.Equal(t, "Updated smoke", fetched.Data.Name)
	request("DELETE", path, "", 204)
	request("GET", path, "", 404)
	request("POST", "/api/tag", `{"name":`, 422)
	for _, path := range []string{"/api/author/schema", "/openapi.json"} {
		var doc map[string]any
		require.NoError(t, json.Unmarshal(request("GET", path, "", 200), &doc))
		require.NotEmpty(t, doc["paths"])
		require.NotEmpty(t, doc["openapi"])
	}
	require.Contains(t, string(request("GET", "/openapi.yaml", "", 200)), "openapi:")
	require.Contains(t, string(request("GET", "/meta/docs/", "", 200)), "html")
	request("GET", "/missing", "", 404)
	cancel()
	require.NoError(t, receiveResult(t, done))
	await(t, run.Done())
}
