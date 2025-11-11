package crud

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
	"github.com/uptrace/bun"
)

type relationFilter struct {
	field    string
	operator string
	value    string
	column   string
}

type relationIncludeNode struct {
	name        string
	requestName string
	filters     []relationFilter
	children    map[string]*relationIncludeNode
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

type QueryBuilderOption func(*queryBuilderConfig)

type queryBuilderConfig struct {
	trace         *queryTraceOptions
	allowedFields map[string]string
}

func WithAllowedFields(fields map[string]string) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		if len(fields) == 0 {
			return
		}
		cfg.allowedFields = fields
	}
}

func BuildQueryCriteria[T any](ctx Context, op CrudOperation, opts ...QueryBuilderOption) ([]repository.SelectCriteria, *Filters, error) {
	cfg := queryBuilderConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return buildQueryCriteria[T](ctx, op, cfg)
}

func BuildQueryCriteriaWithLogger[T any](ctx Context, op CrudOperation, logger Logger, enableTrace bool, opts ...QueryBuilderOption) ([]repository.SelectCriteria, *Filters, error) {
	cfg := queryBuilderConfig{}
	if logger != nil {
		cfg.trace = &queryTraceOptions{
			logger:  logger,
			enabled: enableTrace,
		}
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return buildQueryCriteria[T](ctx, op, cfg)
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
func buildQueryCriteria[T any](ctx Context, op CrudOperation, cfg queryBuilderConfig) ([]repository.SelectCriteria, *Filters, error) {
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

	criteria := &queryCriteria{op: op}

	// Basic limit/offset criteria
	criteria.pagination = append(criteria.pagination, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Limit(limit).Offset(offset)
	})

	// For fields that are allowable.
	// E.g. "name" => "name", "created_at" => "created_at", etc.
	allowedFieldsMap := cfg.allowedFields
	if len(allowedFieldsMap) == 0 {
		allowedFieldsMap = getAllowedFields[T]()
	}

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
		meta := getRelationMetadataForType(typeOf[T]())
		includeNodes, err := buildIncludeTree(include, meta)
		if err != nil {
			return nil, nil, err
		}

		if len(includeNodes) > 0 {
			includePaths, relationInfos := collectIncludeDetails(includeNodes)
			filters.Include = append(filters.Include, includePaths...)
			descriptor := getRelationDescriptorForType(typeOf[T]())
			filters.Relations = mergeRelationInfos(includePaths, relationInfos, descriptor)

			rootKeys := sortedRelationKeys(includeNodes)
			for _, key := range rootKeys {
				node := includeNodes[key]
				if node == nil {
					continue
				}
				criteria.included = append(criteria.included, func(n *relationIncludeNode) repository.SelectCriteria {
					return func(q *bun.SelectQuery) *bun.SelectQuery {
						return includeRelation(q, n)
					}
				}(node))
			}
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

	andConditions := []func(*bun.SelectQuery) *bun.SelectQuery{}
	orGroups := []func(*bun.SelectQuery) *bun.SelectQuery{}

	// For each parameter, if it's not in excludeParams, record a where condition
	for param, values := range queryParams {
		if excludeParams[param] {
			continue
		}

		field, operator := parseFieldOperator(param)
		columnName, ok := allowedFieldsMap[field]
		if !ok {
			continue
		}

		operator = strings.ToLower(operator)
		switch operator {
		case "and":
			valuesList := strings.Split(values, ",")
			cleaned := make([]string, 0, len(valuesList))
			for _, value := range valuesList {
				if v := strings.TrimSpace(value); v != "" {
					cleaned = append(cleaned, v)
				}
			}
			if len(cleaned) == 0 {
				continue
			}

			andConditions = append(andConditions, func(q *bun.SelectQuery) *bun.SelectQuery {
				for _, v := range cleaned {
					q = q.Where(fmt.Sprintf("%s = ?", columnName), v)
				}
				return q
			})
		case "or":
			valuesList := strings.Split(values, ",")
			cleaned := make([]string, 0, len(valuesList))
			for _, value := range valuesList {
				if v := strings.TrimSpace(value); v != "" {
					cleaned = append(cleaned, v)
				}
			}
			if len(cleaned) == 0 {
				continue
			}

			orGroups = append(orGroups, func(q *bun.SelectQuery) *bun.SelectQuery {
				for i, v := range cleaned {
					if i == 0 {
						q = q.Where(fmt.Sprintf("%s = ?", columnName), v)
					} else {
						q = q.WhereOr(fmt.Sprintf("%s = ?", columnName), v)
					}
				}
				return q
			})
		default:
			splitted := strings.Split(values, ",")
			cleaned := make([]string, 0, len(splitted))
			for _, value := range splitted {
				if v := strings.TrimSpace(value); v != "" {
					cleaned = append(cleaned, v)
				}
			}
			if len(cleaned) == 0 {
				continue
			}

			andConditions = append(andConditions, func(q *bun.SelectQuery) *bun.SelectQuery {
				for _, v := range cleaned {
					q = q.Where(fmt.Sprintf("%s %s ?", columnName, operator), v)
				}
				return q
			})
		}
	}

	criteria.filters = append(criteria.filters, func(q *bun.SelectQuery) *bun.SelectQuery {
		for _, fn := range andConditions {
			q = fn(q)
		}

		if len(orGroups) > 0 {
			q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				for i, grp := range orGroups {
					if i == 0 {
						q = grp(q)
					} else {
						q = q.WhereGroup(" OR ", grp)
					}
				}
				return q
			})
		}

		return q
	})

	if cfg.trace != nil {
		cfg.trace.debug(filters, queryParams)
	}

	return criteria.compute(), filters, nil
}

