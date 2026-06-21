package crud

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	querybun "github.com/goliatone/go-crud/pkg/go-query-bun"
	"github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
	"github.com/uptrace/bun"
)

type relationFilter struct {
	field    string
	operator string
	value    string
	column   string
	criteria querybun.Criteria
}

type relationIncludeNode struct {
	name        string
	requestName string
	filters     []relationFilter
	children    map[string]*relationIncludeNode
}

type QueryBuilderOption func(*queryBuilderConfig)

type queryBuilderConfig struct {
	trace               *queryTraceOptions
	allowedFields       map[string]string
	fieldResolver       querybun.FieldResolver
	searchColumns       []string
	searchConfig        *querybun.SearchConfig
	operators           *querybun.OperatorSet
	predicateBuilders   *querybun.PredicateBuilderSet
	validation          *querybun.ValidationConfig
	strictValidation    *bool
	strictSearchColumns *bool
}

func WithAllowedFields(fields map[string]string) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		if len(fields) == 0 {
			return
		}
		cfg.allowedFields = fields
		cfg.fieldResolver = querybun.NewMapResolver(fields)
	}
}

// WithFieldResolver configures semantic field resolution for query planning.
func WithFieldResolver(resolver querybun.FieldResolver) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		cfg.fieldResolver = resolver
	}
}

// WithStrictQueryValidation enables strict operator validation for this build call.
// Unsupported operators return QueryValidationError instead of falling back to eq.
func WithStrictQueryValidation(enabled bool) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		cfg.strictValidation = new(enabled)
	}
}

// WithSearchColumns configures the fields/columns used to resolve _search.
// Search matches OR across these columns and ANDs the group with other filters.
func WithSearchColumns(columns ...string) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		if len(columns) == 0 {
			cfg.searchColumns = nil
			return
		}
		cfg.searchColumns = append([]string{}, columns...)
	}
}

// WithSearchConfig configures search aliases, fields, and matching behavior.
func WithSearchConfig(search querybun.SearchConfig) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		cfg.searchConfig = &search
	}
}

// WithOperatorSet configures semantic operators for query planning.
func WithOperatorSet(operators querybun.OperatorSet) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		cfg.operators = &operators
	}
}

// WithPredicateBuilders configures custom predicate builders.
func WithPredicateBuilders(builders querybun.PredicateBuilderSet) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		cfg.predicateBuilders = &builders
	}
}

// WithValidationConfig configures planner validation behavior.
func WithValidationConfig(validation querybun.ValidationConfig) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		cfg.validation = &validation
	}
}

// WithStrictSearchColumns makes strict mode return a typed error when _search
// is provided and no searchable columns are configured/resolved.
func WithStrictSearchColumns(enabled bool) QueryBuilderOption {
	return func(cfg *queryBuilderConfig) {
		cfg.strictSearchColumns = new(enabled)
	}
}

func (cfg queryBuilderConfig) strictValidationEnabled() bool {
	if cfg.strictValidation != nil {
		return *cfg.strictValidation
	}
	return StrictQueryValidationEnabled()
}

func (cfg queryBuilderConfig) strictSearchColumnsEnabled() bool {
	if cfg.strictSearchColumns != nil {
		return *cfg.strictSearchColumns
	}
	return false
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

func paginationCriteria(limit, offset int) repository.SelectCriteria {
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Limit(limit).Offset(offset)
	}
}

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
	queryParams := ctx.Queries()
	opts := queryBunOptionsFromContext(ctx, queryParams)
	plan, err := querybun.BuildQueryPlan(opts, queryBunConfig[T](cfg))
	if err != nil {
		return nil, nil, convertQueryBunError(err)
	}

	filters := filtersFromQueryBunPlan(plan, op)
	criteria := adaptQueryBunCriteria(plan.ListCriteria())
	if op != OpList {
		criteria = adaptQueryBunCriteria(plan.ReadCriteria())
	}

	includeCriteria, includePaths, relations, err := buildIncludeCriteriaForType[T](strings.Join(filters.Include, ","), cfg)
	if err != nil {
		return nil, nil, err
	}
	if len(includePaths) > 0 {
		filters.Include = includePaths
		filters.Relations = relations
		criteria = append(criteria, includeCriteria...)
	}

	if cfg.trace != nil {
		cfg.trace.debug(filters, queryParams)
	}

	return criteria, filters, nil
}

