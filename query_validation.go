package crud

import (
	"fmt"
	"strings"
	"sync/atomic"

	querybun "github.com/goliatone/go-crud/pkg/go-query-bun"
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

func parseFieldOperatorWithValidation(param string, strict bool) (string, resolvedOperator, error) {
	field := strings.TrimSpace(param)
	operatorToken := "eq"

	if strings.Contains(param, "__") {
		parts := strings.SplitN(param, "__", 2)
		field = strings.TrimSpace(parts[0])
		operatorToken = strings.TrimSpace(parts[1])
	}

	op, err := querybun.ResolveOperator(operatorToken, field, querybun.Config{
		OperatorMap:                  operatorMap,
		StrictValidation:             strict,
		FallbackUnsupportedOperators: true,
	})
	if err != nil {
		return field, resolvedOperator{}, convertQueryBunError(err)
	}

	return field, resolvedOperator{
		token:     op.Token,
		canonical: op.Canonical,
		sql:       op.SQL,
	}, nil
}
