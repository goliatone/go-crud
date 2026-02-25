package rpc

import (
	"context"
	"fmt"
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
		headers:  cloneStringMap(meta.Headers),
		response: nil,
	}
}

func actorFromMeta(meta RequestMeta) crud.ActorContext {
	actor := crud.ActorContext{
		ActorID:  strings.TrimSpace(meta.ActorID),
		TenantID: strings.TrimSpace(meta.Tenant),
	}
	if len(meta.Roles) > 0 {
		roles := make([]string, 0, len(meta.Roles))
		for _, role := range meta.Roles {
			if trimmed := strings.TrimSpace(role); trimmed != "" {
				roles = append(roles, trimmed)
			}
		}
		if len(roles) > 0 {
			actor.Role = roles[0]
			actor.Metadata = map[string]any{
				"roles":       roles,
				"permissions": append([]string(nil), meta.Permissions...),
			}
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
	if bypass, ok := raw["bypass"].(bool); ok {
		scope.Bypass = bypass
	}

	if labels, ok := readStringMap(raw["labels"]); ok && len(labels) > 0 {
		scope.Labels = labels
	}

	for _, key := range []string{"columnFilters", "column_filters"} {
		filters, ok := raw[key].([]any)
		if !ok || len(filters) == 0 {
			continue
		}
		for _, item := range filters {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			column, _ := entry["column"].(string)
			operator, _ := entry["operator"].(string)
			values := asStringSlice(entry["values"])
			if len(values) == 0 {
				if single, ok := entry["value"].(string); ok {
					values = []string{single}
				}
			}
			scope.AddColumnFilter(column, operator, values...)
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
	source, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		if str, ok := value.(string); ok {
			out[key] = str
		}
	}
	return out, true
}

func asStringSlice(raw any) []string {
	switch typed := raw.(type) {
	case nil:
		return nil
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return []string{trimmed}
		}
	case []string:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok {
				if trimmed := strings.TrimSpace(str); trimmed != "" {
					out = append(out, trimmed)
				}
			}
		}
		return out
	}
	return nil
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
	return c.headers[key]
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
