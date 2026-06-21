package querybun

import (
	"maps"
	"strings"
)

const (
	DefaultLimit  = 25
	DefaultOffset = 0
)

// ListOptions provides a non-HTTP contract for list query planning.
type ListOptions struct {
	Page       int
	PerPage    int
	Limit      int
	LimitSet   bool
	Offset     int
	OffsetSet  bool
	SortBy     string
	SortDesc   bool
	Order      string
	Search     string
	Filters    map[string]any
	Predicates []Predicate
	Select     []string
	Include    []string
}

// Config controls syntax, field resolution, operators, search, predicates,
// pagination, and validation.
type Config struct {
	Syntax     QuerySyntax
	Fields     FieldConfig
	Operators  OperatorSet
	Search     SearchConfig
	Predicates PredicateBuilderSet
	Pagination PaginationConfig
	Validation ValidationConfig
}

// FieldConfig groups field resolution behavior.
type FieldConfig struct {
	Resolver FieldResolver
}

// QuerySyntax owns generic query decoding behavior outside search aliases.
type QuerySyntax struct {
	PredicateDelimiter string
	ReservedKeys       map[string]struct{}
	OperatorAliases    map[string]OperatorKind
	LogicalAliases     map[string]LogicalKind
}

// SearchConfig owns search aliases, fields, and matching behavior.
type SearchConfig struct {
	Aliases []string
	Fields  []string
	Pattern PatternConfig
	Case    CaseMode
}

// PatternMode controls how user input becomes a SQL pattern.
type PatternMode string

const (
	PatternRaw             PatternMode = "raw"
	PatternContains        PatternMode = "contains"
	PatternContainsEscaped PatternMode = "contains_escaped"
)

// CaseMode controls case-sensitive or case-insensitive pattern matching.
type CaseMode string

const (
	CaseSensitive   CaseMode = "sensitive"
	CaseInsensitive CaseMode = "insensitive"
)

// MultiValueMode controls whether repeated values are ANDed or ORed.
type MultiValueMode string

const (
	MultiValueAll MultiValueMode = "all"
	MultiValueAny MultiValueMode = "any"
)

// PatternConfig controls pattern wrapping and escaping.
type PatternConfig struct {
	Mode       PatternMode
	EscapeChar string
	MultiValue MultiValueMode
}

// PaginationConfig groups pagination defaults.
type PaginationConfig struct {
	DefaultLimit  int
	DefaultOffset int
}

// ValidationMode chooses strict errors or collect-style unsupported metadata.
type ValidationMode string

const (
	ValidationCollect ValidationMode = "collect"
	ValidationStrict  ValidationMode = "strict"
)

// ValidationConfig controls strict versus collect behavior.
type ValidationConfig struct {
	Mode               ValidationMode
	StrictFields       bool
	StrictSearchFields bool
}

