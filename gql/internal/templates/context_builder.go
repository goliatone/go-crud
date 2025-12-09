package templates

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ettle/strcase"
	"golang.org/x/mod/modfile"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/hooks"
	"github.com/goliatone/go-crud/gql/internal/overlay"
)

// ContextOptions controls context construction.
type ContextOptions struct {
	ConfigPath         string
	OutDir             string
	PolicyHook         string
	EmitDataloader     bool
	EmitSubscriptions  bool
	SubscriptionEvents []string
	Overlay            overlay.Overlay
	HookOptions        hooks.Options
}

// BuildContext produces a template Context with defaults plus overlay additions.
func BuildContext(doc formatter.Document, opts ContextOptions) Context {
	ctx := NewContext(doc)

	configDir := filepath.Dir(opts.ConfigPath)
	ctx.SchemaPath = toSlash(relOrDefault(configDir, filepath.Join(opts.OutDir, "schema.graphql")))
	modPath := findModulePath(configDir)
	modelRel := toSlash(relOrDefault(configDir, filepath.Join(opts.OutDir, "model")))
	if modPath != "" {
		ctx.ModelPackage = path.Join(modPath, modelRel)
	} else {
		ctx.ModelPackage = modelRel
	}
	ctx.ResolverPackage = toSlash(relOrDefault(configDir, filepath.Join(opts.OutDir, "resolvers")))
	dataloaderRel := toSlash(relOrDefault(configDir, filepath.Join(opts.OutDir, "dataloader")))
	if modPath != "" {
		ctx.DataloaderPackage = path.Join(modPath, dataloaderRel)
	} else {
		ctx.DataloaderPackage = dataloaderRel
	}
	ctx.PolicyHook = opts.PolicyHook
	ctx.EmitDataloader = opts.EmitDataloader
	ctx.EmitSubscriptions = opts.EmitSubscriptions

	events := normalizeSubscriptionEvents(opts.SubscriptionEvents, opts.EmitSubscriptions)
	defaultOverlay := buildDefaultOverlay(doc, events)
	merged := overlay.Merge(defaultOverlay, opts.Overlay)

	ctx.Scalars = toTemplateScalars(sanitizeScalars(merged.Scalars))
	ctx.Enums = toTemplateEnums(sanitizeEnums(merged.Enums))
	ctx.Inputs = toTemplateInputs(sanitizeInputs(merged.Inputs))
	ctx.Queries = toTemplateOperations(sanitizeOperations(merged.Queries))
	ctx.Mutations = toTemplateOperations(sanitizeOperations(merged.Mutations))
	ctx.Subscriptions = toTemplateSubscriptions(sanitizeSubscriptions(merged.Subscriptions))

	ctx.Entities = append([]formatter.Entity{}, ctx.Entities...)
	ctx.Entities = append(ctx.Entities, buildPageInfo())
	ctx.Entities = append(ctx.Entities, buildConnections(doc.Entities)...)
	sortEntities(ctx.Entities)

	ctx.ModelStructs, ctx.ModelEnums, ctx.ModelImports = buildModels(ctx)
	ctx.Criteria = buildCriteriaConfig(doc)
	ctx.Hooks = hooks.Build(doc, opts.HookOptions)
	ctx.ResolverEntities = make([]ResolverEntity, 0, len(doc.Entities))
	for _, ent := range doc.Entities {
		ctx.ResolverEntities = append(ctx.ResolverEntities, ResolverEntity{
			Entity: ent,
			Hooks:  ctx.Hooks.Entities[ent.Name],
		})
	}

	ctx.DataloaderEntities = buildDataloaderEntities(doc, ctx.ModelStructs)

	return ctx
}

