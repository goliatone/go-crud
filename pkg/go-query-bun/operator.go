package querybun

import (
	"maps"
	"strings"
	"sync"
)

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

var defaultOperatorMap = struct {
	sync.RWMutex
	values map[string]string
}{
	values: cloneOperatorMap(canonicalOperatorSQL),
}

// CanonicalOperatorMap returns a copy of the canonical operator-to-SQL mapping.
func CanonicalOperatorMap() map[string]string {
	return cloneOperatorMap(canonicalOperatorSQL)
}

// DefaultOperatorMap returns a copy of the package default operator map.
func DefaultOperatorMap() map[string]string {
	defaultOperatorMap.RLock()
	defer defaultOperatorMap.RUnlock()
	return cloneOperatorMap(defaultOperatorMap.values)
}

// SetDefaultOperatorMap replaces package default operator aliases.
func SetDefaultOperatorMap(operators map[string]string) {
	defaultOperatorMap.Lock()
	defer defaultOperatorMap.Unlock()
	defaultOperatorMap.values = cloneOperatorMap(operators)
}

// ResolveOperator resolves token into a canonical operator and SQL operator.
func ResolveOperator(token, field string, cfg Config) (Operator, error) {
	rawToken := strings.TrimSpace(token)
	normalized := normalizeOperatorToken(rawToken)
	if normalized == "" {
		normalized = "eq"
	}

	operators := effectiveOperatorMap(cfg.OperatorMap)

	// Canonical operators always work, even if aliases replace the map.
	if canonicalSQL, ok := canonicalOperatorSQL[normalized]; ok {
		sqlOp := canonicalSQL
		if mapped, ok := operators[normalized]; ok {
			if trimmed := strings.TrimSpace(mapped); trimmed != "" {
				sqlOp = trimmed
			}
		}
		return Operator{
			Token:     normalized,
			Canonical: normalized,
			SQL:       sqlOp,
		}, nil
	}

	if mapped, ok := operators[normalized]; ok {
		if trimmed := strings.TrimSpace(mapped); trimmed != "" {
			canonical := canonicalOperatorForSQL(trimmed)
			if canonical != "" {
				return Operator{
					Token:     normalized,
					Canonical: canonical,
					SQL:       trimmed,
				}, nil
			}
		}
	}

	if cfg.StrictValidation {
		return Operator{}, &ValidationError{
			Code:     ValidationUnsupportedOperator,
			Field:    strings.TrimSpace(field),
			Operator: rawToken,
		}
	}

	eqSQL := canonicalOperatorSQL["eq"]
	if mapped, ok := operators["eq"]; ok {
		if trimmed := strings.TrimSpace(mapped); trimmed != "" {
			eqSQL = trimmed
		}
	}

	return Operator{
		Token:     "eq",
		Canonical: "eq",
		SQL:       eqSQL,
	}, nil
}

func effectiveOperatorMap(operators map[string]string) map[string]string {
	if operators != nil {
		return cloneOperatorMap(operators)
	}
	return DefaultOperatorMap()
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

func cloneOperatorMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
