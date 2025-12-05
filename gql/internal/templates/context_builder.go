package templates

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ettle/strcase"
	"golang.org/x/mod/modfile"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/overlay"
)

// ContextOptions controls context construction.
type ContextOptions struct {
	ConfigPath     string
	OutDir         string
	PolicyHook     string
	EmitDataloader bool
	Overlay        overlay.Overlay
}

// BuildContext produces a template Context with defaults plus overlay additions.
func BuildContext(doc formatter.Document, opts ContextOptions) Context {
	ctx := NewContext(doc)
	ctx.ResolverEntities = doc.Entities

	configDir := filepath.Dir(opts.ConfigPath)
	ctx.SchemaPath = toSlash(relOrDefault(configDir, filepath.Join(opts.OutDir, "schema.graphql")))
	modPath := findModulePath(configDir)
	modelRel := toSlash(filepath.Clean(filepath.Join(opts.OutDir, "model")))
	if modPath != "" {
		ctx.ModelPackage = path.Join(modPath, modelRel)
	} else {
		ctx.ModelPackage = toSlash(relOrDefault(configDir, filepath.Join(opts.OutDir, "model")))
	}
	ctx.ResolverPackage = toSlash(relOrDefault(configDir, filepath.Join(opts.OutDir, "resolvers")))
	ctx.DataloaderPackage = toSlash(relOrDefault(configDir, filepath.Join(opts.OutDir, "dataloader")))
	ctx.PolicyHook = opts.PolicyHook
	ctx.EmitDataloader = opts.EmitDataloader

	defaultOverlay := buildDefaultOverlay(doc)
	merged := overlay.Merge(defaultOverlay, opts.Overlay)

	ctx.Scalars = toTemplateScalars(sanitizeScalars(merged.Scalars))
	ctx.Enums = toTemplateEnums(sanitizeEnums(merged.Enums))
	ctx.Inputs = toTemplateInputs(sanitizeInputs(merged.Inputs))
	ctx.Queries = toTemplateOperations(sanitizeOperations(merged.Queries))
	ctx.Mutations = toTemplateOperations(sanitizeOperations(merged.Mutations))

	ctx.Entities = append([]formatter.Entity{}, ctx.Entities...)
	ctx.Entities = append(ctx.Entities, buildPageInfo())
	sortEntities(ctx.Entities)

	ctx.ModelStructs, ctx.ModelEnums, ctx.ModelImports = buildModels(ctx)

	return ctx
}

func buildDefaultOverlay(doc formatter.Document) overlay.Overlay {
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
				ReturnType: entity.Name,
				List:       true,
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
		Scalars:   scalars,
		Enums:     enums,
		Inputs:    inputs,
		Queries:   queries,
		Mutations: mutations,
	}
}

func buildPageInfo() formatter.Entity {
	return formatter.Entity{
		Name: "PageInfo",
		Fields: []formatter.Field{
			{Name: "total", OriginalName: "total", Type: "Int", Required: true},
			{Name: "hasNextPage", OriginalName: "hasNextPage", Type: "Boolean", Required: true},
			{Name: "hasPreviousPage", OriginalName: "hasPreviousPage", Type: "Boolean", Required: true},
		},
	}
}

func buildEntityInputs(entity formatter.Entity) (overlay.Input, overlay.Input) {
	var createFields, updateFields []overlay.InputField

	for _, f := range entity.Fields {
		// Skip identifiers and relation fields; inputs should only expose scalars/FKs.
		if strings.EqualFold(f.OriginalName, "id") || f.ReadOnly || f.Relation != nil {
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