func buildDefaultOverlay(doc formatter.Document, subscriptionEvents []string) overlay.Overlay {
	scalars := []overlay.Scalar{
		{Name: "UUID", Description: "Custom scalar for UUID values", GoType: "string"},
		{Name: "Time", Description: "Custom scalar for Time values", GoType: "time.Time"},
		{Name: "JSON", Description: "Custom scalar for JSON objects", GoType: "map[string]any"},
	}

	enums := []overlay.Enum{
		{
			Name:        "OrderDirection",
			Description: "Sort direction",
			Values: []overlay.EnumValue{
				{Name: "ASC"},
				{Name: "DESC"},
			},
		},
		{
			Name:        "FilterOperator",
			Description: "Filter comparison operators",
			Values: []overlay.EnumValue{
				{Name: "EQ", Description: "="},
				{Name: "NE", Description: "<>"},
				{Name: "GT", Description: ">"},
				{Name: "LT", Description: "<"},
				{Name: "GTE", Description: ">="},
				{Name: "LTE", Description: "<="},
				{Name: "ILIKE", Description: "ILIKE"},
				{Name: "LIKE", Description: "LIKE"},
				{Name: "IN", Description: "IN"},
				{Name: "NOT_IN", Description: "NOT IN"},
			},
		},
	}

	inputs := []overlay.Input{
		{
			Name:        "PaginationInput",
			Description: "Input type for pagination parameters",
			Fields: []overlay.InputField{
				{Name: "limit", Type: "Int", Required: false},
				{Name: "offset", Type: "Int", Required: false},
			},
		},
		{
			Name:        "OrderByInput",
			Description: "Input type for ordering parameters",
			Fields: []overlay.InputField{
				{Name: "field", Type: "String", Required: true},
				{Name: "direction", Type: "OrderDirection", Required: false},
			},
		},
		{
			Name:        "FilterInput",
			Description: "Input type for filtering",
			Fields: []overlay.InputField{
				{Name: "field", Type: "String", Required: true},
				{Name: "operator", Type: "FilterOperator", Required: true},
				{Name: "value", Type: "String", Required: true},
			},
		},
	}

	var queries, mutations []overlay.Operation

	for _, entity := range doc.Entities {
		createInput, updateInput := buildEntityInputs(entity)
		inputs = append(inputs, createInput, updateInput)

		queries = append(queries,
			overlay.Operation{
				Name:       "get" + entity.Name,
				ReturnType: entity.Name,
				Required:   true,
				Args: []overlay.Argument{
					{Name: "id", Type: "UUID", Required: true},
				},
			},
			overlay.Operation{
				Name:       "list" + entity.Name,
				ReturnType: entity.Name + "Connection",
				Required:   true,
				Args: []overlay.Argument{
					{Name: "pagination", Type: "PaginationInput", Required: false},
					{Name: "orderBy", Type: "OrderByInput", List: true, Required: false},
					{Name: "filter", Type: "FilterInput", List: true, Required: false},
				},
			},
		)

		mutations = append(mutations,
			overlay.Operation{
				Name:       "create" + entity.Name,
				ReturnType: entity.Name,
				Required:   true,
				Args: []overlay.Argument{
					{Name: "input", Type: createInput.Name, Required: true},
				},
			},
			overlay.Operation{
				Name:       "update" + entity.Name,
				ReturnType: entity.Name,
				Required:   true,
				Args: []overlay.Argument{
					{Name: "id", Type: "UUID", Required: true},
					{Name: "input", Type: updateInput.Name, Required: true},
				},
			},
			overlay.Operation{
				Name:       "delete" + entity.Name,
				ReturnType: "Boolean",
				Required:   true,
				Args: []overlay.Argument{
					{Name: "id", Type: "UUID", Required: true},
				},
			},
		)
	}

	return overlay.Overlay{
		Scalars:       scalars,
		Enums:         enums,
		Inputs:        inputs,
		Queries:       queries,
		Mutations:     mutations,
		Subscriptions: buildDefaultSubscriptions(doc.Entities, subscriptionEvents),
	}
}

func buildPageInfo() formatter.Entity {
	return formatter.Entity{
		Name: "PageInfo",
		Fields: []formatter.Field{
			{Name: "total", OriginalName: "total", Type: "Int", Required: true},
			{Name: "hasNextPage", OriginalName: "hasNextPage", Type: "Boolean", Required: true},
			{Name: "hasPreviousPage", OriginalName: "hasPreviousPage", Type: "Boolean", Required: true},
			{Name: "startCursor", OriginalName: "startCursor", Type: "String"},
			{Name: "endCursor", OriginalName: "endCursor", Type: "String"},
		},
	}
}

