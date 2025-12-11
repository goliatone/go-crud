package formatter

import (
	"strings"

	"github.com/goliatone/go-router"
)

// TypeRef describes a scalar mapping key composed of JSON type, format, and Go type.
type TypeRef struct {
	Type   string
	Format string
	GoType string
}

// TypeMapping maps router.PropertyInfo definitions to GraphQL scalar names.
type TypeMapping struct {
	defaults  map[string]string
	overrides map[string]string
}

func defaultTypeMapping() TypeMapping {
	defaults := map[string]string{
		makeTypeKey("string", "uuid", ""):      "UUID",
		makeTypeKey("string", "date-time", ""): "Time",
		makeTypeKey("string", "date", ""):      "Date",
		makeTypeKey("string", "", ""):          "String",
		makeTypeKey("integer", "", ""):         "Int",
		makeTypeKey("number", "", ""):          "Float",
		makeTypeKey("boolean", "", ""):         "Boolean",
		makeTypeKey("object", "", ""):          "JSON",
	}
	return TypeMapping{
		defaults:  defaults,
		overrides: make(map[string]string),
	}
}

// Resolve returns the GraphQL scalar name for the given property.
func (m TypeMapping) Resolve(prop router.PropertyInfo) string {
	ref := TypeRefFromProperty(prop)

	for _, key := range []string{
		makeTypeKey(ref.Type, ref.Format, ref.GoType),
		makeTypeKey("", "", ref.GoType),
		makeTypeKey(ref.Type, ref.Format, ""),
		makeTypeKey(ref.Type, "", ref.GoType),
		makeTypeKey(ref.Type, "", ""),
	} {
		if scalar, ok := m.overrides[key]; ok && scalar != "" {
			return scalar
		}
	}

	for _, key := range []string{
		makeTypeKey(ref.Type, ref.Format, ""),
		makeTypeKey(ref.Type, "", ""),
	} {
		if scalar, ok := m.defaults[key]; ok {
			return scalar
		}
	}

	if scalar, ok := m.defaults[makeTypeKey("string", "", "")]; ok {
		return scalar
	}

	return "String"
}

// WithOverrides returns a copy of the mapping with the provided overrides applied.
func (m TypeMapping) WithOverrides(overrides map[TypeRef]string) TypeMapping {
	if len(overrides) == 0 {
		return m
	}

	next := TypeMapping{
		defaults:  m.defaults,
		overrides: make(map[string]string, len(m.overrides)+len(overrides)),
	}

	for key, val := range m.overrides {
		next.overrides[key] = val
	}

	for ref, scalar := range overrides {
		key := makeTypeKey(strings.ToLower(ref.Type), strings.ToLower(ref.Format), strings.ToLower(ref.GoType))
		next.overrides[key] = scalar
	}
	return next
}

// TypeRefFromProperty builds a TypeRef from a router.PropertyInfo instance.
func TypeRefFromProperty(prop router.PropertyInfo) TypeRef {
	return TypeRef{
		Type:   strings.ToLower(prop.Type),
		Format: strings.ToLower(prop.Format),
		GoType: strings.ToLower(prop.OriginalType),
	}
}

func makeTypeKey(typeName, format, goType string) string {
	return strings.ToLower(typeName) + "|" + strings.ToLower(format) + "|" + strings.ToLower(goType)
}
