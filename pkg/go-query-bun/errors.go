package querybun

import "fmt"

// ValidationErrorCode identifies strict query validation failures.
type ValidationErrorCode string

const (
	ValidationUnsupportedOperator   ValidationErrorCode = "unsupported_operator"
	ValidationSearchColumnsRequired ValidationErrorCode = "search_columns_required"
	ValidationFieldNotAllowed       ValidationErrorCode = "field_not_allowed"
)

// ValidationError provides typed strict-mode query validation failures.
type ValidationError struct {
	Code     ValidationErrorCode
	Field    string
	Operator string
	Search   string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "query validation error"
	}

	switch e.Code {
	case ValidationUnsupportedOperator:
		if e.Field != "" {
			return fmt.Sprintf("unsupported operator %q for field %q", e.Operator, e.Field)
		}
		return fmt.Sprintf("unsupported operator %q", e.Operator)
	case ValidationSearchColumnsRequired:
		return "search term provided but no search columns are configured"
	case ValidationFieldNotAllowed:
		if e.Field != "" {
			return fmt.Sprintf("field %q is not allowed", e.Field)
		}
		return "field is not allowed"
	default:
		return "query validation error"
	}
}
