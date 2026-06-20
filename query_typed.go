package crud

import (
	"strings"

	querybun "github.com/goliatone/go-crud/pkg/go-query-bun"
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
	cfg := queryBuilderConfig{}
	for _, opt := range qbOpts {
		if opt != nil {
			opt(&cfg)
		}
	}

	plan, err := querybun.BuildQueryPlan(toQueryBunListOptions(opts), queryBunConfig[T](cfg))
	if err != nil {
		return nil, nil, convertQueryBunError(err)
	}

	filters := filtersFromQueryBunPlan(plan, OpList)
	criteria := adaptQueryBunCriteria(plan.ListCriteria())

	includeCriteria, includePaths, relations, err := buildIncludeCriteriaForType[T](strings.Join(filters.Include, ","), cfg.strictValidationEnabled())
	if err != nil {
		return nil, nil, err
	}
	if len(includePaths) > 0 {
		filters.Include = includePaths
		filters.Relations = relations
		criteria = append(criteria, includeCriteria...)
	}

	return criteria, filters, nil
}

func toQueryBunListOptions(opts ListQueryOptions) querybun.ListOptions {
	predicates := make([]querybun.Predicate, 0, len(opts.Predicates))
	for _, predicate := range opts.Predicates {
		predicates = append(predicates, querybun.Predicate{
			Field:    predicate.Field,
			Operator: predicate.Operator,
			Values:   append([]string{}, predicate.Values...),
		})
	}
	return querybun.ListOptions{
		Page:       opts.Page,
		PerPage:    opts.PerPage,
		Limit:      opts.Limit,
		Offset:     opts.Offset,
		SortBy:     opts.SortBy,
		SortDesc:   opts.SortDesc,
		Order:      opts.Order,
		Search:     opts.Search,
		Filters:    opts.Filters,
		Predicates: predicates,
		Select:     append([]string{}, opts.Select...),
		Include:    append([]string{}, opts.Include...),
	}
}
