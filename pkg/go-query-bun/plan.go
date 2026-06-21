package querybun

import "strings"

// Plan contains independently applicable criteria groups for a list query.
type Plan struct {
	Filters     []Criteria
	Search      []Criteria
	Order       []Criteria
	Pagination  []Criteria
	Select      []Criteria
	Includes    []IncludeRequest
	Metadata    Metadata
	Unsupported []UnsupportedPredicate
}

// ListCriteria returns the legacy combined criteria order used by go-crud
// list operations: pagination, order, filters/search, select.
func (p Plan) ListCriteria() []Criteria {
	out := make([]Criteria, 0, len(p.Pagination)+len(p.Order)+len(p.Filters)+len(p.Search)+len(p.Select))
	out = append(out, p.Pagination...)
	out = append(out, p.Order...)
	out = append(out, p.Filters...)
	out = append(out, p.Search...)
	out = append(out, p.Select...)
	return out
}

// WhereCriteria returns criteria that constrain rows without order, pagination, or projection.
func (p Plan) WhereCriteria() []Criteria {
	out := make([]Criteria, 0, len(p.Filters)+len(p.Search))
	out = append(out, p.Filters...)
	out = append(out, p.Search...)
	return out
}

// CountCriteria returns criteria suitable for count queries.
func (p Plan) CountCriteria() []Criteria {
	return p.WhereCriteria()
}

// PageCriteria returns pagination criteria only.
func (p Plan) PageCriteria() []Criteria {
	return append([]Criteria{}, p.Pagination...)
}

// ReadCriteria returns criteria that apply to single-record read operations.
func (p Plan) ReadCriteria() []Criteria {
	out := make([]Criteria, 0, len(p.Select))
	out = append(out, p.Select...)
	return out
}

// Metadata contains normalized query values for response adapters and callers.
type Metadata struct {
	Limit   int
	Offset  int
	Page    int
	Search  string
	Order   []Order
	Fields  []string
	Include []string
}

// BuildQueryPlan builds a separated, reusable query plan from list options.
func BuildQueryPlan(opts ListOptions, cfg Config) (Plan, error) {
	cfg = normalizeConfig(cfg)
	opts, searchUnsupported := normalizeSearchOptions(opts, cfg)

	var plan Plan
	plan.Unsupported = append(plan.Unsupported, searchUnsupported...)

	pagination, limit, offset, page := BuildPaginationCriteria(opts, cfg)
	plan.Pagination = pagination
	plan.Metadata.Limit = limit
	plan.Metadata.Offset = offset
	plan.Metadata.Page = page

	orderCriteria, orders := BuildOrderCriteria(NormalizeOrder(opts), cfg)
	plan.Order = orderCriteria
	plan.Metadata.Order = orders

	predicates, unsupported := NormalizePredicatesWithUnsupported(opts, cfg)
	plan.Unsupported = append(plan.Unsupported, unsupported...)

	filterCriteria, filterUnsupported, err := BuildFilterCriteriaFromPredicates(predicates, cfg)
	plan.Filters = filterCriteria
	plan.Unsupported = append(plan.Unsupported, filterUnsupported...)
	if err != nil {
		return plan, err
	}

	searchCriteria, search, err := BuildSearchCriteria(opts.Search, cfg)
	plan.Search = searchCriteria
	plan.Metadata.Search = search
	if err != nil {
		return plan, err
	}

	selectCriteria, selected := BuildSelectCriteria(opts.Select, cfg)
	plan.Select = selectCriteria
	plan.Metadata.Fields = selected

	plan.Includes = NormalizeIncludes(opts.Include)
	if len(plan.Includes) > 0 {
		plan.Metadata.Include = make([]string, len(plan.Includes))
		for i, include := range plan.Includes {
			plan.Metadata.Include[i] = include.Path
		}
	}

	return plan, nil
}

