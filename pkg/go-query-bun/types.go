package querybun

import "github.com/uptrace/bun"

// Criteria is compatible with github.com/goliatone/go-repository-bun SelectCriteria.
type Criteria func(*bun.SelectQuery) *bun.SelectQuery

// Predicate represents a normalized field/operator/value query predicate.
type Predicate struct {
	Field    string
	Operator string
	Values   []string
	RawKey   string
	RawValue any
}

// Operator is a resolved operator token with its canonical semantic operator
// and SQL representation.
type Operator struct {
	Token     string
	Canonical string
	SQL       string
}

// Order describes a trusted order clause after field resolution.
type Order struct {
	Field string
	Dir   string
}

// IncludeRequest describes a normalized include path. Bun Relation criteria
// remain adapter-owned by go-crud during this staging step.
type IncludeRequest struct {
	Path string
}

// UnsupportedReason identifies why a query fragment was not planned.
type UnsupportedReason string

const (
	UnsupportedUnknownField    UnsupportedReason = "unknown_field"
	UnsupportedDisallowedField UnsupportedReason = "disallowed_field"
	UnsupportedOperator        UnsupportedReason = "unsupported_operator"
	UnsupportedEmptyValue      UnsupportedReason = "empty_value"
	UnsupportedValueShape      UnsupportedReason = "unsupported_value_shape"
)

// UnsupportedPredicate records a dropped or rejected query predicate.
type UnsupportedPredicate struct {
	Field    string
	Operator string
	RawKey   string
	RawValue any
	Reason   UnsupportedReason
}