func buildConnections(entities []formatter.Entity) []formatter.Entity {
	result := make([]formatter.Entity, 0, len(entities)*2)
	for _, entity := range entities {
		edgeName := entity.Name + "Edge"
		connName := entity.Name + "Connection"

		edge := formatter.Entity{
			Name:        edgeName,
			RawName:     edgeName,
			Description: "Edge wrapper for " + entity.Name,
			Fields: []formatter.Field{
				{Name: "cursor", OriginalName: "cursor", Type: "String", Required: true},
				{Name: "node", OriginalName: "node", Type: entity.Name, Required: true},
			},
		}

		conn := formatter.Entity{
			Name:        connName,
			RawName:     connName,
			Description: entity.Name + " connection",
			Fields: []formatter.Field{
				{Name: "edges", OriginalName: "edges", Type: edgeName, IsList: true, Required: true},
				{Name: "pageInfo", OriginalName: "pageInfo", Type: "PageInfo", Required: true},
			},
		}

		result = append(result, edge, conn)
	}
	return result
}

func buildEntityInputs(entity formatter.Entity) (overlay.Input, overlay.Input) {
	var createFields, updateFields []overlay.InputField

	for _, f := range entity.Fields {
		// Skip identifiers, relation fields, and mutation-omitted fields; inputs should only expose scalars/FKs.
		if strings.EqualFold(f.OriginalName, "id") || f.ReadOnly || f.OmitFromMutations || f.Relation != nil {
			continue
		}
		createFields = append(createFields, overlay.InputField{
			Name:     f.Name,
			Type:     f.Type,
			Required: f.Required && !f.Nullable,
			List:     f.IsList,
		})
		updateFields = append(updateFields, overlay.InputField{
			Name: f.Name,
			Type: f.Type,
			List: f.IsList,
		})
	}

	sort.SliceStable(createFields, func(i, j int) bool {
		if createFields[i].Required != createFields[j].Required {
			return createFields[i].Required && !createFields[j].Required
		}
		return strings.ToLower(createFields[i].Name) < strings.ToLower(createFields[j].Name)
	})

	if len(updateFields) > 1 {
		order := make(map[string]int, len(createFields))
		for idx, field := range createFields {
			order[strings.ToLower(field.Name)] = idx
		}

		sort.SliceStable(updateFields, func(i, j int) bool {
			left, okLeft := order[strings.ToLower(updateFields[i].Name)]
			right, okRight := order[strings.ToLower(updateFields[j].Name)]

			switch {
			case okLeft && okRight:
				return left < right
			case okLeft:
				return true
			case okRight:
				return false
			default:
				return strings.ToLower(updateFields[i].Name) < strings.ToLower(updateFields[j].Name)
			}
		})
	}

	return overlay.Input{
			Name:        "Create" + entity.Name + "Input",
			Description: "Create payload for " + entity.Name,
			Fields:      createFields,
		}, overlay.Input{
			Name:        "Update" + entity.Name + "Input",
			Description: "Update payload for " + entity.Name,
			Fields:      updateFields,
		}
}

func buildDefaultSubscriptions(entities []formatter.Entity, events []string) []overlay.Subscription {
	if len(events) == 0 {
		return nil
	}

	var subs []overlay.Subscription
	for _, entity := range entities {
		for _, event := range events {
			event = strings.TrimSpace(strings.ToLower(event))
			if event == "" {
				continue
			}
			subs = append(subs, overlay.Subscription{
				Name:       lowerFirst(entity.Name) + strcase.ToPascal(event),
				ReturnType: entity.Name,
				Required:   true,
				Entity:     entity.Name,
				Event:      event,
				Topic:      fmt.Sprintf("%s.%s", lowerFirst(entity.Name), event),
			})
		}
	}
	return subs
}

