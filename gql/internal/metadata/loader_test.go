package metadata

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-router"
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

func TestFromSchemaEntries_ParsesUnionMetadata(t *testing.T) {
	doc := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{
				"blog_post": map[string]any{
					"x-gql": map[string]any{
						"unionTypeMap": map[string]any{
							"hero": "HeroBlockOverride",
						},
					},
					"properties": map[string]any{
						"blocks": map[string]any{
							"type": "array",
							"items": map[string]any{
								"oneOf": []any{
									map[string]any{"$ref": "#/components/schemas/hero_block"},
									map[string]any{"$ref": "#/components/schemas/rich_text_block"},
								},
							},
						},
					},
				},
				"hero_block": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"_type": map[string]any{"const": "hero"},
					},
				},
				"rich_text_block": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"_type": map[string]any{"enum": []any{"rich_text"}},
					},
				},
			},
		},
	}

	entries := []crud.SchemaEntry{{Resource: "blog_post", Document: doc}}
	schemas, err := FromSchemaEntries(entries)
	require.NoError(t, err)
	require.NotEmpty(t, schemas)

	var blog router.SchemaMetadata
	for _, schema := range schemas {
		if schema.Name == "blog_post" {
			blog = schema
			break
		}
	}
	require.NotEmpty(t, blog.Name)

	prop, ok := blog.Properties["blocks"]
	require.True(t, ok)

	members := unionMembersFromProperty(prop)
	require.Equal(t, []string{"hero_block", "rich_text_block"}, members)

	rawDiscriminators, ok := prop.CustomTagData[unionDiscriminatorKey]
	require.True(t, ok)
	discMap, ok := rawDiscriminators.(map[string]string)
	if !ok {
		if rawMap, ok := rawDiscriminators.(map[string]any); ok {
			discMap = toStringMap(rawMap)
		}
	}
	require.NotNil(t, discMap)
	require.Equal(t, "hero_block", discMap["hero"])
	require.Equal(t, "rich_text_block", discMap["rich_text"])

	rawOverrides, ok := prop.CustomTagData[unionOverridesKey]
	require.True(t, ok)
	overrides, ok := rawOverrides.(map[string]string)
	if !ok {
		if rawMap, ok := rawOverrides.(map[string]any); ok {
			overrides = toStringMap(rawMap)
		}
	}
	require.Equal(t, "HeroBlockOverride", overrides["hero"])
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
