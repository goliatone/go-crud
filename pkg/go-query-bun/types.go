package querybun

import "github.com/uptrace/bun"

// Criteria is compatible with github.com/goliatone/go-repository-bun SelectCriteria.
type Criteria func(*bun.SelectQuery) *bun.SelectQuery

// OperatorKind identifies a semantic predicate operator independent of SQL.
type OperatorKind string

const (
	OperatorEq       OperatorKind = "eq"
	OperatorNe       OperatorKind = "ne"
	OperatorGt       OperatorKind = "gt"
	OperatorLt       OperatorKind = "lt"
	OperatorGte      OperatorKind = "gte"
	OperatorLte      OperatorKind = "lte"
	OperatorIn       OperatorKind = "in"
	OperatorNin      OperatorKind = "nin"
	OperatorLike     OperatorKind = "like"
	OperatorILike    OperatorKind = "ilike"
	OperatorContains OperatorKind = "contains"
	OperatorAnd      OperatorKind = "and"
	OperatorOr       OperatorKind = "or"
)

// LogicalKind identifies cross-field boolean expression nodes.
type LogicalKind string

const (
	LogicalAnd LogicalKind = "and"
	LogicalOr  LogicalKind = "or"
	LogicalNot LogicalKind = "not"
)

// FieldUse describes the planner section requesting a field resolution.
type FieldUse string

const (
	FieldUseFilter FieldUse = "filter"
	FieldUseOrder  FieldUse = "order"
	FieldUseSearch FieldUse = "search"
	FieldUseSelect FieldUse = "select"
)

// Predicate represents a normalized field/operator/value query predicate.
type Predicate struct {
	Field    string
	Operator OperatorKind
	Values   []string
	RawKey   string
	RawValue any
}

// Expression is the marker interface for future nested logical query ASTs.
type Expression interface {
	queryExpressionNode()
}

// PredicateExpression wraps a normalized field predicate as an expression node.
type PredicateExpression struct {
	Predicate Predicate
}

func (PredicateExpression) queryExpressionNode() {}

// LogicalExpression models nested cross-field boolean input. SQL rendering for
// these nodes is intentionally not enabled by default.
type LogicalExpression struct {
	Kind     LogicalKind
	Children []Expression
	RawKey   string
	RawValue any
}

func (LogicalExpression) queryExpressionNode() {}

// Operator is a resolved operator token with its semantic kind and SQL behavior.
type Operator struct {
	Token     string
	Canonical OperatorKind
	SQL       string
	Spec      OperatorSpec
}

// FieldRequest describes a request to resolve a public field name.
type FieldRequest struct {
	Field string
	Use   FieldUse
}

// ResolvedField describes a trusted SQL column or expression.
type ResolvedField struct {
	Field      string
	Column     string
	Expression string
	Use        FieldUse
	Trusted    bool
}

// SQLName returns the SQL expression used by the planner.
func (f ResolvedField) SQLName() string {
	if f.Expression != "" {
		return f.Expression
	}
	return f.Column
}

// FieldResolver resolves public field names for filter, order, search, and select use.
type FieldResolver interface {
	ResolveField(FieldRequest) (ResolvedField, bool)
}

// FieldResolverFunc adapts a function to FieldResolver.
type FieldResolverFunc func(FieldRequest) (ResolvedField, bool)

// ResolveField implements FieldResolver.
func (fn FieldResolverFunc) ResolveField(req FieldRequest) (ResolvedField, bool) {
	if fn == nil {
		return ResolvedField{}, false
	}
	return fn(req)
}

// Order describes a trusted order clause after field resolution.
type Order struct {
	Field string
	Dir   string
}

// IncludeRequest describes a normalized include path. Bun Relation criteria
// remain adapter-owned by go-crud.
type IncludeRequest struct {
	Path string
}

// UnsupportedReason identifies why a query fragment was not planned.
type UnsupportedReason string

const (
	UnsupportedUnknownField       UnsupportedReason = "unknown_field"
	UnsupportedDisallowedField    UnsupportedReason = "disallowed_field"
	UnsupportedOperator           UnsupportedReason = "unsupported_operator"
	UnsupportedEmptyValue         UnsupportedReason = "empty_value"
	UnsupportedValueShape         UnsupportedReason = "unsupported_value_shape"
	UnsupportedUnhandledPredicate UnsupportedReason = "unhandled_custom_predicate"
)

// UnsupportedPredicate records a dropped or rejected query predicate.
type UnsupportedPredicate struct {
	Field    string
	Operator OperatorKind
	RawKey   string
	RawValue any
	Reason   UnsupportedReason
}

// PredicateBuilder can render custom field or operator predicates.
type PredicateBuilder interface {
	BuildPredicate(PredicateBuilderInput) (PredicateBuilderResult, error)
}

// PredicateBuilderFunc adapts a function to PredicateBuilder.
type PredicateBuilderFunc func(PredicateBuilderInput) (PredicateBuilderResult, error)

// BuildPredicate implements PredicateBuilder.
func (fn PredicateBuilderFunc) BuildPredicate(in PredicateBuilderInput) (PredicateBuilderResult, error) {
	if fn == nil {
		return PredicateBuilderResult{}, nil
	}
	return fn(in)
}

// PredicateBuilderInput describes one predicate planning request.
type PredicateBuilderInput struct {
	Predicate     Predicate
	Operator      Operator
	Field         ResolvedField
	FieldResolved bool
	Values        []string
	Config        Config
	Helpers       PredicateBuilderHelpers
}

// PredicateBuilderResult reports whether a custom builder handled a predicate.
type PredicateBuilderResult struct {
	Criteria []Criteria
	Handled  bool
}

// PredicateHandled returns a handled custom predicate result.
func PredicateHandled(criteria ...Criteria) PredicateBuilderResult {
	return PredicateBuilderResult{Criteria: criteria, Handled: true}
}

// PredicateUnhandled returns an unhandled custom predicate result.
func PredicateUnhandled() PredicateBuilderResult {
	return PredicateBuilderResult{}
}

// PredicateBuilderSet groups field-specific and operator-specific builders.
type PredicateBuilderSet struct {
	Fields    map[string]PredicateBuilder
	Operators map[OperatorKind]PredicateBuilder
}

// PredicateBuilderHelpers exposes small helpers for custom predicate builders.
type PredicateBuilderHelpers struct{}