func buildModels(ctx Context) ([]ModelStruct, []ModelEnum, []string) {
	imports := map[string]struct{}{}
	enums := make([]ModelEnum, 0, len(ctx.Enums))
	enumNames := make(map[string]struct{}, len(ctx.Enums))
	for _, e := range ctx.Enums {
		enumNames[e.Name] = struct{}{}
		vals := make([]string, 0, len(e.Values))
		for _, v := range e.Values {
			vals = append(vals, v.Name)
		}
		enums = append(enums, ModelEnum{Name: e.Name, Values: vals})
	}

	scalarMap := scalarGoTypes(ctx.Scalars)
	for _, s := range ctx.Scalars {
		if strings.HasPrefix(s.GoType, "time.") {
			imports["time"] = struct{}{}
		}
	}
	if len(ctx.Scalars) > 0 {
		imports["github.com/99designs/gqlgen/graphql"] = struct{}{}
	}

	structs := make([]ModelStruct, 0, len(ctx.Entities)+len(ctx.Inputs))

	for _, ent := range ctx.Entities {
		fields := make([]ModelField, 0, len(ent.Fields))
		for _, f := range ent.Fields {
			goType, imp := goTypeFor(f.Type, f.IsList, f.Required && !f.Nullable, isEntity(ctx.Entities, f.Type), enumNames, scalarMap)
			if imp != "" {
				imports[imp] = struct{}{}
			}
			fields = append(fields, ModelField{
				Name:     strcase.ToPascal(f.Name),
				JSONName: f.OriginalName,
				GoType:   goType,
			})
		}
		structs = append(structs, ModelStruct{
			Name:        ent.Name,
			Description: ent.Description,
			Fields:      fields,
		})
	}

	for _, in := range ctx.Inputs {
		fields := make([]ModelField, 0, len(in.Fields))
		for _, f := range in.Fields {
			goType, imp := goTypeFor(f.Type, f.List, f.Required, isEntity(ctx.Entities, f.Type), enumNames, scalarMap)
			if !f.List && !f.Required && !strings.HasPrefix(goType, "*") {
				goType = "*" + goType
			}
			if imp != "" {
				imports[imp] = struct{}{}
			}
			fields = append(fields, ModelField{
				Name:     strcase.ToPascal(f.Name),
				JSONName: lowerFirst(f.Name),
				GoType:   goType,
			})
		}
		structs = append(structs, ModelStruct{
			Name:        in.Name,
			Description: in.Description,
			Fields:      fields,
		})
	}

	importList := make([]string, 0, len(imports))
	for imp := range imports {
		importList = append(importList, imp)
	}
	sort.Strings(importList)

	return structs, enums, importList
}

func buildDataloaderEntities(doc formatter.Document, models []ModelStruct) []DataloaderEntity {
	modelFields := make(map[string]map[string]ModelField, len(models))
	for _, m := range models {
		fields := make(map[string]ModelField, len(m.Fields))
		for _, f := range m.Fields {
			fields[strings.ToLower(f.JSONName)] = f
		}
		modelFields[m.Name] = fields
	}

	pkByEntity := make(map[string]DataloaderField, len(doc.Entities))
	for _, ent := range doc.Entities {
		pk := findPrimaryKey(ent, modelFields[ent.Name])
		if pk.Column != "" {
			pkByEntity[ent.Name] = pk
		}
	}

	entities := make([]DataloaderEntity, 0, len(doc.Entities))
	for _, ent := range doc.Entities {
		pk := pkByEntity[ent.Name]
		if pk.Column == "" {
			continue
		}

		entity := DataloaderEntity{
			Name:      ent.Name,
			ModelName: ent.Name,
			PK:        pk,
			Relations: buildDataloaderRelations(ent, pk, modelFields, pkByEntity),
			Resolver:  lowerFirst(ent.Name),
		}
		entities = append(entities, entity)
	}

	return entities
}

func findPrimaryKey(entity formatter.Entity, fields map[string]ModelField) DataloaderField {
	var pk formatter.Field
	for _, f := range entity.Fields {
		if f.Relation != nil {
			continue
		}
		if pk.Name == "" || strings.EqualFold(f.OriginalName, "id") {
			pk = f
		}
		if strings.EqualFold(f.OriginalName, "id") {
			break
		}
	}

	if pk.Name == "" {
		return DataloaderField{}
	}

	return toDataloaderField(pk, fields)
}

func toDataloaderField(f formatter.Field, fields map[string]ModelField) DataloaderField {
	field := ModelField{Name: strcase.ToPascal(f.Name), JSONName: f.OriginalName, GoType: f.Type}
	if mf, ok := fields[strings.ToLower(f.OriginalName)]; ok {
		field = mf
	}
	return DataloaderField{
		Name:      f.Name,
		FieldName: field.Name,
		Column:    f.OriginalName,
		GoType:    field.GoType,
	}
}

