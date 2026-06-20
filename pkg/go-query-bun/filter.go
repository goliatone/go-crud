package querybun

import (
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

// BuildFilterCriteriaFromPredicates builds Bun WHERE criteria from normalized
// predicates and a trusted allowed field map.
func BuildFilterCriteriaFromPredicates(predicates []Predicate, cfg Config) ([]Criteria, []UnsupportedPredicate, error) {
	if len(predicates) == 0 {
		return nil, nil, nil
	}

	allowedFields := cloneStringMap(cfg.AllowedFields)
	unsupported := make([]UnsupportedPredicate, 0)
	andConditions := make([]Criteria, 0)
	orGroups := make([]Criteria, 0)

	for _, predicate := range predicates {
		field := strings.TrimSpace(predicate.Field)
		operatorToken := strings.ToLower(strings.TrimSpace(predicate.Operator))
		if operatorToken == "" {
			operatorToken = "eq"
		}

		if field == "" {
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedUnknownField))
			continue
		}

		columnName, ok := allowedFields[field]
		if !ok || strings.TrimSpace(columnName) == "" {
			reason := UnsupportedDisallowedField
			if len(allowedFields) == 0 {
				reason = UnsupportedUnknownField
			}
			entry := unsupportedFromPredicate(predicate, reason)
			unsupported = append(unsupported, entry)
			if cfg.StrictFields {
				return nil, unsupported, &ValidationError{
					Code:  ValidationFieldNotAllowed,
					Field: field,
				}
			}
			continue
		}

		cleaned := normalizeStringValues(predicate.Values)
		if len(cleaned) == 0 {
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedEmptyValue))
			continue
		}

		operator, operatorOK, err := resolveFilterOperator(operatorToken, field, cfg)
		if err != nil {
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedOperator))
			return nil, unsupported, err
		}
		if !operatorOK {
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedOperator))
			continue
		}

		switch operator.Canonical {
		case "and":
			values := append([]string{}, cleaned...)
			column := columnName
			andConditions = append(andConditions, func(q *bun.SelectQuery) *bun.SelectQuery {
				eqOperator := resolveSQLOperator("eq", cfg)
				for _, value := range values {
					q = q.Where(fmt.Sprintf("%s %s ?", column, eqOperator), value)
				}
				return q
			})
		case "or":
			values := append([]string{}, cleaned...)
			column := columnName
			orGroups = append(orGroups, func(q *bun.SelectQuery) *bun.SelectQuery {
				orComparisonOp := resolveSQLOperator("eq", cfg)
				if orComparisonOp == "" {
					orComparisonOp = "="
				}
				for i, value := range values {
					if i == 0 {
						q = q.Where(fmt.Sprintf("%s %s ?", column, orComparisonOp), value)
					} else {
						q = q.WhereOr(fmt.Sprintf("%s %s ?", column, orComparisonOp), value)
					}
				}
				return q
			})
		default:
			values := append([]string{}, cleaned...)
			column := columnName
			sqlOp := operator.SQL
			canonical := operator.Canonical
			andConditions = append(andConditions, func(q *bun.SelectQuery) *bun.SelectQuery {
				if canonical == "in" {
					q = q.Where(fmt.Sprintf("%s IN (?)", column), bun.In(values))
					return q
				}
				for _, value := range values {
					q = q.Where(fmt.Sprintf("%s %s ?", column, sqlOp), value)
				}
				return q
			})
		}
	}

	if len(andConditions) == 0 && len(orGroups) == 0 {
		return nil, unsupported, nil
	}

	criteria := []Criteria{func(q *bun.SelectQuery) *bun.SelectQuery {
		for _, fn := range andConditions {
			q = fn(q)
		}

		if len(orGroups) > 0 {
			q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				for i, group := range orGroups {
					if i == 0 {
						q = group(q)
					} else {
						q = q.WhereGroup(" OR ", group)
					}
				}
				return q
			})
		}

		return q
	}}

	return criteria, unsupported, nil
}

func resolveFilterOperator(token, field string, cfg Config) (Operator, bool, error) {
	if operatorTokenSupported(token, cfg.OperatorMap) || cfg.FallbackUnsupportedOperators {
		operator, err := ResolveOperator(token, field, cfg)
		return operator, true, err
	}
	if cfg.StrictValidation {
		_, err := ResolveOperator(token, field, cfg)
		return Operator{}, false, err
	}
	return Operator{}, false, nil
}

func operatorTokenSupported(token string, operators map[string]string) bool {
	normalized := normalizeOperatorToken(token)
	if normalized == "" {
		normalized = "eq"
	}
	if _, ok := canonicalOperatorSQL[normalized]; ok {
		return true
	}
	effective := effectiveOperatorMap(operators)
	mapped, ok := effective[normalized]
	if !ok || strings.TrimSpace(mapped) == "" {
		return false
	}
	return canonicalOperatorForSQL(mapped) != ""
}

func resolveSQLOperator(op string, cfg Config) string {
	operator, err := ResolveOperator(op, "", cfg)
	if err != nil {
		return op
	}
	return operator.SQL
}

func unsupportedFromPredicate(predicate Predicate, reason UnsupportedReason) UnsupportedPredicate {
	operator := strings.ToLower(strings.TrimSpace(predicate.Operator))
	if operator == "" {
		operator = "eq"
	}
	return UnsupportedPredicate{
		Field:    strings.TrimSpace(predicate.Field),
		Operator: operator,
		RawKey:   predicate.RawKey,
		RawValue: predicate.RawValue,
		Reason:   reason,
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	return out
}
