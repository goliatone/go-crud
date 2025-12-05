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