func buildDataloaderRelations(entity formatter.Entity, pk DataloaderField, modelFields map[string]map[string]ModelField, pkByEntity map[string]DataloaderField) []DataloaderRelation {
	sourceFields := modelFields[entity.Name]
	relations := make([]DataloaderRelation, 0, len(entity.Fields))
	for _, f := range entity.Fields {
		if f.Relation == nil {
			continue
		}
		targetFields := modelFields[f.Relation.Type]
		targetPK := pkByEntity[f.Relation.Type]
		relation := DataloaderRelation{
			Name:         f.Name,
			FieldName:    structFieldName(f.OriginalName, sourceFields),
			Target:       f.Relation.Type,
			TargetField:  structFieldName(f.Relation.Type, targetFields),
			RelationType: normalizeRelationType(f.Relation),
			IsList:       f.Relation.IsList,
			SourceColumn: firstNonEmpty(f.Relation.SourceColumn, f.Relation.SourceField, pk.Column),
			TargetColumn: firstNonEmpty(f.Relation.TargetColumn, "id"),
			SourceField:  f.Relation.SourceField,
			PivotTable:   f.Relation.PivotTable,
			SourcePivot:  f.Relation.SourcePivotField,
			TargetPivot:  f.Relation.TargetPivotField,
			TargetTable:  firstNonEmpty(f.Relation.TargetTable, f.Relation.OriginalName, f.Relation.RelatedSchema),
		}
		if relation.RelationType == "manyToMany" {
			if relation.SourcePivot == "" {
				relation.SourcePivot = fmt.Sprintf("%s_id", strcase.ToSnake(entity.RawName))
			}
			if relation.TargetPivot == "" {
				relation.TargetPivot = fmt.Sprintf("%s_id", strcase.ToSnake(firstNonEmpty(f.Relation.RelatedSchema, f.Relation.OriginalName)))
			}
			if relation.TargetTable == "" {
				relation.TargetTable = strcase.ToSnake(firstNonEmpty(f.Relation.RelatedSchema, f.Relation.OriginalName))
			}
			if relation.PivotTable == "" {
				targetName := relation.TargetTable
				if targetName == "" {
					targetName = strcase.ToSnake(firstNonEmpty(f.Relation.RelatedSchema, f.Relation.OriginalName))
				}
				relation.PivotTable = fmt.Sprintf("%s_%s", strcase.ToSnake(entity.RawName), targetName)
			}
		}
		if relation.SourceColumn == "" {
			relation.SourceColumn = pk.Column
		}
		relation.SourceFieldKey = findModelField(firstNonEmpty(f.Relation.SourceField, f.Relation.SourceColumn), sourceFields, pk)
		relation.TargetFieldKey = findModelField(relation.TargetColumn, targetFields, targetPK)
		relations = append(relations, relation)
	}
	return relations
}

func structFieldName(name string, fields map[string]ModelField) string {
	if name == "" {
		return ""
	}
	if field, ok := fields[strings.ToLower(name)]; ok {
		return field.Name
	}
	return strcase.ToPascal(name)
}

func findModelField(name string, fields map[string]ModelField, fallback DataloaderField) DataloaderField {
	if name == "" {
		return fallback
	}
	key := strings.ToLower(name)
	if field, ok := fields[key]; ok {
		return DataloaderField{
			Name:      lowerFirst(field.Name),
			FieldName: field.Name,
			Column:    field.JSONName,
			GoType:    field.GoType,
		}
	}
	return fallback
}

func normalizeRelationType(rel *formatter.Relation) string {
	if rel == nil {
		return ""
	}

	normalized := strings.ToLower(strings.ReplaceAll(rel.RelationType, "-", ""))
	normalized = strings.ReplaceAll(normalized, "_", "")

	switch normalized {
	case "belongsto":
		return "belongsTo"
	case "hasmany":
		return "hasMany"
	case "hasone":
		return "hasOne"
	case "manytomany", "m2m":
		return "manyToMany"
	}

	if rel.IsList {
		return "hasMany"
	}
	return "hasOne"
}

func scalarGoTypes(scalars []TemplateScalar) map[string]string {
	m := map[string]string{
		"ID":      "string",
		"String":  "string",
		"Boolean": "bool",
		"Int":     "int",
		"Float":   "float64",
	}
	for _, s := range scalars {
		if s.GoType != "" {
			m[s.Name] = s.GoType
		}
	}
	return m
}

