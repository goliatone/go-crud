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
		criteria, asOr, nextUnsupported, err := buildFilterCriteriaForPredicate(predicate, cfg)
		unsupported = append(unsupported, nextUnsupported...)
		if err != nil {
			return nil, unsupported, err
		}
		if asOr {
			orConditions = append(orConditions, criteria...)
		} else {
			conditions = append(conditions, criteria...)
		}
	}

	return combineFilterCriteria(conditions, orConditions, unsupported)
}

func buildFilterCriteriaForPredicate(predicate Predicate, cfg Config) ([]Criteria, bool, []UnsupportedPredicate, error) {
	field := strings.TrimSpace(predicate.Field)
	if field == "" {
		return nil, false, []UnsupportedPredicate{unsupportedFromPredicate(predicate, UnsupportedUnknownField)}, nil
	}

	cleaned := normalizeStringValues(predicate.Values)
	if len(cleaned) == 0 {
		return nil, false, []UnsupportedPredicate{unsupportedFromPredicate(predicate, UnsupportedEmptyValue)}, nil
	}

	operator, err := ResolveOperator(string(predicate.Operator), field, cfg)
	if err != nil {
		unsupported := []UnsupportedPredicate{unsupportedFromPredicate(predicate, UnsupportedOperator)}
		if isStrict(cfg) {
			return nil, false, unsupported, err
		}
		return nil, false, unsupported, nil
	}

	resolvedField, fieldOK := resolveField(cfg, field, FieldUseFilter)
	criteria, handled, unsupported, err := buildWithFieldPredicateBuilder(predicate, operator, resolvedField, fieldOK, cleaned, cfg)
	if handled || err != nil || len(unsupported) > 0 {
		return criteria, false, unsupported, err
	}

	if !fieldOK {
		fieldUnsupported, fieldErr := unresolvedFieldUnsupported(predicate, field, cfg)
		return nil, false, fieldUnsupported, fieldErr
	}

	criteria, handled, unsupported, err = buildWithOperatorPredicateBuilder(predicate, operator, resolvedField, cleaned, cfg)
	if handled || err != nil || len(unsupported) > 0 {
		return criteria, false, unsupported, err
	}

	criterion, ok := buildBuiltinPredicateCriteria(resolvedField.SQLName(), operator, cleaned, cfg)
	if !ok {
		return nil, false, []UnsupportedPredicate{unsupportedFromPredicate(predicate, UnsupportedOperator)}, nil
	}
	return []Criteria{criterion}, operator.Canonical == OperatorOr, nil, nil
}

func buildWithFieldPredicateBuilder(predicate Predicate, operator Operator, field ResolvedField, fieldOK bool, values []string, cfg Config) ([]Criteria, bool, []UnsupportedPredicate, error) {
	builder := cfg.Predicates.Fields[strings.TrimSpace(predicate.Field)]
	if builder == nil {
		return nil, false, nil, nil
	}
	criteria, handled, unsupported, err := runPredicateBuilder(builder, PredicateBuilderInput{
		Predicate:     predicate,
		Operator:      operator,
		Field:         field,
		FieldResolved: fieldOK,
		Values:        append([]string{}, values...),
		Config:        cfg,
		Helpers:       PredicateBuilderHelpers{},
	})
	if handled || err != nil || len(unsupported) > 0 {
		return criteria, handled, unsupported, err
	}
	if !fieldOK {
		return nil, true, []UnsupportedPredicate{unsupportedFromPredicate(predicate, UnsupportedUnhandledPredicate)}, nil
	}
	return nil, false, nil, nil
}

func buildWithOperatorPredicateBuilder(predicate Predicate, operator Operator, field ResolvedField, values []string, cfg Config) ([]Criteria, bool, []UnsupportedPredicate, error) {
	builder := cfg.Predicates.Operators[operator.Canonical]
	if builder == nil {
		return nil, false, nil, nil
	}
	return runPredicateBuilder(builder, PredicateBuilderInput{
		Predicate:     predicate,
		Operator:      operator,
		Field:         field,
		FieldResolved: true,
		Values:        append([]string{}, values...),
		Config:        cfg,
		Helpers:       PredicateBuilderHelpers{},
	})
}

func runPredicateBuilder(builder PredicateBuilder, input PredicateBuilderInput) ([]Criteria, bool, []UnsupportedPredicate, error) {
	result, err := builder.BuildPredicate(input)
	if err != nil {
		unsupported := []UnsupportedPredicate{unsupportedFromPredicate(input.Predicate, UnsupportedUnhandledPredicate)}
		if isStrict(input.Config) {
			return nil, true, unsupported, err
		}
		return nil, true, unsupported, nil
	}
	if result.Handled {
		return result.Criteria, true, nil, nil
	}
	return nil, false, nil, nil
}

func unresolvedFieldUnsupported(predicate Predicate, field string, cfg Config) ([]UnsupportedPredicate, error) {
	reason := UnsupportedDisallowedField
	if cfg.Fields.Resolver == nil {
		reason = UnsupportedUnknownField
	}
	unsupported := []UnsupportedPredicate{unsupportedFromPredicate(predicate, reason)}
	if !cfg.Validation.StrictFields {
		return unsupported, nil
	}
	return unsupported, &ValidationError{
		Code:  ValidationFieldNotAllowed,
		Field: field,
	}
}

func combineFilterCriteria(conditions, orConditions []Criteria, unsupported []UnsupportedPredicate) ([]Criteria, []UnsupportedPredicate, error) {
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
			return q.Where(fmt.Sprintf("%s IN (?)", column), bun.List(copied))
		}, true
	case OperatorNin:
		copied := append([]string{}, values...)
		return func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where(fmt.Sprintf("%s NOT IN (?)", column), bun.List(copied))
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
