package formatter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ettle/strcase"
	"github.com/goliatone/go-router"
)

const (
	unionMembersKey       = "x-gql-union-members"
	unionDiscriminatorKey = "x-gql-union-discriminator-map"
	unionOverridesKey     = "x-gql-union-type-map"
)

// Document is the template ready representation of a list of schemas.
type Document struct {
	Entities []Entity
	Unions   []Union
}

// Entity represents a GraphQL type derived from a SchemaMetadata entry.
type Entity struct {
	Name          string
	RawName       string
	Description   string
	LabelField    string
	Fields        []Field
	Relationships []Relation
}

// Union represents a GraphQL union derived from oneOf schema definitions.
type Union struct {
	Name           string
	Types          []string
	TypeMap        map[string]string
	TypeMapEntries []UnionTypeMapEntry
	SourceEntity   string
	SourceField    string
}

// UnionTypeMapEntry captures a stable ordering for union discriminator mappings.
type UnionTypeMapEntry struct {
	Key   string
	Value string
}

// Field captures the scalar or relation backed properties of an entity.
type Field struct {
	Name              string
	OriginalName      string
	Type              string
	IsList            bool
	Nullable          bool
	Description       string
	GoType            string
	Required          bool
	ReadOnly          bool
	WriteOnly         bool
	OmitFromMutations bool
	Relation          *Relation
}

// Relation holds the relationship metadata required by templates.
type Relation struct {
	Name             string
	OriginalName     string
	Key              string
	AliasFor         string
	Type             string
	Cardinality      string
	SourceField      string
	SourceColumn     string
	TargetColumn     string
	ForeignKey       string
	Inverse          string
	RelationType     string
	RelatedSchema    string
	IsList           bool
	Nullable         bool
	PivotTable       string
	SourcePivotField string
	TargetPivotField string
	TargetTable      string
}

// NameFormatter converts raw identifiers into template-facing names.
type NameFormatter func(string) string

// Options configure formatting behaviour.
type Options struct {
	fieldNamer  NameFormatter
	typeNamer   NameFormatter
	typeMapping TypeMapping
	pinned      []string
}

// Option customises formatter behaviour.
type Option func(*Options)

// Format turns router.SchemaMetadata entries into deterministic, template-ready structs.
func Format(schemas []router.SchemaMetadata, option ...Option) (Document, error) {
	opts := defaultOptions()
	for _, opt := range option {
		if opt != nil {
			opt(&opts)
		}
	}

	doc := Document{
		Entities: make([]Entity, 0, len(schemas)),
	}
	unionRegistry := make(map[string]Union)
	for _, schema := range schemas {
		entity, unions := formatSchema(schema, opts)
		doc.Entities = append(doc.Entities, entity)
		for _, union := range unions {
			mergeUnion(unionRegistry, union)
		}
	}

	sort.Slice(doc.Entities, func(i, j int) bool {
		return doc.Entities[i].Name < doc.Entities[j].Name
	})

	doc.Unions = collectUnions(unionRegistry)

	return doc, nil
}

// WithFieldNamer configures the function used to render field names.
func WithFieldNamer(namer NameFormatter) Option {
	return func(o *Options) {
		if namer != nil {
			o.fieldNamer = namer
		}
	}
}

// WithTypeNamer configures the function used to render entity/type names.
func WithTypeNamer(namer NameFormatter) Option {
	return func(o *Options) {
		if namer != nil {
			o.typeNamer = namer
		}
	}
}

// WithPinnedFields sets the ordering pins applied before alphabetical sorting.
func WithPinnedFields(names ...string) Option {
	return func(o *Options) {
		o.pinned = append([]string(nil), names...)
	}
}

// WithTypeMappings registers scalar overrides keyed by type/format/Go type.
func WithTypeMappings(overrides map[TypeRef]string) Option {
	return func(o *Options) {
		if len(overrides) == 0 {
			return
		}
		o.typeMapping = o.typeMapping.WithOverrides(overrides)
	}
}

// WithScalarOverride registers a single scalar override.
func WithScalarOverride(ref TypeRef, scalar string) Option {
	return func(o *Options) {
		o.typeMapping = o.typeMapping.WithOverrides(map[TypeRef]string{ref: scalar})
	}
}

