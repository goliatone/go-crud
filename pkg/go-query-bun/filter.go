package querybun

import (
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

// BuildFilterCriteriaFromPredicates builds Bun WHERE criteria from normalized
// predicates and a trusted allowed field map.
func BuildFilterCriteriaFromPredicates(predicates []Predicate, cfg Config) ([]Criteria, []UnsupportedPredicate, error) {
	cfg = normalizeConfig(cfg)
	if len(predicates) == 0 {
		return nil, nil, nil
	}

	unsupported := make([]UnsupportedPredicate, 0)
	conditions := make([]Criteria, 0)
	orConditions := make([]Criteria, 0)

	for _, predicate := range predicates {
		field := strings.TrimSpace(predicate.Field)
		if field == "" {
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedUnknownField))
			continue
		}

		cleaned := normalizeStringValues(predicate.Values)
		if len(cleaned) == 0 {
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedEmptyValue))
			continue
		}

		operator, err := ResolveOperator(string(predicate.Operator), field, cfg)
		if err != nil {
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedOperator))
			if isStrict(cfg) {
				return nil, unsupported, err
			}
			continue
		}

		resolvedField, fieldOK := resolveField(cfg, field, FieldUseFilter)
		fieldBuilder := cfg.Predicates.Fields[field]
		if fieldBuilder != nil {
			result, err := fieldBuilder.BuildPredicate(PredicateBuilderInput{
				Predicate:     predicate,
				Operator:      operator,
				Field:         resolvedField,
				FieldResolved: fieldOK,
				Values:        append([]string{}, cleaned...),
				Config:        cfg,
				Helpers:       PredicateBuilderHelpers{},
			})
			if err != nil {
				entry := unsupportedFromPredicate(predicate, UnsupportedUnhandledPredicate)
				unsupported = append(unsupported, entry)
				if isStrict(cfg) {
					return nil, unsupported, err
				}
				continue
			}
			if result.Handled {
				conditions = append(conditions, result.Criteria...)
				continue
			}
			if !fieldOK {
				unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedUnhandledPredicate))
				continue
			}
		}

		if !fieldOK {
			reason := UnsupportedDisallowedField
			if cfg.Fields.Resolver == nil {
				reason = UnsupportedUnknownField
			}
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, reason))
			if cfg.Validation.StrictFields {
				return nil, unsupported, &ValidationError{
					Code:  ValidationFieldNotAllowed,
					Field: field,
				}
			}
			continue
		}

		if operatorBuilder := cfg.Predicates.Operators[operator.Canonical]; operatorBuilder != nil {
			result, err := operatorBuilder.BuildPredicate(PredicateBuilderInput{
				Predicate:     predicate,
				Operator:      operator,
				Field:         resolvedField,
				FieldResolved: true,
				Values:        append([]string{}, cleaned...),
				Config:        cfg,
				Helpers:       PredicateBuilderHelpers{},
			})
			if err != nil {
				unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedUnhandledPredicate))
				if isStrict(cfg) {
					return nil, unsupported, err
				}
				continue
			}
			if result.Handled {
				conditions = append(conditions, result.Criteria...)
				continue
			}
		}

		criterion, ok := buildBuiltinPredicateCriteria(resolvedField.SQLName(), operator, cleaned, cfg)
		if !ok {
			unsupported = append(unsupported, unsupportedFromPredicate(predicate, UnsupportedOperator))
			continue
		}
		if operator.Canonical == OperatorOr {
			orConditions = append(orConditions, criterion)
		} else {
			conditions = append(conditions, criterion)
		}
	}

	if len(conditions) == 0 && len(orConditions) == 0 {
		return nil, unsupported, nil
	}

	criteria := []Criteria{func(q *bun.SelectQuery) *bun.SelectQuery {
		for _, fn := range conditions {
			q = fn(q)
		}
		if len(orConditions) > 0 {
			q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				for i, group := range orConditions {
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

func buildBuiltinPredicateCriteria(column string, operator Operator, values []string, cfg Config) (Criteria, bool) {
	if column == "" || len(values) == 0 {
		return nil, false
	}
	switch operator.Canonical {
	case OperatorIn:
		copied := append([]string{}, values...)
		return func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where(fmt.Sprintf("%s IN (?)", column), bun.In(copied))
		}, true
	case OperatorNin:
		copied := append([]string{}, values...)
		return func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where(fmt.Sprintf("%s NOT IN (?)", column), bun.In(copied))
		}, true
	case OperatorAnd:
		copied := append([]string{}, values...)
		return func(q *bun.SelectQuery) *bun.SelectQuery {
			eq, _ := cfg.Operators.Resolve(string(OperatorEq))
			for _, value := range copied {
				q = q.Where(fmt.Sprintf("%s %s ?", column, eq.SQL), value)
			}
			return q
		}, true
	case OperatorOr:
		copied := append([]string{}, values...)
		return groupedComparisonCriteria(column, Operator{Canonical: OperatorEq, SQL: "=", Spec: OperatorSpec{Kind: OperatorEq, SQL: "=", MultiValue: MultiValueAny}}, copied, cfg, MultiValueAny), true
	case OperatorLike, OperatorILike, OperatorContains:
		return groupedComparisonCriteria(column, operator, append([]string{}, values...), cfg, operator.Spec.MultiValue), true
	case OperatorEq, OperatorNe, OperatorGt, OperatorLt, OperatorGte, OperatorLte:
		return groupedComparisonCriteria(column, operator, append([]string{}, values...), cfg, MultiValueAll), true
	default:
		return nil, false
	}
}

func groupedComparisonCriteria(column string, operator Operator, values []string, cfg Config, mode MultiValueMode) Criteria {
	if mode == "" {
		mode = MultiValueAll
	}
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		if mode == MultiValueAny && len(values) > 1 {
			return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				for i, value := range values {
					if i == 0 {
						q = applyComparison(q, column, operator, value, cfg, false)
					} else {
						q = applyComparison(q, column, operator, value, cfg, true)
					}
				}
				return q
			})
		}
		for _, value := range values {
			q = applyComparison(q, column, operator, value, cfg, false)
		}
		return q
	}
}