func normalizeSearchOptions(opts ListOptions, cfg Config) (ListOptions, []UnsupportedPredicate) {
	search := strings.TrimSpace(opts.Search)

	if len(opts.Predicates) > 0 {
		predicates := make([]Predicate, 0, len(opts.Predicates))
		unsupported := make([]UnsupportedPredicate, 0)
		for _, predicate := range opts.Predicates {
			if !cfg.Search.isSearchAlias(predicate.Field) {
				predicates = append(predicates, predicate)
				continue
			}
			values, ok := NormalizeValueStrings(predicate.Values)
			if !ok {
				unsupported = append(unsupported, unsupportedSearchPredicate(predicate.Field, predicate.Operator, predicate.RawKey, predicate.RawValue, UnsupportedValueShape))
				continue
			}
			if len(values) == 0 {
				unsupported = append(unsupported, unsupportedSearchPredicate(predicate.Field, predicate.Operator, predicate.RawKey, predicate.RawValue, UnsupportedEmptyValue))
				continue
			}
			if search == "" {
				search = values[0]
			}
		}
		if search == "" && len(opts.Filters) > 0 {
			filterSearch, filterUnsupported := searchFromFilters(opts.Filters, cfg)
			unsupported = append(unsupported, filterUnsupported...)
			search = filterSearch
		}
		opts.Predicates = predicates
		opts.Search = search
		return opts, unsupported
	}

	if len(opts.Filters) == 0 {
		opts.Search = search
		return opts, nil
	}

	filters := make(map[string]any, len(opts.Filters))
	unsupported := make([]UnsupportedPredicate, 0)
	for rawKey, rawValue := range opts.Filters {
		field, _ := cfg.Syntax.parsePredicateKey(rawKey)
		if !cfg.Search.isSearchAlias(field) {
			filters[rawKey] = rawValue
		}
	}
	if search == "" {
		filterSearch, filterUnsupported := searchFromFilters(opts.Filters, cfg)
		unsupported = append(unsupported, filterUnsupported...)
		search = filterSearch
	}

	opts.Filters = filters
	opts.Search = search
	return opts, unsupported
}

func searchFromFilters(filters map[string]any, cfg Config) (string, []UnsupportedPredicate) {
	unsupported := make([]UnsupportedPredicate, 0)
	for _, alias := range cfg.Search.Aliases {
		for rawKey, rawValue := range filters {
			field, operator := cfg.Syntax.parsePredicateKey(rawKey)
			if !strings.EqualFold(strings.TrimSpace(field), strings.TrimSpace(alias)) {
				continue
			}
			values, ok := NormalizeValueStrings(rawValue)
			if !ok {
				unsupported = append(unsupported, unsupportedSearchPredicate(field, operator, rawKey, rawValue, UnsupportedValueShape))
				continue
			}
			if len(values) == 0 {
				unsupported = append(unsupported, unsupportedSearchPredicate(field, operator, rawKey, rawValue, UnsupportedEmptyValue))
				continue
			}
			return values[0], unsupported
		}
	}
	return "", unsupported
}

func unsupportedSearchPredicate(field string, operator OperatorKind, rawKey string, rawValue any, reason UnsupportedReason) UnsupportedPredicate {
	normalizedOperator := OperatorKind(normalizeOperatorToken(string(operator)))
	if normalizedOperator == "" {
		normalizedOperator = OperatorEq
	}
	return UnsupportedPredicate{
		Field:    strings.TrimSpace(field),
		Operator: normalizedOperator,
		RawKey:   rawKey,
		RawValue: rawValue,
		Reason:   reason,
	}
}

// NormalizeIncludes returns trimmed, comma-split include requests.
func NormalizeIncludes(includes []string) []IncludeRequest {
	if len(includes) == 0 {
		return nil
	}
	out := make([]IncludeRequest, 0, len(includes))
	for _, raw := range includes {
		for part := range strings.SplitSeq(raw, ",") {
			path := strings.TrimSpace(part)
			if path == "" {
				continue
			}
			out = append(out, IncludeRequest{Path: path})
		}
	}
	return out
}
