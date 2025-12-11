package formatter

import (
	"sort"
	"strings"

	"github.com/ettle/strcase"
	"github.com/goliatone/go-router"
)

// Document is the template ready representation of a list of schemas.
type Document struct {
	Entities []Entity
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
	for _, schema := range schemas {
		doc.Entities = append(doc.Entities, formatSchema(schema, opts))
	}

	sort.Slice(doc.Entities, func(i, j int) bool {
		return doc.Entities[i].Name < doc.Entities[j].Name
	})

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

func formatSchema(schema router.SchemaMetadata, opts Options) Entity {
	entity := Entity{
		RawName:     schema.Name,
		Name:        opts.typeNamer(schema.Name),
		Description: schema.Description,
		LabelField:  formatLabel(schema.LabelField, opts.fieldNamer),
	}

	if len(schema.Properties) == 0 {
		return entity
	}

	required := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		required[strings.ToLower(name)] = struct{}{}
	}

	fields := make([]Field, 0, len(schema.Properties))
	relations := make(map[string]Relation)

	for propName, prop := range schema.Properties {
		field := buildField(propName, prop, schema, required, opts)
		if field.Relation != nil {
			relations[propName] = *field.Relation
		}
		fields = append(fields, field)
	}

	orderFields(fields, opts.pinned)
	entity.Fields = fields
	entity.Relationships = collectRelations(relations)
	return entity
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
		Type:             opts.typeNamer(target),
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

func defaultOptions() Options {
	return Options{
		fieldNamer:  strcase.ToCamel,
		typeNamer:   strcase.ToPascal,
		typeMapping: defaultTypeMapping(),
		pinned:      []string{"id", "created_at", "updated_at"},
	}
}
