package querybun

import (
	"strings"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

// BuildSearchCriteria returns dialect-aware search criteria for _search.
func BuildSearchCriteria(search string, cfg Config) ([]Criteria, string, error) {
	cfg = normalizeConfig(cfg)
	searchTerm := strings.TrimSpace(search)
	if searchTerm == "" {
		return nil, "", nil
	}

	searchColumns := ResolveSearchColumns(cfg.Search.Fields, cfg)
	if len(searchColumns) == 0 {
		if isStrict(cfg) && cfg.Validation.StrictSearchFields {
			return nil, searchTerm, &ValidationError{
				Code:   ValidationSearchColumnsRequired,
				Search: searchTerm,
			}
		}
		return nil, searchTerm, nil
	}

	operator := Operator{
		Canonical: OperatorContains,
		SQL:       "LIKE",
		Spec: OperatorSpec{
			Kind:    OperatorContains,
			SQL:     "LIKE",
			Pattern: cfg.Search.Pattern,
			Case:    cfg.Search.Case,
		},
	}
	return []Criteria{func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for i, column := range searchColumns {
				if i == 0 {
					q = applyComparison(q, column, operator, searchTerm, cfg, false)
				} else {
					q = applyComparison(q, column, operator, searchTerm, cfg, true)
				}
			}
			return q
		})
	}}, searchTerm, nil
}

// ResolveSearchColumns maps configured search fields or trusted columns to SQL columns.
func ResolveSearchColumns(configured []string, cfg Config) []string {
	cfg = normalizeConfig(cfg)
	if len(configured) == 0 {
		return nil
	}

	dedup := make(map[string]struct{}, len(configured))
	out := make([]string, 0, len(configured))

	for _, raw := range configured {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}

		resolvedField, ok := resolveField(cfg, candidate, FieldUseSearch)
		if !ok {
			continue
		}
		resolved := strings.TrimSpace(resolvedField.SQLName())
		if resolved == "" {
			continue
		}
		key := strings.ToLower(resolved)
		if _, exists := dedup[key]; exists {
			continue
		}
		dedup[key] = struct{}{}
		out = append(out, resolved)
	}

	return out
}

func supportsILike(q *bun.SelectQuery) bool {
	if q == nil || q.Dialect() == nil {
		return false
	}
	return q.Dialect().Name() == dialect.PG
}
