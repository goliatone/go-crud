package overlay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Overlay enriches SchemaMetadata with additional SDL constructs.
type Overlay struct {
	Scalars   []Scalar    `json:"scalars" yaml:"scalars"`
	Enums     []Enum      `json:"enums" yaml:"enums"`
	Inputs    []Input     `json:"inputs" yaml:"inputs"`
	Queries   []Operation `json:"queries" yaml:"queries"`
	Mutations []Operation `json:"mutations" yaml:"mutations"`
	Hooks     Hooks       `json:"hooks" yaml:"hooks"`
}

// Scalar describes a custom scalar and its optional Go type mapping.
type Scalar struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	GoType      string `json:"go_type,omitempty" yaml:"go_type,omitempty"`
}

// Enum captures enum definitions.
type Enum struct {
	Name        string      `json:"name" yaml:"name"`
	Description string      `json:"description,omitempty" yaml:"description,omitempty"`
	Values      []EnumValue `json:"values" yaml:"values"`
}

// EnumValue is a single enum entry.
type EnumValue struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Input describes an input object.
type Input struct {
	Name        string       `json:"name" yaml:"name"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Fields      []InputField `json:"fields" yaml:"fields"`
}

// InputField models a field inside an input object.
type InputField struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	List        bool   `json:"list,omitempty" yaml:"list,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// Operation describes a Query or Mutation operation signature.
type Operation struct {
	Name        string     `json:"name" yaml:"name"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	ReturnType  string     `json:"return_type" yaml:"return_type"`
	List        bool       `json:"list,omitempty" yaml:"list,omitempty"`
	Required    bool       `json:"required,omitempty" yaml:"required,omitempty"`
	Args        []Argument `json:"args,omitempty" yaml:"args,omitempty"`
}

// Argument defines a single argument for an operation.
type Argument struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	List        bool   `json:"list,omitempty" yaml:"list,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// Hooks defines resolver hook snippets and required imports.
type Hooks struct {
	Imports  []string               `json:"imports,omitempty" yaml:"imports,omitempty"`
	Default  HookSet                `json:"default,omitempty" yaml:"default,omitempty"`
	Entities map[string]EntityHooks `json:"entities,omitempty" yaml:"entities,omitempty"`
}

// EntityHooks configures hooks for a single entity.
type EntityHooks struct {
	Imports    []string           `json:"imports,omitempty" yaml:"imports,omitempty"`
	All        HookSet            `json:"all,omitempty" yaml:"all,omitempty"`
	Operations map[string]HookSet `json:"operations,omitempty" yaml:"operations,omitempty"`
}

// HookSet captures hook snippets for a given operation.
type HookSet struct {
	Imports      []string `json:"imports,omitempty" yaml:"imports,omitempty"`
	AuthGuard    string   `json:"auth_guard,omitempty" yaml:"auth_guard,omitempty"`
	ScopeGuard   string   `json:"scope_guard,omitempty" yaml:"scope_guard,omitempty"`
	Preload      string   `json:"preload,omitempty" yaml:"preload,omitempty"`
	WrapRepo     string   `json:"wrap_repo,omitempty" yaml:"wrap_repo,omitempty"`
	ErrorHandler string   `json:"error_handler,omitempty" yaml:"error_handler,omitempty"`
}

// Load reads an overlay from JSON or YAML. Empty path returns an empty overlay.
func Load(path string) (Overlay, error) {
	if path == "" {
		return Overlay{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Overlay{}, fmt.Errorf("read overlay: %w", err)
	}

	ext := filepath.Ext(path)
	if ext == ".yaml" || ext == ".yml" {
		var o Overlay
		if err := yaml.Unmarshal(data, &o); err != nil {
			return Overlay{}, fmt.Errorf("decode overlay yaml: %w", err)
		}
		return o, nil
	}

	var o Overlay
	if err := json.Unmarshal(data, &o); err == nil {
		return o, nil
	}

	// Try YAML as a fallback.
	if err := yaml.Unmarshal(data, &o); err != nil {
		return Overlay{}, fmt.Errorf("decode overlay: %w", err)
	}
	return o, nil
}

// Merge combines base and extra overlays by name, with extra taking precedence.
func Merge(base, extra Overlay) Overlay {
	out := base
	out.Scalars = mergeScalars(base.Scalars, extra.Scalars)
	out.Enums = mergeEnums(base.Enums, extra.Enums)
	out.Inputs = mergeInputs(base.Inputs, extra.Inputs)
	out.Hooks = mergeHooks(base.Hooks, extra.Hooks)
	out.Queries = append([]Operation{}, base.Queries...)
	out.Queries = append(out.Queries, extra.Queries...)
	out.Mutations = append([]Operation{}, base.Mutations...)
	out.Mutations = append(out.Mutations, extra.Mutations...)
	return out
}

func mergeScalars(base, extra []Scalar) []Scalar {
	result := make([]Scalar, 0, len(base)+len(extra))
	seen := make(map[string]int)
	for _, s := range base {
		seen[s.Name] = len(result)
		result = append(result, s)
	}
	for _, s := range extra {
		if idx, ok := seen[s.Name]; ok {
			result[idx] = s
			continue
		}
		seen[s.Name] = len(result)
		result = append(result, s)
	}
	return result
}

func mergeEnums(base, extra []Enum) []Enum {
	result := make([]Enum, 0, len(base)+len(extra))
	seen := make(map[string]int)
	for _, e := range base {
		seen[e.Name] = len(result)
		result = append(result, e)
	}
	for _, e := range extra {
		if idx, ok := seen[e.Name]; ok {
			result[idx] = e
			continue
		}
		seen[e.Name] = len(result)
		result = append(result, e)
	}
	return result
}

func mergeInputs(base, extra []Input) []Input {
	result := make([]Input, 0, len(base)+len(extra))
	seen := make(map[string]int)
	for _, in := range base {
		seen[in.Name] = len(result)
		result = append(result, in)
	}
	for _, in := range extra {
		if idx, ok := seen[in.Name]; ok {
			result[idx] = in
			continue
		}
		seen[in.Name] = len(result)
		result = append(result, in)
	}
	return result
}

func mergeHooks(base, extra Hooks) Hooks {
	out := Hooks{
		Imports: append([]string{}, base.Imports...),
		Default: mergeHookSet(base.Default, extra.Default),
	}
	if len(extra.Imports) > 0 {
		out.Imports = append(out.Imports, extra.Imports...)
	}

	if len(base.Entities) == 0 && len(extra.Entities) == 0 {
		return out
	}

	out.Entities = make(map[string]EntityHooks, len(base.Entities)+len(extra.Entities))
	for name, hooks := range base.Entities {
		out.Entities[name] = hooks
	}

	for name, hooks := range extra.Entities {
		current := out.Entities[name]
		current.Imports = append(current.Imports, hooks.Imports...)
		current.All = mergeHookSet(current.All, hooks.All)
		if len(current.Operations) == 0 && len(hooks.Operations) > 0 {
			current.Operations = make(map[string]HookSet, len(hooks.Operations))
		}
		for op, set := range hooks.Operations {
			current.Operations[op] = mergeHookSet(current.Operations[op], set)
		}
		out.Entities[name] = current
	}

	return out
}

func mergeHookSet(base, extra HookSet) HookSet {
	out := base
	if extra.AuthGuard != "" {
		out.AuthGuard = extra.AuthGuard
	}
	if extra.ScopeGuard != "" {
		out.ScopeGuard = extra.ScopeGuard
	}
	if extra.Preload != "" {
		out.Preload = extra.Preload
	}
	if extra.WrapRepo != "" {
		out.WrapRepo = extra.WrapRepo
	}
	if extra.ErrorHandler != "" {
		out.ErrorHandler = extra.ErrorHandler
	}
	if len(extra.Imports) > 0 {
		out.Imports = append(out.Imports, extra.Imports...)
	}
	return out
}
