package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/goliatone/go-crud"
)

type requestContext struct {
	userCtx context.Context

	params  map[string]string
	query   map[string][]string
	headers map[string]string

	statusCode int
	response   any
}

func newRequestContext(base context.Context, meta RequestMeta) *requestContext {
	if base == nil {
		base = context.Background()
	}
	ctx := base

	actor := actorFromMeta(meta)
	if !actor.IsZero() {
		ctx = crud.ContextWithActor(ctx, actor)
	}

	scope := scopeFromMeta(meta.Scope)
	if scope.HasFilters() || scope.Bypass || len(scope.Labels) > 0 || len(scope.Raw) > 0 {
		ctx = crud.ContextWithScope(ctx, scope)
	}

	if requestID := strings.TrimSpace(meta.RequestID); requestID != "" {
		ctx = crud.ContextWithRequestID(ctx, requestID)
	}
	if corrID := strings.TrimSpace(meta.CorrelationID); corrID != "" {
		ctx = crud.ContextWithCorrelationID(ctx, corrID)
	}

	return &requestContext{
		userCtx:  ctx,
		params:   cloneStringMap(meta.Params),
		query:    cloneQueryMap(meta.Query),
		headers:  normalizeHeaders(meta.Headers),
		response: nil,
	}
}

func actorFromMeta(meta RequestMeta) crud.ActorContext {
	roles := normalizeStringSlice(meta.Roles)
	permissions := normalizeStringSlice(meta.Permissions)

	actor := crud.ActorContext{
		ActorID:  strings.TrimSpace(meta.ActorID),
		TenantID: strings.TrimSpace(meta.Tenant),
	}

	if len(roles) > 0 {
		actor.Role = roles[0]
	}
	if len(roles) > 0 || len(permissions) > 0 {
		actor.Metadata = map[string]any{}
		if len(roles) > 0 {
			actor.Metadata["roles"] = roles
		}
		if len(permissions) > 0 {
			actor.Metadata["permissions"] = permissions
		}
	}

	return actor
}

func scopeFromMeta(raw map[string]any) crud.ScopeFilter {
	scope := crud.ScopeFilter{}
	if len(raw) == 0 {
		return scope
	}

	scope.Raw = cloneAnyMap(raw)
	if bypass, ok := asBool(raw["bypass"]); ok {
		scope.Bypass = bypass
	}

	if labels, ok := readStringMap(raw["labels"]); ok && len(labels) > 0 {
		scope.Labels = labels
	}

	for _, key := range []string{"columnFilters", "column_filters"} {
		for _, filter := range readColumnFilters(raw[key]) {
			scope.AddColumnFilter(filter.Column, filter.Operator, filter.Values...)
		}
	}

	if len(scope.ColumnFilters) == 0 {
		for key, value := range raw {
			switch key {
			case "bypass", "labels", "columnFilters", "column_filters":
				continue
			}
			values := asStringSlice(value)
			if len(values) == 0 {
				continue
			}
			scope.AddColumnFilter(key, "=", values...)
		}
	}

	return scope
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneQueryMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(in))
	for key, value := range in {
		out[key] = append([]string(nil), value...)
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func readStringMap(raw any) (map[string]string, bool) {
	source, ok := mapAsStringAny(raw)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		if str, ok := asString(value); ok {
			out[key] = str
		}
	}
	return out, true
}

func readColumnFilters(raw any) []crud.ScopeColumnFilter {
	items, ok := asAnySlice(raw)
	if !ok || len(items) == 0 {
		return nil
	}

	filters := make([]crud.ScopeColumnFilter, 0, len(items))
	for _, item := range items {
		entry, ok := mapAsStringAny(item)
		if !ok || len(entry) == 0 {
			continue
		}

		column, _ := asString(entry["column"])
		operator, _ := asString(entry["operator"])
		values := asStringSlice(entry["values"])
		if len(values) == 0 {
			values = asStringSlice(entry["value"])
		}
		if strings.TrimSpace(column) == "" || len(values) == 0 {
			continue
		}

		filters = append(filters, crud.ScopeColumnFilter{
			Column:   column,
			Operator: operator,
			Values:   values,
		})
	}

	return filters
}

