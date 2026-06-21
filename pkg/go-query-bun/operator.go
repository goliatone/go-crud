package querybun

import (
	"maps"
	"strings"
)

// OperatorSpec describes how a semantic operator renders.
type OperatorSpec struct {
	Kind       OperatorKind
	SQL        string
	Pattern    PatternConfig
	Case       CaseMode
	MultiValue MultiValueMode
}

// OperatorSet resolves aliases to semantic operator specs.
type OperatorSet struct {
	Specs   map[OperatorKind]OperatorSpec
	Aliases map[string]OperatorKind
}

// DefaultOperatorSet returns the built-in semantic operators.
func DefaultOperatorSet() OperatorSet {
	specs := map[OperatorKind]OperatorSpec{
		OperatorEq:       {Kind: OperatorEq, SQL: "=", MultiValue: MultiValueAll},
		OperatorNe:       {Kind: OperatorNe, SQL: "<>", MultiValue: MultiValueAll},
		OperatorGt:       {Kind: OperatorGt, SQL: ">", MultiValue: MultiValueAll},
		OperatorLt:       {Kind: OperatorLt, SQL: "<", MultiValue: MultiValueAll},
		OperatorGte:      {Kind: OperatorGte, SQL: ">=", MultiValue: MultiValueAll},
		OperatorLte:      {Kind: OperatorLte, SQL: "<=", MultiValue: MultiValueAll},
		OperatorIn:       {Kind: OperatorIn, SQL: "IN", MultiValue: MultiValueAny},
		OperatorNin:      {Kind: OperatorNin, SQL: "NOT IN", MultiValue: MultiValueAny},
		OperatorLike:     {Kind: OperatorLike, SQL: "LIKE", Pattern: PatternConfig{Mode: PatternRaw, EscapeChar: `\`, MultiValue: MultiValueAll}, Case: CaseSensitive, MultiValue: MultiValueAll},
		OperatorILike:    {Kind: OperatorILike, SQL: "ILIKE", Pattern: PatternConfig{Mode: PatternRaw, EscapeChar: `\`, MultiValue: MultiValueAll}, Case: CaseInsensitive, MultiValue: MultiValueAll},
		OperatorContains: {Kind: OperatorContains, SQL: "LIKE", Pattern: PatternConfig{Mode: PatternContainsEscaped, EscapeChar: `\`, MultiValue: MultiValueAny}, Case: CaseSensitive, MultiValue: MultiValueAny},
		OperatorAnd:      {Kind: OperatorAnd, SQL: "and", MultiValue: MultiValueAll},
		OperatorOr:       {Kind: OperatorOr, SQL: "or", MultiValue: MultiValueAny},
	}

	aliases := map[string]OperatorKind{}
	for kind := range specs {
		aliases[string(kind)] = kind
		aliases["$"+string(kind)] = kind
	}
	aliases["neq"] = OperatorNe
	aliases["$neq"] = OperatorNe
	aliases["not_in"] = OperatorNin
	aliases["notin"] = OperatorNin
	aliases["$not_in"] = OperatorNin
	aliases["$notin"] = OperatorNin

	return OperatorSet{Specs: specs, Aliases: aliases}
}

// WithAlias returns a copy with one additional alias.
func (s OperatorSet) WithAlias(alias string, kind OperatorKind) OperatorSet {
	s = s.normalized()
	alias = normalizeOperatorToken(alias)
	if alias == "" || kind == "" {
		return s
	}
	s.Aliases[alias] = kind
	return s
}

// WithSpec returns a copy with a replaced or added operator spec.
func (s OperatorSet) WithSpec(spec OperatorSpec, aliases ...string) OperatorSet {
	s = s.normalized()
	if spec.Kind == "" {
		return s
	}
	if spec.SQL == "" {
		if existing, ok := s.Specs[spec.Kind]; ok {
			spec.SQL = existing.SQL
		}
	}
	if spec.Pattern.EscapeChar == "" {
		spec.Pattern.EscapeChar = `\`
	}
	if spec.Pattern.MultiValue == "" {
		spec.Pattern.MultiValue = spec.MultiValue
	}
	if spec.MultiValue == "" {
		spec.MultiValue = spec.Pattern.MultiValue
	}
	if spec.MultiValue == "" {
		spec.MultiValue = MultiValueAll
	}
	s.Specs[spec.Kind] = spec
	s.Aliases[string(spec.Kind)] = spec.Kind
	for _, alias := range aliases {
		if normalized := normalizeOperatorToken(alias); normalized != "" {
			s.Aliases[normalized] = spec.Kind
		}
	}
	return s
}

// Resolve resolves a user token to an operator.
func (s OperatorSet) Resolve(token string) (Operator, bool) {
	s = s.normalized()
	rawToken := strings.TrimSpace(token)
	normalized := normalizeOperatorToken(rawToken)
	if normalized == "" {
		normalized = string(OperatorEq)
	}
	kind, ok := s.Aliases[normalized]
	if !ok {
		kind = OperatorKind(normalized)
	}
	spec, ok := s.Specs[kind]
	if !ok {
		return Operator{}, false
	}
	if spec.Kind == "" {
		spec.Kind = kind
	}
	if spec.Pattern.EscapeChar == "" {
		spec.Pattern.EscapeChar = `\`
	}
	if spec.Pattern.MultiValue == "" {
		spec.Pattern.MultiValue = spec.MultiValue
	}
	if spec.MultiValue == "" {
		spec.MultiValue = spec.Pattern.MultiValue
	}
	if spec.MultiValue == "" {
		spec.MultiValue = MultiValueAll
	}
	return Operator{
		Token:     normalized,
		Canonical: spec.Kind,
		SQL:       spec.SQL,
		Spec:      spec,
	}, true
}

// ResolveOperator resolves token into a semantic operator.
func ResolveOperator(token, field string, cfg Config) (Operator, error) {
	cfg = normalizeConfig(cfg)
	operator, ok := cfg.Operators.Resolve(token)
	if ok {
		return operator, nil
	}
	rawToken := strings.TrimSpace(token)
	if rawToken == "" {
		rawToken = string(OperatorEq)
	}
	return Operator{}, &ValidationError{
		Code:     ValidationUnsupportedOperator,
		Field:    strings.TrimSpace(field),
		Operator: rawToken,
	}
}

func (s OperatorSet) normalized() OperatorSet {
	defaults := DefaultOperatorSet()
	if len(s.Specs) == 0 {
		s.Specs = cloneOperatorSpecs(defaults.Specs)
	} else {
		specs := cloneOperatorSpecs(defaults.Specs)
		for kind, spec := range s.Specs {
			if spec.Kind == "" {
				spec.Kind = kind
			}
			specs[kind] = spec
		}
		s.Specs = specs
	}
	if len(s.Aliases) == 0 {
		s.Aliases = cloneOperatorAliasMap(defaults.Aliases)
	} else {
		aliases := cloneOperatorAliasMap(defaults.Aliases)
		for alias, kind := range s.Aliases {
			if normalized := normalizeOperatorToken(alias); normalized != "" && kind != "" {
				aliases[normalized] = kind
			}
		}
		s.Aliases = aliases
	}
	return s
}

func normalizeOperatorToken(op string) string {
	return strings.ToLower(strings.TrimSpace(op))
}

func cloneOperatorSpecs(in map[OperatorKind]OperatorSpec) map[OperatorKind]OperatorSpec {
	if len(in) == 0 {
		return nil
	}
	out := make(map[OperatorKind]OperatorSpec, len(in))
	maps.Copy(out, in)
	return out
}

func cloneOperatorAliasMap(in map[string]OperatorKind) map[string]OperatorKind {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]OperatorKind, len(in))
	for key, value := range in {
		if normalized := normalizeOperatorToken(key); normalized != "" && value != "" {
			out[normalized] = value
		}
	}
	return out
}
