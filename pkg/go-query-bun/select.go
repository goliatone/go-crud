package querybun

import (
	"strings"

	"github.com/uptrace/bun"
)

// BuildSelectCriteria returns projection criteria and trusted selected columns.
func BuildSelectCriteria(fields []string, cfg Config) ([]Criteria, []string) {
	if len(fields) == 0 {
		return nil, nil
	}

	allowedFields := cloneStringMap(cfg.AllowedFields)
	columns := make([]string, 0, len(fields))
	for _, raw := range fields {
		for field := range strings.SplitSeq(raw, ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			columnName, ok := allowedFields[field]
			if !ok || strings.TrimSpace(columnName) == "" {
				continue
			}
			columns = append(columns, columnName)
		}
	}
	if len(columns) == 0 {
		return nil, nil
	}

	return []Criteria{func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Column(columns...)
	}}, columns
}
