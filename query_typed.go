package crud

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	repository "github.com/goliatone/go-repository-bun"
)

// ListQueryPredicate represents an operator-aware predicate for typed list criteria.
type ListQueryPredicate struct {
	Field    string
	Operator string
	Values   []string
}

// ListQueryOptions provides a non-HTTP contract to build list criteria.
// Supported parity keys are: limit/offset, order, _search, and field__operator filters.
type ListQueryOptions struct {
	Page       int
	PerPage    int
	Limit      int
	Offset     int
	SortBy     string
	SortDesc   bool
	Order      string
	Search     string
	Filters    map[string]any
	Predicates []ListQueryPredicate
	Select     []string
	Include    []string
}

// BuildListCriteriaFromOptions builds list criteria without requiring a synthetic HTTP context.
func BuildListCriteriaFromOptions[T any](opts ListQueryOptions, qbOpts ...QueryBuilderOption) ([]repository.SelectCriteria, *Filters, error) {
	ctx := newListQueryOptionsContext(opts)
	return BuildQueryCriteria[T](ctx, OpList, qbOpts...)
}

type listQueryOptionsContext struct {
	userCtx     context.Context
	queryMap    map[string]string
	queryValues map[string][]string
}

func newListQueryOptionsContext(opts ListQueryOptions) *listQueryOptionsContext {
	queryMap := make(map[string]string)
	queryValues := make(map[string][]string)

	limit, offset := normalizeListPagination(opts)
	queryMap["limit"] = strconv.Itoa(limit)
	queryMap["offset"] = strconv.Itoa(offset)

	if order := normalizeListOrder(opts); order != "" {
		queryMap["order"] = order
	}

	if search := strings.TrimSpace(opts.Search); search != "" {
		queryMap["_search"] = search
	}

	if len(opts.Select) > 0 {
		fields := normalizeStringValues(opts.Select)
		if len(fields) > 0 {
			queryMap["select"] = strings.Join(fields, ",")
		}
	}

	if len(opts.Include) > 0 {
		includes := normalizeStringValues(opts.Include)
		if len(includes) > 0 {
			queryMap["include"] = strings.Join(includes, ",")
			queryValues["include"] = includes
		}
	}

	for _, predicate := range normalizeTypedPredicates(opts) {
		field := strings.TrimSpace(predicate.Field)
		if field == "" || len(predicate.Values) == 0 {
			continue
		}

		if field == "_search" {
			if _, exists := queryMap["_search"]; !exists {
				queryMap["_search"] = predicate.Values[0]
			}
			continue
		}

		operator := strings.ToLower(strings.TrimSpace(predicate.Operator))
		if operator == "" {
			operator = "eq"
		}
		queryMap[field+"__"+operator] = strings.Join(predicate.Values, ",")
	}

	return &listQueryOptionsContext{
		userCtx:     context.Background(),
		queryMap:    queryMap,
		queryValues: queryValues,
	}
}

func normalizeListPagination(opts ListQueryOptions) (int, int) {
	limit := DefaultLimit
	offset := DefaultOffset

	if opts.Limit > 0 || opts.Offset != 0 {
		if opts.Limit > 0 {
			limit = opts.Limit
		}
		offset = opts.Offset
		return limit, offset
	}

	if opts.Page > 0 || opts.PerPage > 0 {
		perPage := opts.PerPage
		if perPage <= 0 {
			perPage = DefaultLimit
		}
		page := opts.Page
		if page < 1 {
			page = 1
		}
		return perPage, (page - 1) * perPage
	}

	return limit, offset
}

func normalizeListOrder(opts ListQueryOptions) string {
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

func normalizeTypedPredicates(opts ListQueryOptions) []ListQueryPredicate {
	if len(opts.Predicates) > 0 {
		out := make([]ListQueryPredicate, 0, len(opts.Predicates))
		for _, predicate := range opts.Predicates {
			field := strings.TrimSpace(predicate.Field)
			if field == "" {
				continue
			}
			values := normalizeStringValues(predicate.Values)
			if len(values) == 0 {
				continue
			}
			out = append(out, ListQueryPredicate{
				Field:    field,
				Operator: strings.ToLower(strings.TrimSpace(predicate.Operator)),
				Values:   values,
			})
		}
		return out
	}

	if len(opts.Filters) == 0 {
		return nil
	}

	out := make([]ListQueryPredicate, 0, len(opts.Filters))
	for rawKey, rawValue := range opts.Filters {
		field, operator := parseListPredicateKey(rawKey)
		if field == "" {
			continue
		}
		values := normalizeAnyValues(rawValue)
		if len(values) == 0 {
			continue
		}
		out = append(out, ListQueryPredicate{
			Field:    field,
			Operator: operator,
			Values:   values,
		})
	}
	return out
}

func parseListPredicateKey(key string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(key), "__", 2)
	field := strings.TrimSpace(parts[0])
	operator := "eq"
	if len(parts) == 2 {
		if op := strings.ToLower(strings.TrimSpace(parts[1])); op != "" {
			operator = op
		}
	}
	return field, operator
}

func normalizeAnyValues(raw any) []string {
	switch typed := raw.(type) {
	case nil:
		return nil
	case string:
		return normalizeStringValues([]string{typed})
	case []string:
		return normalizeStringValues(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			out = append(out, normalizeStringValues([]string{toQueryString(value)})...)
		}
		return out
	default:
		return normalizeStringValues([]string{toQueryString(raw)})
	}
}

func normalizeStringValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func toQueryString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func (c *listQueryOptionsContext) UserContext() context.Context {
	if c == nil || c.userCtx == nil {
		return context.Background()
	}
	return c.userCtx
}

func (c *listQueryOptionsContext) Params(_ string, defaultValue ...string) string {
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (c *listQueryOptionsContext) BodyParser(_ any) error { return nil }

func (c *listQueryOptionsContext) Query(key string, defaultValue ...string) string {
	if c != nil {
		if value, ok := c.queryMap[key]; ok {
			return value
		}
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (c *listQueryOptionsContext) QueryValues(key string) []string {
	if c == nil {
		return nil
	}
	if values, ok := c.queryValues[key]; ok {
		out := make([]string, len(values))
		copy(out, values)
		return out
	}
	if value, ok := c.queryMap[key]; ok {
		return normalizeStringValues([]string{value})
	}
	return nil
}

func (c *listQueryOptionsContext) QueryInt(key string, defaultValue ...int) int {
	value := c.Query(key)
	if value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

func (c *listQueryOptionsContext) Queries() map[string]string {
	out := map[string]string{}
	if c == nil {
		return out
	}
	for key, value := range c.queryMap {
		out[key] = value
	}
	return out
}

func (c *listQueryOptionsContext) Body() []byte { return nil }

func (c *listQueryOptionsContext) Status(_ int) Response { return c }

func (c *listQueryOptionsContext) JSON(_ any, _ ...string) error { return nil }

func (c *listQueryOptionsContext) SendStatus(_ int) error { return nil }