func queryBunOptionsFromContext(ctx Context, queryParams map[string]string) querybun.ListOptions {
	limit := ctx.QueryInt("limit", DefaultLimit)
	offset := ctx.QueryInt("offset", DefaultOffset)
	include := normalizeIncludeParams(ctx)

	filters := make(map[string]any)
	for param, value := range queryParams {
		if isReservedQueryParam(param) {
			continue
		}
		filters[param] = value
	}

	var selectFields []string
	if raw := strings.TrimSpace(ctx.Query("select")); raw != "" {
		selectFields = []string{raw}
	}

	var includes []string
	if include != "" {
		includes = []string{include}
	}

	return querybun.ListOptions{
		Limit:     limit,
		LimitSet:  true,
		Offset:    offset,
		OffsetSet: true,
		Order:     ctx.Query("order"),
		Search:    ctx.Query("_search"),
		Filters:   filters,
		Select:    selectFields,
		Include:   includes,
	}
}

func isReservedQueryParam(param string) bool {
	switch param {
	case "limit", "offset", "order", "select", "include", "_search":
		return true
	default:
		return false
	}
}

func queryBunConfig[T any](cfg queryBuilderConfig) querybun.Config {
	allowedFieldsMap := cfg.allowedFields
	if len(allowedFieldsMap) == 0 {
		allowedFieldsMap = getAllowedFields[T]()
	}
	resolver := cfg.fieldResolver
	if resolver == nil {
		resolver = querybun.NewMapResolver(allowedFieldsMap)
	}
	operators := operatorSet
	if cfg.operators != nil {
		operators = *cfg.operators
	}
	search := querybun.SearchConfig{
		Aliases: []string{"_search", "search", "q"},
		Fields:  append([]string{}, cfg.searchColumns...),
		Pattern: querybun.PatternConfig{Mode: querybun.PatternContains, EscapeChar: `\`, MultiValue: querybun.MultiValueAny},
		Case:    querybun.CaseInsensitive,
	}
	if cfg.searchConfig != nil {
		search = *cfg.searchConfig
	}
	predicates := querybun.PredicateBuilderSet{}
	if cfg.predicateBuilders != nil {
		predicates = *cfg.predicateBuilders
	}
	return querybun.Config{
		Syntax:     querybun.DefaultRESTSyntax(),
		Fields:     querybun.FieldConfig{Resolver: resolver},
		Operators:  operators,
		Search:     search,
		Predicates: predicates,
		Pagination: querybun.PaginationConfig{
			DefaultLimit:  DefaultLimit,
			DefaultOffset: DefaultOffset,
		},
		Validation: queryValidationConfig(cfg),
	}
}

func queryValidationConfig(cfg queryBuilderConfig) querybun.ValidationConfig {
	validation := querybun.ValidationConfig{Mode: querybun.ValidationCollect}
	if cfg.strictValidationEnabled() {
		validation.Mode = querybun.ValidationStrict
	}
	validation.StrictSearchFields = cfg.strictSearchColumnsEnabled()
	if cfg.validation != nil {
		validation = *cfg.validation
	}
	return validation
}

func adaptQueryBunCriteria(criteria []querybun.Criteria) []repository.SelectCriteria {
	if len(criteria) == 0 {
		return nil
	}
	out := make([]repository.SelectCriteria, 0, len(criteria))
	for _, criterion := range criteria {
		if criterion == nil {
			continue
		}
		fn := criterion
		out = append(out, func(q *bun.SelectQuery) *bun.SelectQuery {
			return fn(q)
		})
	}
	return out
}

func filtersFromQueryBunPlan(plan querybun.Plan, op CrudOperation) *Filters {
	filters := &Filters{
		Operation: string(op),
		Limit:     plan.Metadata.Limit,
		Offset:    plan.Metadata.Offset,
		Search:    plan.Metadata.Search,
	}
	if len(plan.Metadata.Order) > 0 {
		filters.Order = make([]Order, len(plan.Metadata.Order))
		for i, order := range plan.Metadata.Order {
			filters.Order[i] = Order{Field: order.Field, Dir: order.Dir}
		}
	}
	filters.Fields = append([]string{}, plan.Metadata.Fields...)
	filters.Include = append([]string{}, plan.Metadata.Include...)
	return filters
}

func buildIncludeCriteriaForType[T any](include string, cfg queryBuilderConfig) ([]repository.SelectCriteria, []string, []RelationInfo, error) {
	if strings.TrimSpace(include) == "" {
		return nil, nil, nil, nil
	}
	meta := getRelationMetadataForType(typeOf[T]())
	includeNodes, err := buildIncludeTree(include, meta, cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(includeNodes) == 0 {
		return nil, nil, nil, nil
	}

	includePaths, relationInfos := collectIncludeDetails(includeNodes)
	descriptor := getRelationDescriptorForType(typeOf[T]())
	relations := mergeRelationInfos(includePaths, relationInfos, descriptor)

	rootKeys := sortedRelationKeys(includeNodes)
	criteria := make([]repository.SelectCriteria, 0, len(rootKeys))
	for _, key := range rootKeys {
		node := includeNodes[key]
		if node == nil {
			continue
		}
		criteria = append(criteria, func(n *relationIncludeNode) repository.SelectCriteria {
			return func(q *bun.SelectQuery) *bun.SelectQuery {
				return includeRelation(q, n)
			}
		}(node))
	}

	return criteria, includePaths, relations, nil
}

func convertQueryBunError(err error) error {
	if err == nil {
		return nil
	}
	var validationErr *querybun.ValidationError
	if !errors.As(err, &validationErr) {
		return err
	}
	switch validationErr.Code {
	case querybun.ValidationUnsupportedOperator:
		return &QueryValidationError{
			Code:     QueryValidationUnsupportedOperator,
			Field:    validationErr.Field,
			Operator: validationErr.Operator,
			Search:   validationErr.Search,
		}
	case querybun.ValidationSearchColumnsRequired:
		return &QueryValidationError{
			Code:   QueryValidationSearchColumnsRequired,
			Field:  validationErr.Field,
			Search: validationErr.Search,
		}
	default:
		return err
	}
}

func normalizeIncludeParams(ctx Context) string {
	if ctx == nil {
		return ""
	}
	values := ctx.QueryValues("include")
	if len(values) == 0 {
		if val := strings.TrimSpace(ctx.Query("include")); val != "" {
			values = []string{val}
		}
	}
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, raw := range values {
		for item := range strings.SplitSeq(raw, ",") {
			if val := strings.TrimSpace(item); val != "" {
				parts = append(parts, val)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}

func buildIncludeTree(includeParam string, meta *relationMetadata, configs ...queryBuilderConfig) (map[string]*relationIncludeNode, error) {
	result := make(map[string]*relationIncludeNode)
	if strings.TrimSpace(includeParam) == "" {
		return result, nil
	}
	cfg := queryBuilderConfig{}
	if len(configs) > 0 {
		cfg = configs[0]
	}

	paths := strings.SplitSeq(includeParam, ",")
	for raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}

		node, err := parseIncludePath(path, meta, cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid relation include %q: %w", path, err)
		}
		mergeIncludeTrees(result, node)
	}

	return result, nil
}

func parseIncludePath(path string, meta *relationMetadata, cfg queryBuilderConfig) (*relationIncludeNode, error) {
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
			filter, keep, err := parseIncludeFilterSegment(segment, current, currentMeta, cfg)
			if err != nil {
				return nil, err
			}
			if keep {
				current.filters = append(current.filters, filter)
			}
			continue
		}

		node, childMeta, err := parseIncludeRelationSegment(segment, currentMeta)
		if err != nil {
			return nil, err
		}
		attachIncludeNode(current, node)
		if current == nil {
			root = node
		}
		current = node
		currentMeta = childMeta
	}

	if root == nil {
		return nil, fmt.Errorf("invalid include path %q", path)
	}

	return root, nil
}

func parseIncludeFilterSegment(segment string, current *relationIncludeNode, currentMeta *relationMetadata, cfg queryBuilderConfig) (relationFilter, bool, error) {
	if current == nil {
		return relationFilter{}, false, fmt.Errorf("filter specified before relation")
	}
	fieldPart, value, found := strings.Cut(segment, "=")
	if !found {
		return relationFilter{}, false, fmt.Errorf("invalid filter syntax")
	}
	fieldName, operatorKind := querybun.ParsePredicateKey(fieldPart)
	columnName := relationFilterColumn(currentMeta, fieldName)
	if columnName == "" {
		return relationFilter{}, false, fmt.Errorf("unsupported filter field %q on relation %q", fieldName, current.requestName)
	}

	filterCfg := queryBunRelationFilterConfig(currentMeta.fields, cfg)
	filterCriteria, unsupported, err := querybun.BuildFilterCriteriaFromPredicates([]querybun.Predicate{
		{Field: fieldName, Operator: operatorKind, Values: []string{value}, RawKey: fieldPart, RawValue: value},
	}, filterCfg)
	if err != nil {
		return relationFilter{}, false, convertQueryBunError(err)
	}
	if len(unsupported) > 0 || len(filterCriteria) == 0 {
		return relationFilter{}, false, relationFilterUnsupportedError(fieldName, operatorKind, cfg)
	}

	operator, err := querybun.ResolveOperator(string(operatorKind), fieldName, filterCfg)
	if err != nil {
		return relationFilter{}, false, convertQueryBunError(err)
	}
	return relationFilter{
		field:    fieldName,
		operator: operator.SQL,
		value:    value,
		column:   columnName,
		criteria: filterCriteria[0],
	}, true, nil
}

func relationFilterColumn(meta *relationMetadata, fieldName string) string {
	if meta == nil || meta.fields == nil {
		return ""
	}
	return meta.fields[fieldName]
}

func relationFilterUnsupportedError(fieldName string, operator querybun.OperatorKind, cfg queryBuilderConfig) error {
	if queryValidationConfig(cfg).Mode != querybun.ValidationStrict {
		return nil
	}
	return &QueryValidationError{
		Code:     QueryValidationUnsupportedOperator,
		Field:    fieldName,
		Operator: string(operator),
	}
}

func parseIncludeRelationSegment(segment string, meta *relationMetadata) (*relationIncludeNode, *relationMetadata, error) {
	if meta == nil {
		return nil, nil, fmt.Errorf("relation metadata unavailable for %q", segment)
	}
	childMeta, ok := meta.children[strings.ToLower(segment)]
	if !ok {
		return nil, nil, fmt.Errorf("unknown relation %q", segment)
	}
	node := &relationIncludeNode{
		name:        childMeta.relationName,
		requestName: segment,
		filters:     make([]relationFilter, 0),
		children:    make(map[string]*relationIncludeNode),
	}
	return node, childMeta, nil
}

func attachIncludeNode(parent, node *relationIncludeNode) {
	if parent == nil {
		return
	}
	parent.children[node.name] = node
}

func queryBunRelationFilterConfig(fields map[string]string, cfg queryBuilderConfig) querybun.Config {
	operators := operatorSet
	if cfg.operators != nil {
		operators = *cfg.operators
	}
	validation := queryValidationConfig(cfg)
	validation.StrictFields = true
	return querybun.Config{
		Syntax:     querybun.DefaultRESTSyntax(),
		Fields:     querybun.FieldConfig{Resolver: querybun.NewMapResolver(fields)},
		Operators:  operators,
		Validation: validation,
	}
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
		if filter.criteria != nil {
			q = filter.criteria(q)
			continue
		}
		column := filter.column
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

	requestMap := relationFilterMap(requestInfos)
	descriptorFilterMap := routerRelationFilterMap(descriptor.Relations)

	results := make([]RelationInfo, 0, len(includePaths))
	for _, path := range includePaths {
		if !shouldIncludeRelationInfo(descriptor, path) {
			continue
		}
		key := strings.ToLower(path)
		filters := firstRelationFilters(requestMap[key], descriptorFilterMap[key])
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
		if !shouldAppendRequestRelationInfo(results, descriptor, info) {
			continue
		}
		results = append(results, RelationInfo{
			Name:    info.Name,
			Filters: cloneRelationFilters(info.Filters),
		})
	}

	return results
}

func relationFilterMap(infos []RelationInfo) map[string][]RelationFilter {
	out := make(map[string][]RelationFilter, len(infos))
	for _, info := range infos {
		if info.Name != "" {
			out[strings.ToLower(info.Name)] = cloneRelationFilters(info.Filters)
		}
	}
	return out
}

func routerRelationFilterMap(infos []router.RelationInfo) map[string][]RelationFilter {
	out := make(map[string][]RelationFilter, len(infos))
	for _, info := range infos {
		if info.Name != "" {
			out[strings.ToLower(info.Name)] = convertRouterRelationFilters(info.Filters)
		}
	}
	return out
}

func shouldIncludeRelationInfo(descriptor *router.RelationDescriptor, path string) bool {
	return path != "" && descriptorIncludes(descriptor, path)
}

func firstRelationFilters(primary, fallback []RelationFilter) []RelationFilter {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func shouldAppendRequestRelationInfo(results []RelationInfo, descriptor *router.RelationDescriptor, info RelationInfo) bool {
	if info.Name == "" || len(info.Filters) == 0 || !descriptorIncludes(descriptor, info.Name) {
		return false
	}
	return !relationInfoExists(results, info.Name)
}

func relationInfoExists(infos []RelationInfo, name string) bool {
	for _, info := range infos {
		if strings.EqualFold(info.Name, name) {
			return true
		}
	}
	return false
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

	if withFields, ok := o.logger.(loggerWithFields); ok {
		withFields.WithFields(fields).Debug("query criteria built")
		return
	}

	o.logger.Debug("query criteria built filters=%+v query_params=%+v", filters, fields["query_params"])
}