func applyComparison(q *bun.SelectQuery, column string, operator Operator, value string, cfg Config, or bool) *bun.SelectQuery {
	_ = cfg
	sqlOp := operator.SQL
	if sqlOp == "" {
		sqlOp = "="
	}
	pattern, escapeClause := patternValue(value, operator.Spec.Pattern)
	condition := fmt.Sprintf("%s %s ?%s", column, sqlOp, escapeClause)
	if operator.Canonical == OperatorILike || operator.Spec.Case == CaseInsensitive {
		if supportsILike(q) {
			if operator.Canonical == OperatorContains {
				condition = fmt.Sprintf("%s ILIKE ?%s", column, escapeClause)
			}
		} else {
			sqlOp = "LIKE"
			condition = fmt.Sprintf("LOWER(%s) %s LOWER(?)%s", column, sqlOp, escapeClause)
		}
	}
	if or {
		return q.WhereOr(condition, pattern)
	}
	return q.Where(condition, pattern)
}

func patternValue(value string, cfg PatternConfig) (string, string) {
	escapeChar := cfg.EscapeChar
	if escapeChar == "" {
		escapeChar = `\`
	}
	switch cfg.Mode {
	case PatternContainsEscaped:
		return "%" + escapeLikeLiteral(value, escapeChar) + "%", fmt.Sprintf(" ESCAPE '%s'", escapeSQLLiteral(escapeChar))
	case PatternContains:
		return "%" + value + "%", ""
	default:
		return value, ""
	}
}

func escapeLikeLiteral(value, escapeChar string) string {
	if escapeChar == "" {
		escapeChar = `\`
	}
	value = strings.ReplaceAll(value, escapeChar, escapeChar+escapeChar)
	value = strings.ReplaceAll(value, "%", escapeChar+"%")
	value = strings.ReplaceAll(value, "_", escapeChar+"_")
	return value
}

func escapeSQLLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

// WhereExpr builds a raw trusted criteria expression for custom builders.
func (PredicateBuilderHelpers) WhereExpr(expr string, args ...any) Criteria {
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where(expr, args...)
	}
}

// Compare builds comparison criteria for custom builders.
func (PredicateBuilderHelpers) Compare(column, sqlOp string, values []string) Criteria {
	values = append([]string{}, values...)
	sqlOp = strings.TrimSpace(sqlOp)
	if sqlOp == "" {
		sqlOp = "="
	}
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		for _, value := range values {
			q = q.Where(fmt.Sprintf("%s %s ?", column, sqlOp), value)
		}
		return q
	}
}

func unsupportedFromPredicate(predicate Predicate, reason UnsupportedReason) UnsupportedPredicate {
	operator := OperatorKind(normalizeOperatorToken(string(predicate.Operator)))
	if operator == "" {
		operator = OperatorEq
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
