package metadata

import (
	"encoding/json"
	"testing"

	"github.com/goliatone/go-crud"
	"github.com/stretchr/testify/require"
)

func TestFromSchemaEntries_NestedPropertiesAndRelationHints(t *testing.T) {
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"components": {"schemas": {"article": {
			"required": ["owner_id"],
			"properties": {
				"owner_id": {"type": "string", "x-relationships": {"type": "belongsTo", "target": "user"}},
				"owner": {"allOf": [{"$ref": "#/components/schemas/user"}], "nullable": true,
					"x-relationships": {"type": "belongsTo", "sourceField": "owner_id"}},
				"teams": {"type": "array", "items": {"$ref": "#/components/schemas/team"},
					"x-relationships": {"type": "hasMany", "target": "team"}},
				"settings": {"type": "object", "properties": {
					"enabled": {"type": "boolean", "readOnly": true},
					"history": {"type": "array", "items": {"type": "object", "properties": {
						"note": {"type": "string", "nullable": true}
					}}}
				}},
				"invalid": {"type": "integer", "nullable": "true", "readOnly": 1}
			}
		}}}
	}`), &doc))

	schemas, err := FromSchemaEntries([]crud.SchemaEntry{{Resource: "article", Document: doc}})
	require.NoError(t, err)
	require.Len(t, schemas, 1)
	schema := schemas[0]
	require.True(t, schema.Properties["owner_id"].Required)
	require.NotContains(t, schema.Relationships, "owner_id", "foreign key hints must not create navigation fields")
	require.Contains(t, schema.Relationships, "owner")
	require.Equal(t, "user", schema.Relationships["owner"].RelatedSchema)
	require.Equal(t, "one", schema.Relationships["owner"].Cardinality)
	require.Equal(t, "owner_id", schema.Relationships["owner"].SourceField)
	require.True(t, schema.Properties["owner"].Nullable)
	require.Equal(t, "object", schema.Properties["owner"].Type)
	require.Contains(t, schema.Relationships, "teams")
	require.True(t, schema.Relationships["teams"].IsSlice)
	require.Equal(t, "many", schema.Relationships["teams"].Cardinality)
	require.NotNil(t, schema.Properties["teams"].Items)
	require.Equal(t, "team", schema.Properties["teams"].Items.RelatedSchema)
	settings := schema.Properties["settings"].Properties
	require.True(t, settings["enabled"].ReadOnly)
	require.NotNil(t, settings["history"].Items)
	require.True(t, settings["history"].Items.Properties["note"].Nullable)
	require.False(t, schema.Properties["invalid"].Nullable)
	require.False(t, schema.Properties["invalid"].ReadOnly)
}
