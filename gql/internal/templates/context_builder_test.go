package templates

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-router"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/metadata"
)

func TestBuildContext_UsesPivotMetadataForManyToMany(t *testing.T) {
	schemas := []router.SchemaMetadata{
		{
			Name:     "author",
			Required: []string{"id"},
			Properties: map[string]router.PropertyInfo{
				"id":   {Type: "string", Format: "uuid"},
				"tags": {Type: "array", Items: &router.PropertyInfo{Type: "object", RelatedSchema: "tag"}},
			},
			Relationships: map[string]*router.RelationshipInfo{
				"tags": {
					RelationType:      "many-to-many",
					Cardinality:       "many",
					RelatedSchema:     "tag",
					IsSlice:           true,
					PivotTable:        "author_tags",
					SourcePivotColumn: "author_id",
					TargetPivotColumn: "tag_id",
					TargetTable:       "tags",
					SourceTable:       "authors",
					SourceColumn:      "id",
				},
			},
		},
		{
			Name:     "book",
			Required: []string{"id"},
			Properties: map[string]router.PropertyInfo{
				"id":    {Type: "string", Format: "uuid"},
				"title": {Type: "string"},
				"tags":  {Type: "array", Items: &router.PropertyInfo{Type: "object", RelatedSchema: "tag"}},
			},
			Relationships: map[string]*router.RelationshipInfo{
				"tags": {
					RelationType:      "many-to-many",
					Cardinality:       "many",
					RelatedSchema:     "tag",
					IsSlice:           true,
					PivotTable:        "book_tags",
					SourcePivotColumn: "book_id",
					TargetPivotColumn: "tag_id",
					TargetTable:       "tags",
					SourceTable:       "books",
					SourceColumn:      "id",
				},
			},
		},
		{
			Name:     "tag",
			Required: []string{"id"},
			Properties: map[string]router.PropertyInfo{
				"id":      {Type: "string", Format: "uuid"},
				"name":    {Type: "string"},
				"books":   {Type: "array", Items: &router.PropertyInfo{Type: "object", RelatedSchema: "book"}},
				"authors": {Type: "array", Items: &router.PropertyInfo{Type: "object", RelatedSchema: "author"}},
			},
			Relationships: map[string]*router.RelationshipInfo{
				// Intentionally omit pivot details; they should be inferred from the counterpart relations.
				"books": {
					RelationType:  "many-to-many",
					Cardinality:   "many",
					RelatedSchema: "book",
					IsSlice:       true,
				},
				"authors": {
					RelationType:  "many-to-many",
					Cardinality:   "many",
					RelatedSchema: "author",
					IsSlice:       true,
				},
			},
		},
	}

	doc, err := formatter.Format(schemas)
	require.NoError(t, err)

	ctx := BuildContext(doc, ContextOptions{
		ConfigPath: "gqlgen.yml",
		OutDir:     "graph",
	})

	tagCriteria := ctx.Criteria["Tag"]
	require.NotNil(t, tagCriteria)

	assertPivot(t, tagCriteria, "books.id", "book_tags", "tag_id", "book_id")
	assertPivot(t, tagCriteria, "authors.id", "author_tags", "tag_id", "author_id")

	bookCriteria := ctx.Criteria["Book"]
	require.NotNil(t, bookCriteria)
	assertPivot(t, bookCriteria, "tags.id", "book_tags", "book_id", "tag_id")

	authorCriteria := ctx.Criteria["Author"]
	require.NotNil(t, authorCriteria)
	assertPivot(t, authorCriteria, "tags.id", "author_tags", "author_id", "tag_id")

	tagLoader := findLoader(ctx.DataloaderEntities, "Tag")
	require.NotNil(t, tagLoader)

	assertLoaderPivot(t, tagLoader.Relations, "books", "book_tags", "tag_id", "book_id")
	assertLoaderPivot(t, tagLoader.Relations, "authors", "author_tags", "tag_id", "author_id")
}

func assertPivot(t *testing.T, fields []CriteriaField, field string, pivot string, sourcePivot string, targetPivot string) {
	t.Helper()
	item := findCriteriaField(fields, field)
	require.NotNil(t, item, "criteria missing %s", field)
	require.Equal(t, pivot, item.PivotTable)
	require.Equal(t, sourcePivot, item.SourcePivot)
	require.Equal(t, targetPivot, item.TargetPivot)
}

func findCriteriaField(fields []CriteriaField, field string) *CriteriaField {
	for i := range fields {
		if strings.EqualFold(fields[i].Field, field) {
			return &fields[i]
		}
	}
	return nil
}

func findLoader(loaders []DataloaderEntity, name string) *DataloaderEntity {
	for i := range loaders {
		if loaders[i].Name == name {
			return &loaders[i]
		}
	}
	return nil
}

func assertLoaderPivot(t *testing.T, relations []DataloaderRelation, name, pivot, sourcePivot, targetPivot string) {
	t.Helper()
	for _, rel := range relations {
		if rel.Name == name {
			require.Equal(t, pivot, rel.PivotTable)
			require.Equal(t, sourcePivot, rel.SourcePivot)
			require.Equal(t, targetPivot, rel.TargetPivot)
			return
		}
	}
	require.Failf(t, "relation not found", "missing relation %s", name)
}

func TestBuildContext_AuthFlags(t *testing.T) {
	schemas, err := metadata.FromFile(filepath.Join("testdata", "metadata.json"))
	require.NoError(t, err)

	doc, err := formatter.Format(schemas)
	require.NoError(t, err)

	ctx := BuildContext(doc, ContextOptions{
		ConfigPath:  "gqlgen.yml",
		OutDir:      "graph",
		AuthPackage: "github.com/goliatone/go-auth",
	})

	require.True(t, ctx.AuthEnabled, "auth should be enabled when auth package is set")
	require.False(t, ctx.HasAuthGuard, "guard should be false when no auth guard expression is provided")
	require.True(t, ctx.AuthImportRequired, "auth import should be required when hooks did not add it")
}