func goTypeFor(gqlType string, isList bool, required bool, isEntity bool, enumNames map[string]struct{}, scalarMap map[string]string) (string, string) {
	baseType := gqlType
	var importName string

	if goType, ok := scalarMap[baseType]; ok {
		baseType = goType
	} else if _, ok := enumNames[baseType]; ok {
		// keep enum name
	} else if isEntity {
		baseType = "*" + baseType
	}

	if strings.HasPrefix(baseType, "time.") {
		importName = "time"
	}

	if isList {
		if strings.HasPrefix(baseType, "*") {
			return "[]" + baseType, importName
		}
		return "[]" + baseType, importName
	}

	if !required && !strings.HasPrefix(baseType, "*") && !isEntity && !isScalarBuiltin(baseType) {
		baseType = "*" + baseType
	}

	return baseType, importName
}

func isScalarBuiltin(goType string) bool {
	switch goType {
	case "string", "bool", "int", "float64", "any", "map[string]any":
		return true
	default:
		return strings.HasPrefix(goType, "[]")
	}
}

func isEntity(entities []formatter.Entity, name string) bool {
	for _, e := range entities {
		if e.Name == name {
			return true
		}
	}
	return false
}

func relOrDefault(base, target string) string {
	if base == "" {
		base = "."
	}
	if rel, err := filepath.Rel(base, target); err == nil {
		return rel
	}
	return target
}

func toSlash(pathValue string) string {
	return filepath.ToSlash(filepath.Clean(pathValue))
}

func sortEntities(entities []formatter.Entity) {
	for i := range entities {
		for j := i + 1; j < len(entities); j++ {
			if entities[j].Name < entities[i].Name {
				entities[i], entities[j] = entities[j], entities[i]
			}
		}
	}
}

func lowerFirst(val string) string {
	if val == "" {
		return val
	}
	return strings.ToLower(val[:1]) + val[1:]
}

func findModulePath(startDir string) string {
	dir := filepath.Clean(startDir)
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			if mf, perr := modfile.Parse("go.mod", data, nil); perr == nil && mf.Module != nil {
				return mf.Module.Mod.Path
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func sanitizeScalars(list []overlay.Scalar) []overlay.Scalar {
	var out []overlay.Scalar
	for _, s := range list {
		s.Name = strings.TrimSpace(s.Name)
		if s.Name == "" {
			continue
		}
		if strings.TrimSpace(s.GoType) == "" {
			s.GoType = "string"
		}
		out = append(out, s)
	}
	return out
}

func toTemplateScalars(list []overlay.Scalar) []TemplateScalar {
	out := make([]TemplateScalar, 0, len(list))
	for _, s := range list {
		out = append(out, TemplateScalar{
			Name:        s.Name,
			Description: s.Description,
			GoType:      s.GoType,
		})
	}
	return out
}

func sanitizeEnums(list []overlay.Enum) []overlay.Enum {
	var out []overlay.Enum
	for _, e := range list {
		e.Name = strings.TrimSpace(e.Name)
		if e.Name == "" {
			continue
		}
		var vals []overlay.EnumValue
		for _, v := range e.Values {
			if strings.TrimSpace(v.Name) == "" {
				continue
			}
			vals = append(vals, overlay.EnumValue{Name: strings.TrimSpace(v.Name), Description: v.Description})
		}
		e.Values = vals
		out = append(out, e)
	}
	return out
}

func sanitizeInputs(list []overlay.Input) []overlay.Input {
	var out []overlay.Input
	for _, in := range list {
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" {
			continue
		}
		var fields []overlay.InputField
		for _, f := range in.Fields {
			f.Name = strings.TrimSpace(f.Name)
			f.Type = strings.TrimSpace(f.Type)
			if f.Name == "" || f.Type == "" {
				continue
			}
			fields = append(fields, f)
		}
		in.Fields = fields
		out = append(out, in)
	}
	return out
}

func buildCriteriaConfig(doc formatter.Document) map[string][]CriteriaField {
	entityByName := make(map[string]formatter.Entity, len(doc.Entities))
	entityByRaw := make(map[string]formatter.Entity, len(doc.Entities))
	for _, e := range doc.Entities {
		entityByName[e.Name] = e
		entityByRaw[strings.ToLower(e.RawName)] = e
	}

	config := make(map[string][]CriteriaField, len(doc.Entities))
	for _, e := range doc.Entities {
		fields := collectCriteriaFields(e, entityByName, entityByRaw)
		if len(fields) == 0 {
			continue
		}
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].Field < fields[j].Field
		})
		config[e.Name] = fields
	}
	return config
}