func formatSchema(schema router.SchemaMetadata, opts Options) (Entity, []Union) {
	entity := Entity{
		RawName:     schema.Name,
		Name:        formatTypeName(schema.Name, opts),
		Description: schema.Description,
		LabelField:  formatLabel(schema.LabelField, opts.fieldNamer),
	}

	if len(schema.Properties) == 0 {
		return entity, nil
	}

	required := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		required[strings.ToLower(name)] = struct{}{}
	}

	fields := make([]Field, 0, len(schema.Properties))
	unions := make([]Union, 0, len(schema.Properties))
	relations := make(map[string]Relation)

	for propName, prop := range schema.Properties {
		field := buildField(propName, prop, schema, required, opts)
		if union := buildUnion(schema.Name, propName, prop, opts); union != nil {
			field.Type = union.Name
			unions = append(unions, *union)
		}
		if field.Relation != nil {
			relations[propName] = *field.Relation
		}
		fields = append(fields, field)
	}

	orderFields(fields, opts.pinned)
	entity.Fields = fields
	entity.Relationships = collectRelations(relations)
	return entity, unions
}

func buildField(name string, prop router.PropertyInfo, schema router.SchemaMetadata, required map[string]struct{}, opts Options) Field {
	displayName := opts.fieldNamer(name)
	base := prop
	isList := base.Items != nil
	if base.Items != nil {
		base = *base.Items
	}

	relKey, relInfo, isAlias := lookupRelation(name, schema)
	var rel *Relation
	if relInfo != nil {
		rel = buildRelation(name, relKey, relInfo, base, isAlias, opts)
		isList = rel.IsList
	}

	fieldType := opts.typeMapping.Resolve(base)
	if rel != nil && rel.Type != "" {
		fieldType = rel.Type
	}

	requiredField := prop.Required || isRequired(required, name)
	if !requiredField && strings.EqualFold(name, "id") {
		requiredField = true
	}

	return Field{
		Name:         displayName,
		OriginalName: name,
		Type:         fieldType,
		IsList:       isList,
		Nullable:     prop.Nullable,
		Description:  prop.Description,
		GoType:       prop.OriginalType,
		Required:     requiredField,
		ReadOnly:     prop.ReadOnly,
		WriteOnly:    prop.WriteOnly,
		Relation:     rel,
	}
}

func buildUnion(schemaName, propName string, prop router.PropertyInfo, opts Options) *Union {
	members := unionMembers(prop)
	if len(members) == 0 {
		return nil
	}

	unionName := formatTypeName(schemaName, opts) + opts.typeNamer(singularize(propName))
	types := make([]string, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member == "" {
			continue
		}
		typeName := formatTypeName(member, opts)
		if _, ok := seen[typeName]; ok {
			continue
		}
		seen[typeName] = struct{}{}
		types = append(types, typeName)
	}
	sort.Strings(types)

	typeMap := buildUnionTypeMap(prop, opts)
	return &Union{
		Name:           unionName,
		Types:          types,
		TypeMap:        typeMap,
		TypeMapEntries: unionTypeMapEntries(typeMap),
		SourceEntity:   formatTypeName(schemaName, opts),
		SourceField:    opts.fieldNamer(propName),
	}
}

func buildUnionTypeMap(prop router.PropertyInfo, opts Options) map[string]string {
	discriminators := unionDiscriminators(prop)
	overrides := unionOverrides(prop)
	if len(discriminators) == 0 && len(overrides) == 0 {
		return nil
	}
	out := make(map[string]string, len(discriminators)+len(overrides))
	for key, schemaName := range discriminators {
		normalized := normalizeDiscriminatorKey(key)
		if normalized == "" {
			continue
		}
		if schemaName == "" {
			continue
		}
		out[normalized] = formatTypeName(schemaName, opts)
	}
	for key, value := range overrides {
		normalized := normalizeDiscriminatorKey(key)
		if normalized == "" || strings.TrimSpace(value) == "" {
			continue
		}
		out[normalized] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func unionMembers(prop router.PropertyInfo) []string {
	if prop.CustomTagData == nil {
		return nil
	}
	raw := prop.CustomTagData[unionMembersKey]
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if val, ok := entry.(string); ok && strings.TrimSpace(val) != "" {
				out = append(out, val)
			}
		}
		return out
	default:
		return nil
	}
}

