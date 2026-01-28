package crud

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterSchemaDocument(t *testing.T) {
	resetSchemaRegistry()

	doc := map[string]any{"openapi": "3.0.0"}
	ok := RegisterSchemaDocument("article", "articles", doc)
	require.True(t, ok, "expected registration to succeed")

	entry, found := GetSchema("article")
	require.True(t, found, "expected schema to be registered")
	assert.Equal(t, "articles", entry.Plural)
	assert.Equal(t, "3.0.0", entry.Document["openapi"])
	assert.False(t, entry.UpdatedAt.IsZero())

	doc["openapi"] = "3.0.1"
	entry, found = GetSchema("article")
	require.True(t, found, "expected schema to remain registered")
	assert.Equal(t, "3.0.0", entry.Document["openapi"], "expected registry entry to be cloned")
}

func TestExportSchemas(t *testing.T) {
	resetSchemaRegistry()

	RegisterSchemaDocument("beta", "", map[string]any{"openapi": "3.0.0"})
	RegisterSchemaDocument("alpha", "", map[string]any{"openapi": "3.0.0"})

	var buf bytes.Buffer
	err := ExportSchemas(&buf, WithSchemaExportIndent("  "))
	require.NoError(t, err)

	var entries []SchemaEntry
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entries))
	require.Len(t, entries, 2)
	assert.Equal(t, "alpha", entries[0].Resource)
	assert.Equal(t, "beta", entries[1].Resource)
}

func TestRegisterSchemaDocumentStoresCMSProjection(t *testing.T) {
	resetSchemaRegistry()

	doc := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "Article",
			"version": "1.0.0",
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"article": map[string]any{
					"type": "object",
				},
			},
		},
		"x-cms": map[string]any{
			"content_type": "article",
			"schema":       "article@v1.0.0",
		},
	}

	ok := RegisterSchemaDocument("article", "articles", doc)
	require.True(t, ok, "expected registration to succeed")

	entry, found := GetSchema("article")
	require.True(t, found, "expected schema to be registered")
	meta, ok := entry.Document["x-cms"].(map[string]any)
	require.True(t, ok, "expected x-cms metadata")
	assert.Equal(t, "article", meta["content_type"])
	assert.Equal(t, "article@v1.0.0", meta["schema"])
}