func getDirection(dir string) string {
	dir = strings.TrimSpace(strings.ToUpper(dir))
	if dir == "ASC" || dir == "DESC" {
		return dir
	}
	return "ASC"
}

func buildIncludeTree(includeParam string, meta *relationMetadata) (map[string]*relationIncludeNode, error) {
	result := make(map[string]*relationIncludeNode)
	if strings.TrimSpace(includeParam) == "" {
		return result, nil
	}

	paths := strings.Split(includeParam, ",")
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}

		node, err := parseIncludePath(path, meta)
		if err != nil {
			return nil, fmt.Errorf("invalid relation include %q: %w", path, err)
		}
		mergeIncludeTrees(result, node)
	}

	return result, nil
}

func parseIncludePath(path string, meta *relationMetadata) (*relationIncludeNode, error) {
	if meta == nil {
		return nil, fmt.Errorf("relation metadata unavailable")
	}

	segments := strings.Split(path, ".")
	currentMeta := meta

	var root *relationIncludeNode
	var current *relationIncludeNode

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil, fmt.Errorf("empty segment in include")
		}

		if strings.Contains(segment, "=") {
			if current == nil {
				return nil, fmt.Errorf("filter specified before relation")
			}
			fieldPart, value, found := strings.Cut(segment, "=")
			if !found {
				return nil, fmt.Errorf("invalid filter syntax")
			}
			fieldName, operator := parseFieldOperator(fieldPart)
			columnName := ""
			if currentMeta != nil && currentMeta.fields != nil {
				columnName = currentMeta.fields[fieldName]
			}
			if columnName == "" {
				return nil, fmt.Errorf("unsupported filter field %q on relation %q", fieldName, current.requestName)
			}
			current.filters = append(current.filters, relationFilter{
				field:    fieldName,
				operator: operator,
				value:    value,
				column:   columnName,
			})
			continue
		}

		if currentMeta == nil {
			return nil, fmt.Errorf("relation metadata unavailable for %q", segment)
		}

		childMeta, ok := currentMeta.children[strings.ToLower(segment)]
		if !ok {
			return nil, fmt.Errorf("unknown relation %q", segment)
		}

		node := &relationIncludeNode{
			name:        childMeta.relationName,
			requestName: segment,
			filters:     make([]relationFilter, 0),
			children:    make(map[string]*relationIncludeNode),
		}

		if current == nil {
			root = node
		} else {
			current.children[node.name] = node
		}

		current = node
		currentMeta = childMeta
	}

	if root == nil {
		return nil, fmt.Errorf("invalid include path %q", path)
	}

	return root, nil
}

func mergeIncludeTrees(dst map[string]*relationIncludeNode, node *relationIncludeNode) {
	if node == nil {
		return
	}

	if existing, ok := dst[node.name]; ok {
		mergeIncludeNode(existing, node)
		return
	}

	dst[node.name] = node
}

func mergeIncludeNode(into, other *relationIncludeNode) {
	if into == nil || other == nil {
		return
	}

	into.filters = append(into.filters, other.filters...)

	if into.children == nil && len(other.children) > 0 {
		into.children = make(map[string]*relationIncludeNode, len(other.children))
	}

	for name, child := range other.children {
		if existing, ok := into.children[name]; ok {
			mergeIncludeNode(existing, child)
		} else {
			into.children[name] = child
		}
	}
}

func collectIncludeDetails(nodes map[string]*relationIncludeNode) ([]string, []RelationInfo) {
	var includes []string
	var relations []RelationInfo

	keys := sortedRelationKeys(nodes)
	for _, key := range keys {
		node := nodes[key]
		if node == nil {
			continue
		}
		collectRelationDetails(node, nil, &includes, &relations)
	}

	return includes, relations
}

