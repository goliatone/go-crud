package querybun

import (
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

// NormalizeOrder returns the effective order string for list options.
func NormalizeOrder(opts ListOptions) string {
	if order := strings.TrimSpace(opts.Order); order != "" {
		return order
	}

	field := strings.TrimSpace(opts.SortBy)
	if field == "" {
		return ""
	}

	dir := "asc"
	if opts.SortDesc {
		dir = "desc"
	}

	return field + " " + dir
}

// BuildOrderCriteria returns trusted order criteria and metadata.
func BuildOrderCriteria(order string, cfg Config) ([]Criteria, []Order) {
	if strings.TrimSpace(order) == "" {
		return nil, nil
	}

	allowedFields := cloneStringMap(cfg.AllowedFields)
	orders := make([]Order, 0)
	for raw := range strings.SplitSeq(order, ",") {
		parts := strings.Fields(strings.TrimSpace(raw))
		if len(parts) == 0 {
			continue
		}
		field := parts[0]
		direction := "ASC"
		if len(parts) > 1 {
			direction = normalizeDirection(parts[1])
		}
		columnName, ok := allowedFields[field]
		if !ok || strings.TrimSpace(columnName) == "" {
			continue
		}
		orders = append(orders, Order{Field: columnName, Dir: direction})
	}
	if len(orders) == 0 {
		return nil, nil
	}

	return []Criteria{func(q *bun.SelectQuery) *bun.SelectQuery {
		for _, order := range orders {
			q = q.OrderExpr(fmt.Sprintf("%s %s", order.Field, order.Dir))
		}
		return q
	}}, orders
}

func normalizeDirection(dir string) string {
	dir = strings.TrimSpace(strings.ToUpper(dir))
	if dir == "ASC" || dir == "DESC" {
		return dir
	}
	return "ASC"
}
