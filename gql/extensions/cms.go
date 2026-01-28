package extensions

import (
	"fmt"
	"strings"
)

func init() {
	RegisterSchemaExtension(cmsHandler{})
}

type cmsHandler struct{}

func (cmsHandler) Name() string { return "x-cms" }

func (cmsHandler) ApplyDoc(doc map[string]any, ctx *SchemaExtensionContext) {
	if ctx == nil || len(doc) == 0 {
		return
	}
	raw, ok := doc["x-cms"].(map[string]any)
	if !ok {
		return
	}
	meta := cmsMetadataFromMap(raw)
	if meta.ContentType == "" {
		return
	}
	resolved := cmsSchemaIdentifier(meta)
	if resolved == "" {
		return
	}
	if ctx.NameMap == nil {
		ctx.NameMap = map[string]string{}
	}
	ctx.NameMap[meta.ContentType] = resolved
}

func (cmsHandler) ApplySchema(schemaName string, rawSchema map[string]any, ctx *SchemaExtensionContext) {
	if ctx == nil || len(rawSchema) == 0 {
		return
	}
	raw, ok := rawSchema["x-cms"].(map[string]any)
	if !ok {
		return
	}
	meta := cmsMetadataFromMap(raw)
	if meta.ContentType == "" {
		meta.ContentType = schemaName
	}
	resolved := cmsSchemaIdentifier(meta)
	if resolved == "" {
		return
	}
	if ctx.NameMap == nil {
		ctx.NameMap = map[string]string{}
	}
	ctx.NameMap[schemaName] = resolved
	ctx.SchemaName = resolved
}

type cmsMetadata struct {
	ContentType string
	Schema      string
	Version     string
}

func cmsMetadataFromMap(raw map[string]any) cmsMetadata {
	return cmsMetadata{
		ContentType: stringValueFromMap(raw, "content_type", "contentType", "contentTypeSlug", "slug"),
		Schema:      stringValueFromMap(raw, "schema", "schema_version", "schemaVersion"),
		Version:     stringValueFromMap(raw, "version", "schema_version", "schemaVersion"),
	}
}

func cmsSchemaIdentifier(meta cmsMetadata) string {
	if meta.Schema != "" {
		if strings.Contains(meta.Schema, "@") {
			return meta.Schema
		}
		if meta.ContentType != "" && strings.HasPrefix(strings.ToLower(meta.Schema), "v") {
			return fmt.Sprintf("%s@%s", meta.ContentType, meta.Schema)
		}
		if meta.ContentType != "" && isSemverLike(meta.Schema) {
			return fmt.Sprintf("%s@v%s", meta.ContentType, meta.Schema)
		}
		return meta.Schema
	}
	if meta.ContentType != "" && meta.Version != "" {
		version := meta.Version
		if !strings.HasPrefix(strings.ToLower(version), "v") {
			version = "v" + version
		}
		return fmt.Sprintf("%s@%s", meta.ContentType, version)
	}
	return ""
}

func stringValueFromMap(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			if str := strings.TrimSpace(stringValue(val)); str != "" {
				return str
			}
		}
	}
	return ""
}

func stringValue(val any) string {
	switch typed := val.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

func isSemverLike(value string) bool {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 0 || len(parts) > 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for i := 0; i < len(part); i++ {
			if part[i] < '0' || part[i] > '9' {
				return false
			}
		}
	}
	return true
}
