package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goliatone/go-crud"
	"github.com/stretchr/testify/require"
)

// These include paths previously failed or silently omitted requested children.
func TestRelationshipIncludeAcceptance(t *testing.T) {
	db := setupDatabase()
	defer db.Close()
	repos := registerRepositories(db)
	require.NoError(t, migrateSchema(db))
	require.NoError(t, seedDatabase(context.Background(), db, repos))
	app := newServer(repos)
	app.Init()
	for _, include := range []string{"tags", "books.tags", "books.chapters", "author.profile", "publisher.headquarters"} {
		t.Run(include, func(t *testing.T) {
			path := "/api/authors?include=" + include
			if include == "author.profile" {
				path = "/api/books?include=" + include
			}
			resp, err := app.WrappedRouter().Test(httptest.NewRequest(http.MethodGet, path, nil))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			if include == "author.profile" {
				var result crud.APIListResponse[Book]
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
				require.Len(t, result.Data, 3)
				for _, book := range result.Data {
					require.NotNil(t, book.Author, book.Title)
					require.NotNil(t, book.Author.Profile, book.Title)
					require.NotEmpty(t, book.Author.Profile.Biography, book.Title)
				}
				return
			}
			var result crud.APIListResponse[Author]
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			require.Len(t, result.Data, 3)
			var lina *Author
			for i := range result.Data {
				if result.Data[i].FullName == "Lina Ortiz" {
					lina = &result.Data[i]
				}
			}
			require.NotNil(t, lina)
			switch include {
			case "tags":
				require.Len(t, lina.Tags, 2)
				names := []string{lina.Tags[0].Name, lina.Tags[1].Name}
				require.ElementsMatch(t, []string{"Science Fiction", "Space Opera"}, names)
			case "books.tags":
				require.Len(t, lina.Books, 2)
				for _, book := range lina.Books {
					require.NotEmpty(t, book.Tags, book.Title)
				}
			case "books.chapters":
				require.Len(t, lina.Books, 2)
				var chapters []string
				for _, book := range lina.Books {
					if book.Title == "Contact Shadows" {
						for _, chapter := range book.Chapters {
							chapters = append(chapters, chapter.Title)
						}
					}
				}
				require.ElementsMatch(t, []string{"Signal Twelve", "Embassy in Orbit"}, chapters)
			case "publisher.headquarters":
				require.NotNil(t, lina.Publisher)
				require.Equal(t, "Aurora Press", lina.Publisher.Name)
				require.NotNil(t, lina.Publisher.Headquarters)
				require.Equal(t, "Seattle", lina.Publisher.Headquarters.City)
			}
		})
	}
}
