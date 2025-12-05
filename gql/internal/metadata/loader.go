package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-router"
)

// FromFile reads SchemaMetadata from a JSON file. The payload can be a single schema,
// an array of schemas, or a wrapper object with a "schemas" field.
func FromFile(path string) ([]router.SchemaMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open metadata file: %w", err)
	}
	defer file.Close()

	return FromReader(file)
}

// FromReader decodes SchemaMetadata from an io.Reader.
func FromReader(r io.Reader) ([]router.SchemaMetadata, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	return fromJSONBytes(data)
}

// FromRegistry loads schema metadata from the in-memory schema registry.
func FromRegistry() ([]router.SchemaMetadata, error) {
	return FromSchemaEntries(crud.ListSchemas())
}

// FromSchemaEntries converts schema registry entries into SchemaMetadata instances.
func FromSchemaEntries(entries []crud.SchemaEntry) ([]router.SchemaMetadata, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("no schema entries provided")
	}

	schemas := make(map[string]router.SchemaMetadata)
	for _, entry := range entries {
		if entry.Document == nil {
			continue
		}

		found, err := schemasFromDocument(entry.Document)
		if err != nil {
			return nil, fmt.Errorf("convert schema document for %s: %w", entry.Resource, err)
		}
		for name, schema := range found {
			schemas[name] = schema
		}
	}

	if len(schemas) == 0 {
		return nil, fmt.Errorf("no schemas discovered in registry entries")
	}

	return normalizeSchemasMap(schemas), nil
}

func fromJSONBytes(data []byte) ([]router.SchemaMetadata, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("metadata payload is empty")
	}

	if trimmed[0] == '[' {
		var schemas []router.SchemaMetadata
		if err := json.Unmarshal(trimmed, &schemas); err == nil {
			return normalizeSchemas(schemas), nil
		}

		var entries []crud.SchemaEntry
		if err := json.Unmarshal(trimmed, &entries); err == nil {
			return FromSchemaEntries(entries)
		}
		return nil, fmt.Errorf("unable to decode schema array")
	}

	var wrapper struct {
		Schemas []router.SchemaMetadata `json:"schemas"`
		Entries []crud.SchemaEntry      `json:"entries"`
		Schema  router.SchemaMetadata   `json:"schema"`
	}
	if err := json.Unmarshal(trimmed, &wrapper); err == nil {
		switch {
		case len(wrapper.Schemas) > 0:
			return normalizeSchemas(wrapper.Schemas), nil
		case len(wrapper.Entries) > 0:
			return FromSchemaEntries(wrapper.Entries)
		case wrapper.Schema.Name != "":
			return normalizeSchemas([]router.SchemaMetadata{wrapper.Schema}), nil
		}
	}

	var single router.SchemaMetadata
	if err := json.Unmarshal(trimmed, &single); err == nil && single.Name != "" {
		return normalizeSchemas([]router.SchemaMetadata{single}), nil
	}

	return nil, fmt.Errorf("unsupported metadata JSON shape")
}

func schemasFromDocument(doc map[string]any) (map[string]router.SchemaMetadata, error) {
	components, ok := doc["components"].(map[string]any)
	if !ok || len(components) == 0 {
		return nil, fmt.Errorf("missing components section")
	}

	rawSchemas, ok := components["schemas"].(map[string]any)
	if !ok || len(rawSchemas) == 0 {
		return nil, fmt.Errorf("missing components.schemas")
	}

	result := make(map[string]router.SchemaMetadata, len(rawSchemas))
	for name, raw := range rawSchemas {
		rawMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		result[name] = schemaFromOpenAPI(name, rawMap)
	}
	return result, nil
}

func schemaFromOpenAPI(name string, raw map[string]any) router.SchemaMetadata {
	schema := router.SchemaMetadata{
		Name:          name,
		Description:   stringValue(raw["description"]),
		LabelField:    labelFromExtensions(raw),
		Properties:    make(map[string]router.PropertyInfo),
		Relationships: make(map[string]*router.RelationshipInfo),
	}

	required := toStringSlice(raw["required"])
	requiredSet := make(map[string]struct{}, len(required))
	for _, r := range required {
		requiredSet[strings.ToLower(r)] = struct{}{}
	}
	schema.Required = required

	props, _ := raw["properties"].(map[string]any)
	for propName, propVal := range props {
		propMap, ok := propVal.(map[string]any)
		if !ok {
			continue
		}
		propInfo, rel := propertyFromOpenAPI(propName, propMap)
		if _, ok := requiredSet[strings.ToLower(propName)]; ok {
			propInfo.Required = true
		}
		schema.Properties[propName] = propInfo
		if rel != nil {
			schema.Relationships[propName] = rel
		}
	}

	if len(schema.Relationships) == 0 {
		schema.Relationships = nil
	}
	if len(schema.Properties) == 0 {
		schema.Properties = nil
	}

	return schema
}

