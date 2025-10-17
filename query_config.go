package crud

import (
	"maps"
	"reflect"
	"strings"
	"sync"

	"github.com/ettle/strcase"
	"github.com/goliatone/go-repository-bun"
)

type relationMetadata struct {
	relationName string
	fields       map[string]string
	children     map[string]*relationMetadata
}

type queryConfig struct {
	root *relationMetadata
}

type modelFieldsProvider interface {
	GetModelFields() []repository.ModelField
}

var (
	queryConfigRegistry sync.Map // map[reflect.Type]*queryConfig
	fieldsCache         sync.Map // map[reflect.Type]map[string]string
)

func newFieldMapProviderFromRepo(repo any, resourceType reflect.Type) FieldMapProvider {
	mfRepo, ok := repo.(modelFieldsProvider)
	if !ok {
		return nil
	}
	fields := mfRepo.GetModelFields()
	if len(fields) == 0 {
		return nil
	}
	base := indirectType(resourceType)
	if base == nil {
		return nil
	}
	fieldMap := make(map[string]string, len(fields))
	for _, field := range fields {
		key := strcase.ToSnake(field.Name)
		if key == "" {
			continue
		}
		fieldMap[key] = strcase.ToSnake(field.Name)
	}
	if len(fieldMap) == 0 {
		return nil
	}
	return func(t reflect.Type) map[string]string {
		if indirectType(t) != base {
			return nil
		}
		return cloneStringMap(fieldMap)
	}
}

func registerQueryConfig(typ reflect.Type, provider FieldMapProvider) {
	base := indirectType(typ)
	if base.Kind() != reflect.Struct {
		return
	}

	root := buildRelationMetadata(base, provider, make(map[reflect.Type]bool))
	if root == nil {
		return
	}

	queryConfigRegistry.Store(base, &queryConfig{root: root})
}

func getRelationMetadataForType(typ reflect.Type) *relationMetadata {
	base := indirectType(typ)
	if cfg, ok := queryConfigRegistry.Load(base); ok {
		return cfg.(*queryConfig).root
	}

	root := buildRelationMetadata(base, nil, make(map[reflect.Type]bool))
	if root == nil {
		return nil
	}

	queryConfigRegistry.Store(base, &queryConfig{root: root})
	return root
}

func buildRelationMetadata(typ reflect.Type, provider FieldMapProvider, visited map[reflect.Type]bool) *relationMetadata {
	base := indirectType(typ)
	if base.Kind() != reflect.Struct {
		return nil
	}

	if visited[base] {
		// Prevent infinite recursion on cyclical relations
		return &relationMetadata{
			fields:   map[string]string{},
			children: map[string]*relationMetadata{},
		}
	}
	visited[base] = true

	fields := getOrBuildFieldMap(base, provider)

	meta := &relationMetadata{
		fields:   fields,
		children: make(map[string]*relationMetadata),
	}

	for i := 0; i < base.NumField(); i++ {
		field := base.Field(i)
		if !field.IsExported() {
			continue
		}

		if field.Tag.Get(TAG_CRUD) == "-" {
			continue
		}

		bunTag := field.Tag.Get(TAG_BUN)
		if bunTag == "" || !strings.Contains(bunTag, "rel:") {
			continue
		}

		childType := field.Type
		for childType.Kind() == reflect.Ptr || childType.Kind() == reflect.Slice || childType.Kind() == reflect.Array {
			childType = childType.Elem()
		}

		childMeta := buildRelationMetadata(childType, provider, visited)
		if childMeta == nil {
			continue
		}

		childMeta.relationName = field.Name

		registerChild(meta, field, childMeta)
	}

	return meta
}

func registerChild(parent *relationMetadata, field reflect.StructField, child *relationMetadata) {
	if parent.children == nil {
		parent.children = make(map[string]*relationMetadata)
	}

	parent.children[strings.ToLower(field.Name)] = child

	if jsonTag := field.Tag.Get(TAG_JSON); jsonTag != "" {
		alias := strings.Split(jsonTag, ",")[0]
		if alias != "" && alias != "-" {
			parent.children[strings.ToLower(alias)] = child
		}
	}
}

func getOrBuildFieldMap(typ reflect.Type, provider FieldMapProvider) map[string]string {
	if m, ok := fieldsCache.Load(typ); ok {
		return m.(map[string]string)
	}

	fieldMap := buildDefaultFieldMap(typ)

	if provider != nil {
		if provided := provider(typ); len(provided) > 0 {
			providedMap := normalizeFieldMap(provided)
			if len(providedMap) > 0 {
				if fieldMap == nil {
					fieldMap = providedMap
				} else {
					maps.Copy(fieldMap, providedMap)
				}
			}
		}
	}

	if fieldMap == nil {
		fieldMap = make(map[string]string)
	}

	fieldsCache.Store(typ, fieldMap)
	return fieldMap
}

func buildDefaultFieldMap(typ reflect.Type) map[string]string {
	fields := make(map[string]string)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		if field.Tag.Get(TAG_CRUD) == "-" {
			continue
		}

		columnName := strcase.ToSnake(field.Name)
		if bunTag := field.Tag.Get(TAG_BUN); bunTag != "" {
			parts := strings.Split(bunTag, ",")
			if len(parts) > 0 && parts[0] != "" {
				columnName = parts[0]
			}
		}

		jsonKey := strcase.ToSnake(field.Name)
		if jsonTag := field.Tag.Get(TAG_JSON); jsonTag != "" {
			jsonKey = strings.Split(jsonTag, ",")[0]
			if jsonKey == "" {
				jsonKey = strcase.ToSnake(field.Name)
			}
		}

		fields[jsonKey] = columnName
	}
	return fields
}

func normalizeFieldMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func indirectType(t reflect.Type) reflect.Type {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func typeOf[T any]() reflect.Type {
	var zero T
	return reflect.TypeOf(zero)
}