func asStringSlice(raw any) []string {
	if value, ok := asString(raw); ok {
		return []string{value}
	}

	switch typed := raw.(type) {
	case nil:
		return nil
	case []string:
		return normalizeStringSlice(typed)
	}

	items, ok := asAnySlice(raw)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := asString(item); ok {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func asAnySlice(raw any) ([]any, bool) {
	if raw == nil {
		return nil, false
	}
	if typed, ok := raw.([]any); ok {
		return typed, true
	}

	value := reflect.ValueOf(raw)
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return nil, false
	}

	out := make([]any, 0, value.Len())
	for i := range value.Len() {
		out = append(out, value.Index(i).Interface())
	}
	return out, true
}

func mapAsStringAny(raw any) (map[string]any, bool) {
	if raw == nil {
		return nil, false
	}

	switch typed := raw.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out, true
	}

	value := reflect.ValueOf(raw)
	if value.Kind() != reflect.Map {
		return nil, false
	}

	out := make(map[string]any, value.Len())
	iter := value.MapRange()
	for iter.Next() {
		key, ok := asString(iter.Key().Interface())
		if !ok {
			continue
		}
		out[key] = iter.Value().Interface()
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func asString(raw any) (string, bool) {
	switch typed := raw.(type) {
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return trimmed, true
		}
	case []byte:
		if trimmed := strings.TrimSpace(string(typed)); trimmed != "" {
			return trimmed, true
		}
	case json.Number:
		if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
			return trimmed, true
		}
	case fmt.Stringer:
		if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
			return trimmed, true
		}
	case bool:
		return strconv.FormatBool(typed), true
	case int:
		return strconv.FormatInt(int64(typed), 10), true
	case int8:
		return strconv.FormatInt(int64(typed), 10), true
	case int16:
		return strconv.FormatInt(int64(typed), 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case uintptr:
		return strconv.FormatUint(uint64(typed), 10), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	}
	return "", false
}

func asBool(raw any) (bool, bool) {
	switch typed := raw.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return parsed, true
		}
	}
	return false, false
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		lookupKey := headerLookupKey(key)
		if lookupKey == "" {
			continue
		}
		out[lookupKey] = value
	}
	return out
}

func headerLookupKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func (c *requestContext) UserContext() context.Context {
	return c.userCtx
}

func (c *requestContext) SetUserContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	c.userCtx = ctx
}

func (c *requestContext) Params(key string, defaultValue ...string) string {
	if value, ok := c.params[key]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (c *requestContext) BodyParser(out any) error {
	if out == nil {
		return nil
	}
	return fmt.Errorf("body parser not supported in rpc request context")
}

func (c *requestContext) Query(key string, defaultValue ...string) string {
	values := c.QueryValues(key)
	if len(values) > 0 {
		return values[0]
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (c *requestContext) QueryValues(key string) []string {
	values := c.query[key]
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func (c *requestContext) QueryInt(key string, defaultValue ...int) int {
	value := c.Query(key)
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return parsed
}

func (c *requestContext) Queries() map[string]string {
	out := make(map[string]string, len(c.query))
	for key, values := range c.query {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func (c *requestContext) Body() []byte {
	return nil
}

func (c *requestContext) Header(key string) string {
	return c.headers[headerLookupKey(key)]
}

func (c *requestContext) Status(status int) crud.Response {
	c.statusCode = status
	return c
}

func (c *requestContext) JSON(data any, _ ...string) error {
	c.response = data
	return nil
}

func (c *requestContext) SendStatus(status int) error {
	c.statusCode = status
	return nil
}
