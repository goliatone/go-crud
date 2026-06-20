package querybun

import (
	"fmt"
	"strings"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

// BuildSearchCriteria returns dialect-aware search criteria for _search.
func BuildSearchCriteria(search string, cfg Config) ([]Criteria, string, error) {
	searchTerm := strings.TrimSpace(search)
	if searchTerm == "" {
		return nil, "", nil
	}

	searchColumns := ResolveSearchColumns(cfg.SearchColumns, cfg.AllowedFields)
	if len(searchColumns) == 0 {
		if cfg.StrictValidation && cfg.StrictSearchColumns {
			return nil, searchTerm, &ValidationError{
				Code:   ValidationSearchColumnsRequired,
				Search: searchTerm,
			}
		}
		return nil, searchTerm, nil
	}

	pattern := "%" + searchTerm + "%"
	return []Criteria{func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for i, column := range searchColumns {
				if i == 0 {
					q = applySearchWhere(q, column, pattern, cfg)
				} else {
					q = applySearchWhereOr(q, column, pattern, cfg)
				}
			}
			return q
		})
	}}, searchTerm, nil
}

// ResolveSearchColumns maps configured search fields or trusted columns to SQL columns.
func ResolveSearchColumns(configured []string, allowedFields map[string]string) []string {
	if len(configured) == 0 {
		return nil
	}

	allowed := cloneStringMap(allowedFields)
	allowedColumns := make(map[string]struct{}, len(allowed))
	for _, column := range allowed {
		if trimmed := strings.TrimSpace(column); trimmed != "" {
			allowedColumns[strings.ToLower(trimmed)] = struct{}{}
		}
	}

	dedup := make(map[string]struct{}, len(configured))
	out := make([]string, 0, len(configured))

	for _, raw := range configured {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}

		resolved := ""
		if mapped, ok := allowed[candidate]; ok {
			resolved = strings.TrimSpace(mapped)
		} else if _, ok := allowedColumns[strings.ToLower(candidate)]; ok {
			resolved = candidate
		}

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

func applySearchWhere(q *bun.SelectQuery, column, pattern string, cfg Config) *bun.SelectQuery {
	if supportsILike(q) {
		op := resolveSQLOperator("ilike", cfg)
		return q.Where(fmt.Sprintf("%s %s ?", column, op), pattern)
	}
	return q.Where(fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", column), pattern)
}

func applySearchWhereOr(q *bun.SelectQuery, column, pattern string, cfg Config) *bun.SelectQuery {
	if supportsILike(q) {
		op := resolveSQLOperator("ilike", cfg)
		return q.WhereOr(fmt.Sprintf("%s %s ?", column, op), pattern)
	}
	return q.WhereOr(fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", column), pattern)
}

func supportsILike(q *bun.SelectQuery) bool {
	if q == nil || q.Dialect() == nil {
		return false
	}
	return q.Dialect().Name() == dialect.PG
}
