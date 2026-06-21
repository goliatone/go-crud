package querybun

import (
	"strings"

	"github.com/uptrace/bun"
)

// BuildSelectCriteria returns projection criteria and trusted selected columns.
func BuildSelectCriteria(fields []string, cfg Config) ([]Criteria, []string) {
	cfg = normalizeConfig(cfg)
	if len(fields) == 0 {
		return nil, nil
	}

	columns := make([]string, 0, len(fields))
	for _, raw := range fields {
		for field := range strings.SplitSeq(raw, ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			resolved, ok := resolveField(cfg, field, FieldUseSelect)
			if !ok || strings.TrimSpace(resolved.SQLName()) == "" {
				continue
			}
			columns = append(columns, resolved.SQLName())
		}
	}
	if len(columns) == 0 {
		return nil, nil
	}

	return []Criteria{func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Column(columns...)
	}}, columns
}
