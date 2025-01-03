package crud

import (
	"fmt"
	"strings"

	"github.com/goliatone/go-repository-bun"
	"github.com/uptrace/bun"
)

type relationFilter struct {
	field    string
	operator string
	value    string
}

type relationInfo struct {
	name    string
	filters []relationFilter
}

type queryCriteria struct {
	op         CrudOperation
	pagination []repository.SelectCriteria
	selected   []repository.SelectCriteria
	order      []repository.SelectCriteria
	included   []repository.SelectCriteria
	filters    []repository.SelectCriteria
}

func (q *queryCriteria) compute() []repository.SelectCriteria {
	out := []repository.SelectCriteria{}

	if q.op == OpList {
		out = append(out, q.pagination...)
		out = append(out, q.order...)
		out = append(out, q.filters...)
	}

	out = append(out, q.selected...)
	out = append(out, q.included...)

	return out
}

var DefaultLimit = 25
var DefaultOffset = 0

// Index supports different query string parameters:
// GET /users?limit=10&offset=20
// GET /users?order=name asc,created_at desc
// GET /users?select=id,name,email
// GET /users?name__ilike=John&age__gte=30
// GET /users?name__and=John,Jack
// GET /users?name__or=John,Jack
// GET /users?include=Company,Profile
// GET /users?include=Profile.status=outdated
// TODO: Support /projects?include=Message&include=Company
func BuildQueryCriteria[T any](ctx Context, op CrudOperation) ([]repository.SelectCriteria, *Filters, error) {
	// Parse known query parameters
	limit := ctx.QueryInt("limit", DefaultLimit)
	offset := ctx.QueryInt("offset", DefaultOffset)
	order := ctx.Query("order")
	selectFields := ctx.Query("select")
	include := ctx.Query("include")

	filters := &Filters{
		Limit:     limit,
		Offset:    offset,
		Operation: string(op),
	}

	// Start building our criteria slice
	criteria := &queryCriteria{op: op}

	// Basic limit/offset criteria
	criteria.pagination = append(criteria.pagination, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Limit(limit).Offset(offset)
	})

	// For fields that are allowable.
	// E.g. "name" => "name", "created_at" => "created_at", etc.
	allowedFieldsMap := getAllowedFields[T]()

	// Handle SELECT fields
	if selectFields != "" {
		fields := strings.Split(selectFields, ",")
		var columns []string
		for _, field := range fields {
			columnName, ok := allowedFieldsMap[field]
			if !ok {
				//TODO: log info
				continue // skip, unknown fields!
			}
			columns = append(columns, columnName)
		}
		if len(columns) > 0 {
			criteria.selected = append(criteria.selected, func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Column(columns...)
			})
			filters.Fields = columns
		}
	}

	// Handle ORDER clauses
	if order != "" {
		orders := strings.Split(order, ",")
		for _, o := range orders {
			parts := strings.Fields(strings.TrimSpace(o))
			if len(parts) > 0 {
				field := parts[0]
				direction := "ASC" // default direction
				if len(parts) > 1 {
					direction = getDirection(parts[1])
				}

				// Check if field is allowed
				columnName, ok := allowedFieldsMap[field]
				if ok {
					filters.Order = append(filters.Order, Order{
						Field: columnName,
						Dir:   direction,
					})
				}
			}
		}

		criteria.order = append(criteria.order, func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, o := range filters.Order {
				orderClause := fmt.Sprintf("%s %s", o.Field, o.Dir)
				q = q.Order(orderClause)
			}
			return q
		})
	}

	// Handle includes
	if include != "" {
		relations := strings.Split(include, ",")
		for _, relation := range relations {
			relationInfo, err := parseRelation(relation)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid relation format: %v", err)
			}

			filters.Include = append(filters.Include, relationInfo.name)

			criteria.included = append(criteria.included, func(q *bun.SelectQuery) *bun.SelectQuery {
				if len(relationInfo.filters) == 0 {
					return q.Relation(relationInfo.name)
				}

				return q.Relation(relationInfo.name, func(q *bun.SelectQuery) *bun.SelectQuery {
					for _, filter := range relationInfo.filters {
						q = q.Where(fmt.Sprintf("%s %s ?", filter.field, filter.operator), filter.value)
					}
					return q
				})
			})
		}
	}

	// Build WHERE conditions from other query params
	excludeParams := map[string]bool{
		"limit":   true,
		"offset":  true,
		"order":   true,
		"select":  true,
		"include": true,
	}

	queryParams := ctx.Queries()

	// For each parameter, if it's not in excludeParams, add a where condition
	criteria.filters = append(criteria.filters, func(q *bun.SelectQuery) *bun.SelectQuery {
		for param, values := range queryParams {
			if excludeParams[param] {
				continue
			}
			// TODO: we could check that if we are in sqlite that we support the operator, e.g. ilike
			// parseFieldOperator might parse, e.g. "name__ilike" => ("name", "ilike")
			field, operator := parseFieldOperator(param)

			columnName, ok := allowedFieldsMap[field]
			if !ok {
				continue // skip, not allowed TODO: Log
			}

			operator = strings.ToLower(operator)
			switch operator {
			case "and", "or":
				// handle "name__and=John,Jack" => name=John AND name=Jack
				// or => name=John OR name=Jack
				whereGroup := func(q *bun.SelectQuery) *bun.SelectQuery {
					splitted := strings.Split(values, ",")
					for i, value := range splitted {
						v := strings.TrimSpace(value)
						if v == "" {
							continue
						}
						if i == 0 {
							q = q.Where(fmt.Sprintf("%s = ?", columnName), v)
						} else {
							q = q.WhereOr(fmt.Sprintf("%s = ?", columnName), v)
						}
					}
					return q
				}

				if operator == "and" {
					q = q.WhereGroup(" AND ", whereGroup)
				} else {
					q = q.WhereGroup(" OR ", whereGroup)
				}
			default:
				// Handle typical operator: eq, gt, gte, ilike, etc.
				splitted := strings.Split(values, ",")
				for _, value := range splitted {
					v := strings.TrimSpace(value)
					if v == "" {
						continue
					}
					q = q.Where(fmt.Sprintf("%s %s ?", columnName, operator), v)
				}
			}
		}
		return q
	})

	return criteria.compute(), filters, nil
}

func parseRelation(relation string) (*relationInfo, error) {
	parts := strings.Split(relation, ".")
	if len(parts) < 2 {
		return &relationInfo{name: relation}, nil
	}

	info := &relationInfo{
		name: parts[0],
	}

	filterPart := strings.Join(parts[1:], ".")

	filterParts := strings.Split(filterPart, "=")
	if len(filterParts) != 2 {
		return info, nil
	}

	field, operator := parseFieldOperator(filterParts[0])
	filter := relationFilter{
		field:    field,
		operator: operator,
		value:    filterParts[1],
	}
	info.filters = append(info.filters, filter)

	return info, nil
}

func getDirection(dir string) string {
	dir = strings.TrimSpace(strings.ToUpper(dir))
	if dir == "ASC" || dir == "DESC" {
		return dir
	}
	return "ASC"
}