func collectCriteriaFields(entity formatter.Entity, byName, byRaw map[string]formatter.Entity) []CriteriaField {
	result := make([]CriteriaField, 0, len(entity.Fields))
	for _, f := range entity.Fields {
		if f.Relation != nil {
			target, ok := byName[f.Relation.Type]
			if !ok {
				target, ok = byRaw[strings.ToLower(f.Relation.RelatedSchema)]
			}
			if !ok {
				continue
			}
			for _, tf := range target.Fields {
				if tf.Relation != nil {
					continue
				}
				rt := strings.ToLower(strings.ReplaceAll(f.Relation.RelationType, "-", ""))
				rt = strings.ReplaceAll(rt, "_", "")
				pivotTable := f.Relation.PivotTable
				if pivotTable == "" && (rt == "manytomany" || rt == "m2m") {
					pivotTable = fmt.Sprintf("%s_%s", strcase.ToSnake(firstNonEmpty(f.Relation.RelatedSchema, f.Relation.OriginalName)), strcase.ToSnake(entity.RawName))
				}
				sourcePivot := f.Relation.SourcePivotField
				if sourcePivot == "" && (rt == "manytomany" || rt == "m2m") {
					sourcePivot = fmt.Sprintf("%s_id", strcase.ToSnake(entity.RawName))
				}
				targetPivot := f.Relation.TargetPivotField
				if targetPivot == "" && (rt == "manytomany" || rt == "m2m") {
					targetPivot = fmt.Sprintf("%s_id", strcase.ToSnake(firstNonEmpty(f.Relation.RelatedSchema, f.Relation.OriginalName)))
				}
				targetTable := firstNonEmpty(f.Relation.TargetTable, f.Relation.OriginalName, f.Relation.RelatedSchema)

				result = append(result, CriteriaField{
					Field:        fmt.Sprintf("%s.%s", f.Name, tf.Name),
					Column:       fmt.Sprintf("%s.%s", f.OriginalName, tf.OriginalName),
					Relation:     strcase.ToPascal(f.Name),
					RelationType: f.Relation.RelationType,
					PivotTable:   pivotTable,
					SourceColumn: firstNonEmpty(f.Relation.SourceColumn, "id"),
					TargetColumn: firstNonEmpty(f.Relation.TargetColumn, "id"),
					SourcePivot:  sourcePivot,
					TargetPivot:  targetPivot,
					TargetTable:  targetTable,
				})
			}
			continue
		}
		result = append(result, CriteriaField{
			Field:  f.Name,
			Column: f.OriginalName,
		})
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func sanitizeOperations(list []overlay.Operation) []overlay.Operation {
	var out []overlay.Operation
	for _, op := range list {
		op.Name = strings.TrimSpace(op.Name)
		op.ReturnType = strings.TrimSpace(op.ReturnType)
		if op.Name == "" || op.ReturnType == "" {
			continue
		}
		var args []overlay.Argument
		for _, a := range op.Args {
			a.Name = strings.TrimSpace(a.Name)
			a.Type = strings.TrimSpace(a.Type)
			if a.Name == "" || a.Type == "" {
				continue
			}
			args = append(args, a)
		}
		op.Args = args
		out = append(out, op)
	}
	return out
}

func toTemplateEnums(list []overlay.Enum) []TemplateEnum {
	out := make([]TemplateEnum, 0, len(list))
	for _, e := range list {
		vals := make([]TemplateEnumValue, 0, len(e.Values))
		for _, v := range e.Values {
			vals = append(vals, TemplateEnumValue{Name: v.Name, Description: v.Description})
		}
		out = append(out, TemplateEnum{
			Name:        e.Name,
			Description: e.Description,
			Values:      vals,
		})
	}
	return out
}

func toTemplateInputs(list []overlay.Input) []TemplateInput {
	out := make([]TemplateInput, 0, len(list))
	for _, in := range list {
		fields := make([]TemplateInputField, 0, len(in.Fields))
		for _, f := range in.Fields {
			fields = append(fields, TemplateInputField{
				Name:        f.Name,
				Type:        f.Type,
				Description: f.Description,
				List:        f.List,
				Required:    f.Required,
			})
		}
		out = append(out, TemplateInput{
			Name:        in.Name,
			Description: in.Description,
			Fields:      fields,
		})
	}
	return out
}

func toTemplateOperations(list []overlay.Operation) []TemplateOperation {
	out := make([]TemplateOperation, 0, len(list))
	for _, op := range list {
		args := make([]TemplateArgument, 0, len(op.Args))
		for _, a := range op.Args {
			args = append(args, TemplateArgument{
				Name:        a.Name,
				Type:        a.Type,
				Description: a.Description,
				List:        a.List,
				Required:    a.Required,
			})
		}
		out = append(out, TemplateOperation{
			Name:          op.Name,
			ReturnType:    op.ReturnType,
			List:          op.List,
			Required:      op.Required,
			Description:   op.Description,
			Args:          args,
			ArgsSignature: buildArgsSignature(args),
		})
	}
	return out
}

func buildArgsSignature(args []TemplateArgument) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		sig := a.Name + ": "
		if a.List {
			sig += "[" + a.Type
			if a.Required {
				sig += "!"
			}
			sig += "]"
			if a.Required {
				sig += "!"
			}
		} else {
			sig += a.Type
			if a.Required {
				sig += "!"
			}
		}
		parts = append(parts, sig)
	}
	return strings.Join(parts, ", ")
}

