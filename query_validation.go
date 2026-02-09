package crud

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// QueryValidationErrorCode identifies query validation failure categories.
type QueryValidationErrorCode string

const (
	QueryValidationUnsupportedOperator   QueryValidationErrorCode = "unsupported_operator"
	QueryValidationSearchColumnsRequired QueryValidationErrorCode = "search_columns_required"
)

// QueryValidationError provides typed query validation failures for strict mode.
type QueryValidationError struct {
	Code     QueryValidationErrorCode
	Field    string
	Operator string
	Search   string
}

func (e *QueryValidationError) Error() string {
	if e == nil {
		return "query validation error"
	}

	switch e.Code {
	case QueryValidationUnsupportedOperator:
		if e.Field != "" {
			return fmt.Sprintf("unsupported operator %q for field %q", e.Operator, e.Field)
		}
		return fmt.Sprintf("unsupported operator %q", e.Operator)
	case QueryValidationSearchColumnsRequired:
		return "search term provided but no search columns are configured"
	default:
		return "query validation error"
	}
}

var strictQueryValidation atomic.Bool

// SetStrictQueryValidation toggles strict query operator validation globally.
// When enabled, unsupported operators return QueryValidationError.
func SetStrictQueryValidation(enabled bool) {
	strictQueryValidation.Store(enabled)
}

// StrictQueryValidationEnabled reports the current global strict mode.
func StrictQueryValidationEnabled() bool {
	return strictQueryValidation.Load()
}

type resolvedOperator struct {
	token     string
	canonical string
	sql       string
}

var canonicalOperatorSQL = map[string]string{
	"eq":    "=",
	"ne":    "<>",
	"gt":    ">",
	"lt":    "<",
	"gte":   ">=",
	"lte":   "<=",
	"in":    "IN",
	"ilike": "ILIKE",
	"like":  "LIKE",
	"and":   "and",
	"or":    "or",
}

var canonicalOperatorBySQL = map[string]string{
	"=":     "eq",
	"<>":    "ne",
	">":     "gt",
	"<":     "lt",
	">=":    "gte",
	"<=":    "lte",
	"IN":    "in",
	"ILIKE": "ilike",
	"LIKE":  "like",
	"AND":   "and",
	"OR":    "or",
}

func parseFieldOperatorWithValidation(param string, strict bool) (string, resolvedOperator, error) {
	field := strings.TrimSpace(param)
	operatorToken := "eq"

	if strings.Contains(param, "__") {
		parts := strings.SplitN(param, "__", 2)
		field = strings.TrimSpace(parts[0])
		operatorToken = strings.TrimSpace(parts[1])
	}

	op, err := resolveOperatorToken(operatorToken, field, strict)
	if err != nil {
		return field, resolvedOperator{}, err
	}

	return field, op, nil
}

func resolveOperatorToken(token, field string, strict bool) (resolvedOperator, error) {
	rawToken := strings.TrimSpace(token)
	normalized := normalizeOperatorToken(rawToken)
	if normalized == "" {
		normalized = "eq"
	}

	// Canonical operators always work, even if SetOperatorMap replaces aliases.
	if canonicalSQL, ok := canonicalOperatorSQL[normalized]; ok {
		sqlOp := canonicalSQL
		if mapped, ok := operatorMap[normalized]; ok {
			if trimmed := strings.TrimSpace(mapped); trimmed != "" {
				sqlOp = trimmed
			}
		}
		return resolvedOperator{
			token:     normalized,
			canonical: normalized,
			sql:       sqlOp,
		}, nil
	}

	if mapped, ok := operatorMap[normalized]; ok {
		if trimmed := strings.TrimSpace(mapped); trimmed != "" {
			canonical := canonicalOperatorForSQL(trimmed)
			if canonical != "" {
				return resolvedOperator{
					token:     normalized,
					canonical: canonical,
					sql:       trimmed,
				}, nil
			}
		}
	}

	if strict {
		return resolvedOperator{}, &QueryValidationError{
			Code:     QueryValidationUnsupportedOperator,
			Field:    strings.TrimSpace(field),
			Operator: rawToken,
		}
	}

	eqSQL := canonicalOperatorSQL["eq"]
	if mapped, ok := operatorMap["eq"]; ok {
		if trimmed := strings.TrimSpace(mapped); trimmed != "" {
			eqSQL = trimmed
		}
	}

	return resolvedOperator{
		token:     "eq",
		canonical: "eq",
		sql:       eqSQL,
	}, nil
}

func canonicalOperatorForSQL(sql string) string {
	if canonical, ok := canonicalOperatorBySQL[normalizeSQLOperator(sql)]; ok {
		return canonical
	}
	return ""
}

func normalizeOperatorToken(op string) string {
	return strings.ToLower(strings.TrimSpace(op))
}

func normalizeSQLOperator(op string) string {
	return strings.ToUpper(strings.TrimSpace(op))
}