// DefaultConfig returns the primary semantic planner configuration.
func DefaultConfig() Config {
	return Config{
		Syntax:    DefaultRESTSyntax(),
		Operators: DefaultOperatorSet(),
		Search: SearchConfig{
			Aliases: []string{"_search", "search", "q"},
			Pattern: PatternConfig{Mode: PatternContains, EscapeChar: `\`, MultiValue: MultiValueAny},
			Case:    CaseInsensitive,
		},
		Pagination: PaginationConfig{
			DefaultLimit:  DefaultLimit,
			DefaultOffset: DefaultOffset,
		},
		Validation: ValidationConfig{Mode: ValidationCollect},
	}
}

// DefaultRESTSyntax returns the current REST field__operator query syntax.
func DefaultRESTSyntax() QuerySyntax {
	operators := map[string]OperatorKind{
		"eq": OperatorEq, "$eq": OperatorEq,
		"ne": OperatorNe, "neq": OperatorNe, "$ne": OperatorNe, "$neq": OperatorNe,
		"gt": OperatorGt, "$gt": OperatorGt,
		"lt": OperatorLt, "$lt": OperatorLt,
		"gte": OperatorGte, "$gte": OperatorGte,
		"lte": OperatorLte, "$lte": OperatorLte,
		"in": OperatorIn, "$in": OperatorIn,
		"nin": OperatorNin, "not_in": OperatorNin, "$nin": OperatorNin,
		"like": OperatorLike, "$like": OperatorLike,
		"ilike": OperatorILike, "$ilike": OperatorILike,
		"contains": OperatorContains, "$contains": OperatorContains,
		"and": OperatorAnd, "$and": OperatorAnd,
		"or": OperatorOr, "$or": OperatorOr,
	}
	return QuerySyntax{
		PredicateDelimiter: "__",
		ReservedKeys: map[string]struct{}{
			"limit": {}, "offset": {}, "page": {}, "per_page": {},
			"order": {}, "sort": {}, "sort_by": {}, "sort_desc": {},
			"select": {}, "include": {},
		},
		OperatorAliases: operators,
		LogicalAliases: map[string]LogicalKind{
			"and": LogicalAnd, "$and": LogicalAnd,
			"or": LogicalOr, "$or": LogicalOr,
			"not": LogicalNot, "$not": LogicalNot,
		},
	}
}

// MapResolver maps trusted public field names to trusted SQL columns or expressions.
type MapResolver map[string]string

// NewMapResolver returns a trusted map resolver with copied values.
func NewMapResolver(fields map[string]string) MapResolver {
	out := make(MapResolver, len(fields))
	for key, value := range fields {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

// ResolveField implements FieldResolver.
func (r MapResolver) ResolveField(req FieldRequest) (ResolvedField, bool) {
	field := strings.TrimSpace(req.Field)
	if field == "" || len(r) == 0 {
		return ResolvedField{}, false
	}
	column, ok := r[field]
	if !ok || strings.TrimSpace(column) == "" {
		return ResolvedField{}, false
	}
	return ResolvedField{
		Field:   field,
		Column:  strings.TrimSpace(column),
		Use:     req.Use,
		Trusted: true,
	}, true
}

// SafeIdentifierResolver resolves only simple SQL identifiers.
type SafeIdentifierResolver struct {
	AllowDotted bool
}

// ResolveField implements FieldResolver.
func (r SafeIdentifierResolver) ResolveField(req FieldRequest) (ResolvedField, bool) {
	field := strings.TrimSpace(req.Field)
	if !isSafeIdentifier(field, r.AllowDotted) {
		return ResolvedField{}, false
	}
	return ResolvedField{
		Field:   field,
		Column:  field,
		Use:     req.Use,
		Trusted: true,
	}, true
}

// ComposedResolver tries resolvers in order.
type ComposedResolver []FieldResolver

// ComposeResolvers creates a resolver chain.
func ComposeResolvers(resolvers ...FieldResolver) ComposedResolver {
	out := make(ComposedResolver, 0, len(resolvers))
	for _, resolver := range resolvers {
		if resolver != nil {
			out = append(out, resolver)
		}
	}
	return out
}

// ResolveField implements FieldResolver.
func (r ComposedResolver) ResolveField(req FieldRequest) (ResolvedField, bool) {
	for _, resolver := range r {
		if resolver == nil {
			continue
		}
		if field, ok := resolver.ResolveField(req); ok {
			return field, true
		}
	}
	return ResolvedField{}, false
}

func normalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()

	if cfg.Syntax.PredicateDelimiter == "" {
		cfg.Syntax.PredicateDelimiter = defaults.Syntax.PredicateDelimiter
	}
	if cfg.Syntax.ReservedKeys == nil {
		cfg.Syntax.ReservedKeys = defaults.Syntax.ReservedKeys
	} else {
		cfg.Syntax.ReservedKeys = cloneStringSet(cfg.Syntax.ReservedKeys)
	}
	if cfg.Syntax.OperatorAliases == nil {
		cfg.Syntax.OperatorAliases = defaults.Syntax.OperatorAliases
	} else {
		cfg.Syntax.OperatorAliases = cloneOperatorAliases(cfg.Syntax.OperatorAliases)
	}
	if cfg.Syntax.LogicalAliases == nil {
		cfg.Syntax.LogicalAliases = defaults.Syntax.LogicalAliases
	} else {
		cfg.Syntax.LogicalAliases = cloneLogicalAliases(cfg.Syntax.LogicalAliases)
	}

	cfg.Operators = cfg.Operators.normalized()

	cfg.Search.Aliases = cloneStringSliceOrDefault(cfg.Search.Aliases, defaults.Search.Aliases)
	cfg.Search.Fields = append([]string{}, cfg.Search.Fields...)
	if cfg.Search.Pattern.Mode == "" {
		cfg.Search.Pattern = defaults.Search.Pattern
	}
	if cfg.Search.Pattern.EscapeChar == "" {
		cfg.Search.Pattern.EscapeChar = `\`
	}
	if cfg.Search.Pattern.MultiValue == "" {
		cfg.Search.Pattern.MultiValue = MultiValueAny
	}
	if cfg.Search.Case == "" {
		cfg.Search.Case = defaults.Search.Case
	}

	cfg.Predicates = cfg.Predicates.clone()

	if cfg.Pagination.DefaultLimit == 0 {
		cfg.Pagination.DefaultLimit = defaults.Pagination.DefaultLimit
	}
	if cfg.Validation.Mode == "" {
		cfg.Validation.Mode = ValidationCollect
	}

	return cfg
}

func resolveField(cfg Config, field string, use FieldUse) (ResolvedField, bool) {
	if cfg.Fields.Resolver == nil {
		return ResolvedField{}, false
	}
	return cfg.Fields.Resolver.ResolveField(FieldRequest{Field: field, Use: use})
}

func (s QuerySyntax) parsePredicateKey(key string) (string, OperatorKind) {
	delimiter := s.PredicateDelimiter
	if delimiter == "" {
		delimiter = "__"
	}
	parts := strings.SplitN(strings.TrimSpace(key), delimiter, 2)
	field := strings.TrimSpace(parts[0])
	operator := OperatorEq
	if len(parts) == 2 {
		operator = s.resolveOperatorAlias(parts[1])
	}
	return field, operator
}

func (s QuerySyntax) resolveOperatorAlias(token string) OperatorKind {
	normalized := normalizeOperatorToken(token)
	if normalized == "" {
		return OperatorEq
	}
	if s.OperatorAliases != nil {
		if kind, ok := s.OperatorAliases[normalized]; ok && kind != "" {
			return kind
		}
	}
	return OperatorKind(normalized)
}

func (s QuerySyntax) resolveLogicalAlias(token string) (LogicalKind, bool) {
	normalized := normalizeOperatorToken(token)
	if normalized == "" {
		return "", false
	}
	if s.LogicalAliases != nil {
		kind, ok := s.LogicalAliases[normalized]
		return kind, ok
	}
	return "", false
}

func (s QuerySyntax) isReservedKey(key string) bool {
	if s.ReservedKeys == nil {
		return false
	}
	_, ok := s.ReservedKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

func (s SearchConfig) isSearchAlias(field string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	if field == "" {
		return false
	}
	for _, alias := range s.Aliases {
		if field == strings.ToLower(strings.TrimSpace(alias)) {
			return true
		}
	}
	return false
}

func (s PredicateBuilderSet) clone() PredicateBuilderSet {
	out := PredicateBuilderSet{}
	if len(s.Fields) > 0 {
		out.Fields = make(map[string]PredicateBuilder, len(s.Fields))
		maps.Copy(out.Fields, s.Fields)
	}
	if len(s.Operators) > 0 {
		out.Operators = make(map[OperatorKind]PredicateBuilder, len(s.Operators))
		maps.Copy(out.Operators, s.Operators)
	}
	return out
}

func isStrict(cfg Config) bool {
	return cfg.Validation.Mode == ValidationStrict
}

func cloneStringSliceOrDefault(values, defaults []string) []string {
	if len(values) == 0 {
		return append([]string{}, defaults...)
	}
	return append([]string{}, values...)
}

func cloneStringSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	maps.Copy(out, in)
	return out
}

func cloneOperatorAliases(in map[string]OperatorKind) map[string]OperatorKind {
	out := make(map[string]OperatorKind, len(in))
	maps.Copy(out, in)
	return out
}

func cloneLogicalAliases(in map[string]LogicalKind) map[string]LogicalKind {
	out := make(map[string]LogicalKind, len(in))
	maps.Copy(out, in)
	return out
}

func isSafeIdentifier(identifier string, allowDotted bool) bool {
	if identifier == "" {
		return false
	}
	if allowDotted {
		parts := strings.Split(identifier, ".")
		if len(parts) == 0 {
			return false
		}
		for _, part := range parts {
			if !isSafeIdentifierPart(part) {
				return false
			}
		}
		return true
	}
	return isSafeIdentifierPart(identifier)
}

func isSafeIdentifierPart(identifier string) bool {
	if identifier == "" {
		return false
	}
	for i, r := range identifier {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	first := identifier[0]
	return (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_'
}