func collectRelationDetails(node *relationIncludeNode, path []string, includes *[]string, relations *[]RelationInfo) {
	if node == nil {
		return
	}

	currentPath := append(path, node.requestName)
	pathStr := strings.Join(currentPath, ".")
	*includes = append(*includes, pathStr)

	if len(node.filters) > 0 {
		relationFilters := make([]RelationFilter, len(node.filters))
		for i, filter := range node.filters {
			relationFilters[i] = RelationFilter{
				Field:    filter.field,
				Operator: filter.operator,
				Value:    filter.value,
			}
		}
		*relations = append(*relations, RelationInfo{
			Name:    pathStr,
			Filters: relationFilters,
		})
	}

	childKeys := sortedRelationKeys(node.children)
	for _, key := range childKeys {
		collectRelationDetails(node.children[key], currentPath, includes, relations)
	}
}

func includeRelation(q *bun.SelectQuery, node *relationIncludeNode) *bun.SelectQuery {
	if node == nil {
		return q
	}

	return q.Relation(node.name, func(rel *bun.SelectQuery) *bun.SelectQuery {
		rel = applyRelationFilters(rel, node.filters)
		childKeys := sortedRelationKeys(node.children)
		for _, key := range childKeys {
			rel = includeRelation(rel, node.children[key])
		}
		return rel
	})
}

func applyRelationFilters(q *bun.SelectQuery, filters []relationFilter) *bun.SelectQuery {
	for _, filter := range filters {
		column := filter.column
		if column == "" {
			column = filter.field
		}
		q = q.Where(fmt.Sprintf("%s %s ?", column, filter.operator), filter.value)
	}
	return q
}

func sortedRelationKeys(nodes map[string]*relationIncludeNode) []string {
	if len(nodes) == 0 {
		return nil
	}
	keys := make([]string, 0, len(nodes))
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergeRelationInfos(includePaths []string, requestInfos []RelationInfo, descriptor *router.RelationDescriptor) []RelationInfo {
	if descriptor == nil {
		return cloneRelationInfos(requestInfos)
	}

	requestMap := make(map[string][]RelationFilter, len(requestInfos))
	for _, info := range requestInfos {
		if info.Name == "" {
			continue
		}
		requestMap[strings.ToLower(info.Name)] = cloneRelationFilters(info.Filters)
	}

	descriptorFilterMap := make(map[string][]RelationFilter, len(descriptor.Relations))
	for _, info := range descriptor.Relations {
		if info.Name == "" {
			continue
		}
		key := strings.ToLower(info.Name)
		descriptorFilterMap[key] = convertRouterRelationFilters(info.Filters)
	}

	results := make([]RelationInfo, 0, len(includePaths))
	for _, path := range includePaths {
		if path == "" || !descriptorIncludes(descriptor, path) {
			continue
		}
		key := strings.ToLower(path)
		filters := requestMap[key]
		if len(filters) == 0 {
			filters = descriptorFilterMap[key]
		}
		if len(filters) == 0 {
			continue
		}
		results = append(results, RelationInfo{
			Name:    path,
			Filters: cloneRelationFilters(filters),
		})
	}

	// Preserve request-specific relations that might not have been part of includePaths (edge cases)
	for _, info := range requestInfos {
		if info.Name == "" || len(info.Filters) == 0 {
			continue
		}
		if !descriptorIncludes(descriptor, info.Name) {
			continue
		}
		found := false
		for _, existing := range results {
			if strings.EqualFold(existing.Name, info.Name) {
				found = true
				break
			}
		}
		if !found {
			results = append(results, RelationInfo{
				Name:    info.Name,
				Filters: cloneRelationFilters(info.Filters),
			})
		}
	}

	return results
}

func convertRouterRelationFilters(filters []router.RelationFilter) []RelationFilter {
	if len(filters) == 0 {
		return nil
	}
	out := make([]RelationFilter, len(filters))
	for i, filter := range filters {
		out[i] = RelationFilter{
			Field:    filter.Field,
			Operator: filter.Operator,
			Value:    filter.Value,
		}
	}
	return out
}

func cloneRelationFilters(filters []RelationFilter) []RelationFilter {
	if len(filters) == 0 {
		return nil
	}
	out := make([]RelationFilter, len(filters))
	copy(out, filters)
	return out
}

func cloneRelationInfos(infos []RelationInfo) []RelationInfo {
	if len(infos) == 0 {
		return nil
	}
	out := make([]RelationInfo, len(infos))
	for i, info := range infos {
		out[i] = RelationInfo{
			Name:    info.Name,
			Filters: cloneRelationFilters(info.Filters),
		}
	}
	return out
}

type queryTraceOptions struct {
	logger  Logger
	enabled bool
}

func (o *queryTraceOptions) debug(filters *Filters, params map[string]string) {
	if o == nil || !o.enabled || o.logger == nil {
		return
	}

	fields := Fields{
		"filters": filters,
	}

	if len(params) > 0 {
		fields["query_params"] = cloneStringMap(params)
	}

	if loggerWithFields, ok := o.logger.(loggerWithFields); ok {
		loggerWithFields.WithFields(fields).Debug("query criteria built")
		return
	}

	o.logger.Debug("query criteria built filters=%+v query_params=%+v", filters, fields["query_params"])
}
