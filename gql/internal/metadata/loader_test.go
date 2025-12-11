package metadata

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud"
)

func TestFromReader_ParsesSchemaArray(t *testing.T) {
	payload := `
[
	{
		"entity_name": "alpha",
		"description": "first schema",
		"required": ["id"],
		"properties": {
			"id": {"type": "string", "format": "uuid"},
			"name": {"type": "string"}
		}
	},
	{
		"entity_name": "beta",
		"properties": {
			"id": {"type": "integer"},
			"score": {"type": "number"}
		}
	}
]`

	schemas, err := FromReader(strings.NewReader(payload))
	require.NoError(t, err)
	require.Len(t, schemas, 2)

	assert.Equal(t, "alpha", schemas[0].Name)
	assert.Equal(t, "beta", schemas[1].Name)
	assert.True(t, schemas[0].Properties["id"].Required)
	assert.Equal(t, "uuid", schemas[0].Properties["id"].Format)
}

func TestFromSchemaEntries_ConvertsOpenAPIDoc(t *testing.T) {
	doc := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{
				"sample": map[string]any{
					"description":           "sample resource",
					"required":              []any{"id"},
					"x-formgen-label-field": "title",
					"properties": map[string]any{
						"id":    map[string]any{"type": "string", "format": "uuid"},
						"title": map[string]any{"type": "string"},
						"owner": map[string]any{
							"type": "object",
							"x-relationships": map[string]any{
								"type":        "belongsTo",
								"target":      "#/components/schemas/user",
								"cardinality": "one",
								"sourceField": "owner_id",
							},
						},
						"tags": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
						},
					},
				},
				"user": map[string]any{
					"properties": map[string]any{
						"id": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	entries := []crud.SchemaEntry{{Resource: "sample", Document: doc}}
	schemas, err := FromSchemaEntries(entries)
	require.NoError(t, err)
	require.Len(t, schemas, 2)

	sample := schemas[0]
	assert.Equal(t, "sample", sample.Name)
	assert.Equal(t, "title", sample.LabelField)
	require.Contains(t, sample.Relationships, "owner")
	assert.Equal(t, "user", sample.Relationships["owner"].RelatedSchema)
	assert.Equal(t, "owner_id", sample.Relationships["owner"].SourceField)

	assert.True(t, sample.Properties["id"].Required)
	assert.NotNil(t, sample.Properties["tags"].Items)
}

func TestFromFile_LoadsSchemasFixture(t *testing.T) {
	schemas, err := FromFile("testdata/metadata.json")
	require.NoError(t, err)
	require.Len(t, schemas, 2)

	assert.Equal(t, "article", schemas[0].Name)
	assert.Equal(t, "comment", schemas[1].Name)
	assert.Equal(t, "date-time", schemas[0].Properties["published_at"].Format)
	assert.True(t, schemas[0].Properties["id"].Required)
	assert.True(t, schemas[1].Properties["body"].Nullable)
}