func unionDiscriminators(prop router.PropertyInfo) map[string]string {
	if prop.CustomTagData == nil {
		return nil
	}
	raw := prop.CustomTagData[unionDiscriminatorKey]
	switch typed := raw.(type) {
	case map[string]string:
		return cloneStringMap(typed)
	case map[string]any:
		return toStringMap(typed)
	default:
		return nil
	}
}

func unionOverrides(prop router.PropertyInfo) map[string]string {
	if prop.CustomTagData == nil {
		return nil
	}
	raw := prop.CustomTagData[unionOverridesKey]
	switch typed := raw.(type) {
	case map[string]string:
		return cloneStringMap(typed)
	case map[string]any:
		return toStringMap(typed)
	default:
		return nil
	}
}

func normalizeDiscriminatorKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func unionTypeMapEntries(typeMap map[string]string) []UnionTypeMapEntry {
	if len(typeMap) == 0 {
		return nil
	}
	keys := make([]string, 0, len(typeMap))
	for key := range typeMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]UnionTypeMapEntry, 0, len(keys))
	for _, key := range keys {
		out = append(out, UnionTypeMapEntry{Key: key, Value: typeMap[key]})
	}
	return out
}

func singularize(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return name
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "ies") && len(trimmed) > 3:
		return trimmed[:len(trimmed)-3] + "y"
	case strings.HasSuffix(lower, "ses") && len(trimmed) > 3:
		return trimmed[:len(trimmed)-2]
	case strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") && len(trimmed) > 1:
		return trimmed[:len(trimmed)-1]
	default:
		return trimmed
	}
}

func lookupRelation(propName string, schema router.SchemaMetadata) (string, *router.RelationshipInfo, bool) {
	if rel := schema.Relationships[propName]; rel != nil {
		return propName, rel, false
	}

	if schema.RelationAliases != nil {
		if relName, ok := schema.RelationAliases[propName]; ok {
			if rel := schema.Relationships[relName]; rel != nil {
				return relName, rel, true
			}
		}
	}
	return "", nil, false
}

func buildRelation(propName, relKey string, rel *router.RelationshipInfo, prop router.PropertyInfo, isAlias bool, opts Options) *Relation {
	if rel == nil {
		return nil
	}

	target := firstNonEmpty(rel.RelatedSchema, rel.RelatedTypeName, prop.RelatedSchema, relKey)
	cardinality := strings.ToLower(rel.Cardinality)
	if cardinality == "" {
		if rel.IsSlice || prop.Items != nil {
			cardinality = "many"
		} else {
			cardinality = "one"
		}
	}
	isMany := strings.Contains(cardinality, "many")
	if isMany {
		cardinality = "many"
	}

	relation := Relation{
		Name:             opts.fieldNamer(propName),
		OriginalName:     propName,
		Key:              relKey,
		Type:             formatTypeName(target, opts),
		Cardinality:      cardinality,
		SourceField:      rel.SourceField,
		SourceColumn:     rel.SourceColumn,
		TargetColumn:     rel.TargetColumn,
		ForeignKey:       rel.ForeignKey,
		Inverse:          rel.Inverse,
		RelationType:     rel.RelationType,
		RelatedSchema:    target,
		IsList:           rel.IsSlice || prop.Items != nil || isMany,
		Nullable:         prop.Nullable,
		PivotTable:       rel.PivotTable,
		SourcePivotField: rel.SourcePivotColumn,
		TargetPivotField: rel.TargetPivotColumn,
		TargetTable:      rel.TargetTable,
	}

	if isAlias && relKey != "" {
		relation.AliasFor = relKey
	}

	return &relation
}

func orderFields(fields []Field, pinned []string) {
	if len(fields) == 0 {
		return
	}

	pinnedIndex := make(map[string]int, len(pinned))
	for idx, name := range pinned {
		pinnedIndex[strings.ToLower(name)] = idx
	}

	sort.SliceStable(fields, func(i, j int) bool {
		left, right := strings.ToLower(fields[i].OriginalName), strings.ToLower(fields[j].OriginalName)
		li, leftPinned := pinnedIndex[left]
		rj, rightPinned := pinnedIndex[right]

		if leftPinned && rightPinned {
			if li != rj {
				return li < rj
			}
		} else if leftPinned != rightPinned {
			return leftPinned
		}

		return fields[i].Name < fields[j].Name
	})
}

