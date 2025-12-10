package crud

import (
	"reflect"
	"strings"
	"time"
)

// annotateVirtualFieldsInSchema augments the generated OpenAPI document with virtual field properties.
func annotateVirtualFieldsInSchema(doc map[string]any, schemaName string, modelType reflect.Type) {
	if len(doc) == 0 || schemaName == "" {
		return
	}

	props, schema := ensureSchemaProperties(doc, schemaName)
	if props == nil {
		return
	}

	for _, def := range extractVirtualFieldDefsFromType(modelType) {
		if _, exists := props[def.JSONName]; !exists {
			props[def.JSONName] = buildVirtualProperty(def)
			continue
		}
		// Ensure extensions are present even if property already exists.
		if prop, ok := props[def.JSONName].(map[string]any); ok {
			appendVirtualExtensions(prop, def)
		}
	}

	// ensure schema gets written back (maps are shared but be explicit)
	schema["properties"] = props
}

func ensureSchemaProperties(doc map[string]any, schemaName string) (map[string]any, map[string]any) {
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return nil, nil
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil, nil
	}
	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		return nil, nil
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	return props, schema
}

func extractVirtualFieldDefsFromType(modelType reflect.Type) []VirtualFieldDef {
	if modelType == nil {
		return nil
	}
	return extractVirtualFieldDefsForType(modelType, tagOptionAllowZero)
}

func extractVirtualFieldDefsForType(modelType reflect.Type, allowZeroTag string) []VirtualFieldDef {
	var defs []VirtualFieldDef
	t := modelType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		crudTag := field.Tag.Get(TAG_CRUD)
		if !strings.HasPrefix(crudTag, tagVirtualPrefix) {
			continue
		}
		def, ok := buildVirtualFieldDef(field, i, crudTag, allowZeroTag)
		if !ok {
			continue
		}
		if !hasStringMapField(t, def.SourceField) {
			continue
		}
		defs = append(defs, def)
	}
	return defs
}

func buildVirtualProperty(def VirtualFieldDef) map[string]any {
	prop := map[string]any{}
	appendVirtualExtensions(prop, def)
	for k, v := range mapGoTypeToOpenAPI(def.FieldType) {
		prop[k] = v
	}
	return prop
}

func appendVirtualExtensions(prop map[string]any, def VirtualFieldDef) {
	if prop == nil {
		return
	}
	prop["x-virtual-field"] = true
	prop["x-virtual-source"] = def.SourceField
	prop["x-virtual-key"] = def.JSONName
	if def.MergeStrategy != "" {
		prop["x-virtual-merge"] = def.MergeStrategy
	}
}

func mapGoTypeToOpenAPI(t reflect.Type) map[string]any {
	schema := map[string]any{}
	t = indirectType(t)
	switch t.Kind() {
	case reflect.String:
		schema["type"] = "string"
	case reflect.Bool:
		schema["type"] = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"
	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"
	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		schema["items"] = mapGoTypeToOpenAPI(t.Elem())
	case reflect.Map:
		schema["type"] = "object"
	case reflect.Struct:
		if t == reflect.TypeOf(time.Time{}) {
			schema["type"] = "string"
			schema["format"] = "date-time"
			break
		}
		schema["type"] = "object"
	default:
		schema["type"] = "string"
	}
	return schema
}