func normalizeSubscriptionEvents(events []string, enabled bool) []string {
	if !enabled {
		return nil
	}
	if len(events) == 0 {
		events = []string{"created", "updated", "deleted"}
	}
	seen := make(map[string]struct{}, len(events))
	out := make([]string, 0, len(events))
	for _, e := range events {
		normalized := strings.TrimSpace(strings.ToLower(e))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func sanitizeSubscriptions(list []overlay.Subscription) []overlay.Subscription {
	var out []overlay.Subscription
	for _, sub := range list {
		sub.Name = strings.TrimSpace(sub.Name)
		sub.ReturnType = strings.TrimSpace(sub.ReturnType)
		if sub.Name == "" || sub.ReturnType == "" {
			continue
		}
		var args []overlay.Argument
		for _, a := range sub.Args {
			a.Name = strings.TrimSpace(a.Name)
			a.Type = strings.TrimSpace(a.Type)
			if a.Name == "" || a.Type == "" {
				continue
			}
			args = append(args, a)
		}
		sub.Args = args
		if sub.Entity == "" {
			sub.Entity = strings.TrimSpace(sub.ReturnType)
		}

		event := strings.TrimSpace(strings.ToLower(sub.Event))
		if event == "" && len(sub.Events) > 0 {
			if normalized := normalizeSubscriptionEvents(sub.Events, true); len(normalized) > 0 {
				event = normalized[0]
			}
		}
		sub.Event = event

		if sub.Topic == "" {
			switch {
			case sub.Entity != "" && sub.Event != "":
				sub.Topic = fmt.Sprintf("%s.%s", lowerFirst(sub.Entity), sub.Event)
			case sub.Event != "":
				sub.Topic = sub.Event
			case sub.Name != "":
				sub.Topic = lowerFirst(sub.Name)
			}
		}

		out = append(out, sub)
	}
	return out
}

func toTemplateSubscriptions(list []overlay.Subscription) []TemplateSubscription {
	out := make([]TemplateSubscription, 0, len(list))
	for _, sub := range list {
		args := make([]TemplateArgument, 0, len(sub.Args))
		for _, a := range sub.Args {
			args = append(args, TemplateArgument{
				Name:        a.Name,
				Type:        a.Type,
				Description: a.Description,
				List:        a.List,
				Required:    a.Required,
			})
		}
		out = append(out, TemplateSubscription{
			Name:          sub.Name,
			ReturnType:    sub.ReturnType,
			List:          sub.List,
			Required:      sub.Required,
			Description:   sub.Description,
			Args:          args,
			ArgsSignature: buildArgsSignature(args),
			Entity:        sub.Entity,
			Event:         sub.Event,
			Topic:         sub.Topic,
			MethodName:    strcase.ToPascal(sub.Name),
		})
	}
	return out
}