func collectRelations(relations map[string]Relation) []Relation {
	if len(relations) == 0 {
		return nil
	}

	result := make([]Relation, 0, len(relations))
	for _, rel := range relations {
		result = append(result, rel)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

func isRequired(required map[string]struct{}, name string) bool {
	_, ok := required[strings.ToLower(name)]
	return ok
}

func formatLabel(label string, namer NameFormatter) string {
	if label == "" || namer == nil {
		return label
	}
	return namer(label)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func mergeUnion(registry map[string]Union, union Union) {
	if registry == nil || union.Name == "" {
		return
	}
	existing, ok := registry[union.Name]
	if !ok {
		if union.TypeMapEntries == nil {
			union.TypeMapEntries = unionTypeMapEntries(union.TypeMap)
		}
		registry[union.Name] = union
		return
	}

	typeSet := make(map[string]struct{}, len(existing.Types)+len(union.Types))
	for _, t := range existing.Types {
		typeSet[t] = struct{}{}
	}
	for _, t := range union.Types {
		typeSet[t] = struct{}{}
	}
	mergedTypes := make([]string, 0, len(typeSet))
	for t := range typeSet {
		mergedTypes = append(mergedTypes, t)
	}
	sort.Strings(mergedTypes)

	mergedMap := make(map[string]string)
	for key, val := range existing.TypeMap {
		mergedMap[key] = val
	}
	for key, val := range union.TypeMap {
		if strings.TrimSpace(val) == "" {
			continue
		}
		mergedMap[key] = val
	}

	existing.Types = mergedTypes
	existing.TypeMap = mergedMap
	existing.TypeMapEntries = unionTypeMapEntries(mergedMap)
	if existing.SourceEntity == "" {
		existing.SourceEntity = union.SourceEntity
	}
	if existing.SourceField == "" {
		existing.SourceField = union.SourceField
	}
	registry[union.Name] = existing
}

func collectUnions(registry map[string]Union) []Union {
	if len(registry) == 0 {
		return nil
	}
	out := make([]Union, 0, len(registry))
	for _, union := range registry {
		if union.TypeMapEntries == nil {
			union.TypeMapEntries = unionTypeMapEntries(union.TypeMap)
		}
		out = append(out, union)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, val := range in {
		out[key] = val
	}
	return out
}

func toStringMap(raw map[string]any) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, val := range raw {
		if key == "" || val == nil {
			continue
		}
		switch v := val.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				out[key] = v
			}
		default:
			out[key] = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParseVersionedName splits "slug@v1.2.3" into ("slug", "1.2.3", true).
func ParseVersionedName(raw string) (string, string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false
	}
	idx := strings.LastIndex(trimmed, "@")
	if idx <= 0 || idx >= len(trimmed)-1 {
		return trimmed, "", false
	}
	base := strings.TrimSpace(trimmed[:idx])
	version := strings.TrimSpace(trimmed[idx+1:])
	version = strings.TrimPrefix(strings.ToLower(version), "v")
	if base == "" || version == "" || !isSemanticVersion(version) {
		return trimmed, "", false
	}
	return base, version, true
}

func formatTypeName(raw string, opts Options) string {
	base, version, ok := ParseVersionedName(raw)
	if !ok {
		return opts.typeNamer(raw)
	}
	suffix := formatVersionSuffix(version)
	if suffix == "" {
		return opts.typeNamer(base)
	}
	return opts.typeNamer(base) + suffix
}

func formatVersionSuffix(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(strings.ToLower(version), "v")
	if version == "" {
		return ""
	}
	return "V" + strings.ReplaceAll(version, ".", "_")
}

func isSemanticVersion(version string) bool {
	parts := strings.Split(version, ".")
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

func defaultOptions() Options {
	return Options{
		fieldNamer:  strcase.ToCamel,
		typeNamer:   strcase.ToPascal,
		typeMapping: defaultTypeMapping(),
		pinned:      []string{"id", "created_at", "updated_at"},
	}
}