func propertyFromOpenAPI(name string, raw map[string]any) (router.PropertyInfo, *router.RelationshipInfo) {
	prop := router.PropertyInfo{
		Type:        stringValue(raw["type"]),
		Format:      stringValue(raw["format"]),
		Description: stringValue(raw["description"]),
		ReadOnly:    boolValue(raw["readOnly"]),
		WriteOnly:   boolValue(raw["writeOnly"]),
		Nullable:    boolValue(raw["nullable"]),
		Example:     raw["example"],
	}

	if nested, ok := raw["properties"].(map[string]any); ok && len(nested) > 0 {
		prop.Properties = make(map[string]router.PropertyInfo, len(nested))
		for nestedName, nestedVal := range nested {
			if nestedMap, ok := nestedVal.(map[string]any); ok {
				nestedProp, _ := propertyFromOpenAPI(nestedName, nestedMap)
				prop.Properties[nestedName] = nestedProp
			}
		}
	}

	if items, ok := raw["items"].(map[string]any); ok {
		itemProp, itemRel := propertyFromOpenAPI(name, items)
		prop.Items = &itemProp
		if itemRel != nil && prop.RelatedSchema == "" {
			prop.RelatedSchema = itemRel.RelatedSchema
		}
	}

	if ref := schemaNameFromRef(raw); ref != "" {
		prop.RelatedSchema = ref
		if prop.Type == "" {
			prop.Type = "object"
		}
	}

	rel := relationFromExtension(raw)
	if rel != nil {
		if rel.RelatedSchema == "" && prop.RelatedSchema != "" {
			rel.RelatedSchema = prop.RelatedSchema
		}
		if prop.Items != nil && !rel.IsSlice {
			rel.IsSlice = true
		}
		rel.Cardinality = normalizeCardinality(rel.Cardinality, rel.IsSlice || prop.Items != nil)
		prop.RelationName = name
		if rel.RelatedSchema != "" {
			prop.RelatedSchema = rel.RelatedSchema
		}
	}

	return prop, rel
}

func relationFromExtension(raw map[string]any) *router.RelationshipInfo {
	ext, ok := raw["x-relationships"].(map[string]any)
	if !ok || len(ext) == 0 {
		return nil
	}

	rel := &router.RelationshipInfo{}

	if val, ok := ext["type"].(string); ok {
		rel.RelationType = val
	}
	if target, ok := ext["target"].(string); ok {
		rel.RelatedSchema = schemaFromRef(target)
	}
	if fk, ok := ext["foreignKey"].(string); ok {
		rel.ForeignKey = fk
	}
	if source, ok := ext["sourceField"].(string); ok {
		rel.SourceField = source
	}
	if inverse, ok := ext["inverse"].(string); ok {
		rel.Inverse = inverse
	}
	if card, ok := ext["cardinality"].(string); ok {
		rel.Cardinality = card
	}

	if rel.Cardinality != "" {
		rel.IsSlice = strings.Contains(strings.ToLower(rel.Cardinality), "many")
	}

	if rel.RelationType == "" && rel.RelatedSchema == "" && rel.ForeignKey == "" && rel.SourceField == "" && rel.Inverse == "" && rel.Cardinality == "" {
		return nil
	}

	return rel
}

func schemaNameFromRef(raw map[string]any) string {
	if ref, ok := raw["$ref"].(string); ok {
		return schemaFromRef(ref)
	}

	if allOf, ok := raw["allOf"].([]any); ok {
		for _, entry := range allOf {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if ref, ok := entryMap["$ref"].(string); ok {
				if name := schemaFromRef(ref); name != "" {
					return name
				}
			}
		}
	}

	return ""
}

func schemaFromRef(ref string) string {
	if ref == "" {
		return ""
	}
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

func normalizeCardinality(cardinality string, isSlice bool) string {
	cardinality = strings.ToLower(strings.TrimSpace(cardinality))
	if cardinality == "" {
		if isSlice {
			return "many"
		}
		return "one"
	}
	if strings.Contains(cardinality, "many") {
		return "many"
	}
	return cardinality
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	val, _ := v.(string)
	return val
}

func boolValue(v any) bool {
	val, _ := v.(bool)
	return val
}

func labelFromExtensions(raw map[string]any) string {
	if label, ok := raw["x-formgen-label-field"].(string); ok {
		return label
	}
	return ""
}

func toStringSlice(value any) []string {
	rawSlice, ok := value.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(rawSlice))
	for _, val := range rawSlice {
		if s, ok := val.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func normalizeSchemas(schemas []router.SchemaMetadata) []router.SchemaMetadata {
	if len(schemas) == 0 {
		return nil
	}

	normalized := make([]router.SchemaMetadata, 0, len(schemas))
	for _, schema := range schemas {
		normalized = append(normalized, applyRequiredFlags(schema))
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Name < normalized[j].Name
	})
	return normalized
}

func normalizeSchemasMap(schemas map[string]router.SchemaMetadata) []router.SchemaMetadata {
	if len(schemas) == 0 {
		return nil
	}

	result := make([]router.SchemaMetadata, 0, len(schemas))
	for _, schema := range schemas {
		result = append(result, applyRequiredFlags(schema))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func applyRequiredFlags(schema router.SchemaMetadata) router.SchemaMetadata {
	if len(schema.Required) == 0 || len(schema.Properties) == 0 {
		return schema
	}

	required := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		required[strings.ToLower(name)] = struct{}{}
	}

	props := make(map[string]router.PropertyInfo, len(schema.Properties))
	for name, prop := range schema.Properties {
		if _, ok := required[strings.ToLower(name)]; ok {
			prop.Required = true
		}
		props[name] = prop
	}
	schema.Properties = props
	return schema
}
