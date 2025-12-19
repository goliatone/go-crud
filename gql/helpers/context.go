package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/goliatone/go-crud"
)

type graphqlContext struct {
	ctx     context.Context
	opCtx   *graphql.OperationContext
	headers http.Header
	status  int
	body    []byte
}

// graphqlContext implements crud.Context on top of gqlgen's OperationContext
// without mutating the underlying request context (suited for resolver paths).
// GraphQLToCrudContext adapts a gqlgen context.Context into a crud.Context.
// The adapter is read-only: Params resolve from GraphQL variables, Query/Queries
// return empty values, BodyParser copies variables into the target, and JSON/Status
// are no-ops suitable for resolver usage.
func GraphQLToCrudContext(ctx context.Context) crud.Context {
	op := safeOperationContext(ctx)
	hdr := cloneHeaders(op, safeRequestContext(ctx))
	return &graphqlContext{ctx: ctx, opCtx: op, headers: hdr}
}

func (g *graphqlContext) UserContext() context.Context {
	return g.ctx
}

func (g *graphqlContext) SetUserContext(ctx context.Context) {
	if ctx != nil {
		g.ctx = ctx
	}
}

func (g *graphqlContext) Params(key string, defaultValue ...string) string {
	if g.opCtx != nil && g.opCtx.Variables != nil {
		if v, ok := g.opCtx.Variables[key]; ok {
			return fmt.Sprint(v)
		}
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (g *graphqlContext) Query(key string, defaultValue ...string) string {
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (g *graphqlContext) QueryValues(key string) []string {
	val := g.Query(key)
	if val == "" {
		return []string{}
	}
	return []string{val}
}

func (g *graphqlContext) QueryInt(key string, defaultValue ...int) int {
	val := strings.TrimSpace(g.Query(key))
	if val == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

func (g *graphqlContext) Queries() map[string]string {
	return map[string]string{}
}

// Body renders the GraphQL operation payload so downstream code can inspect it
// (e.g., for auditing) even though resolvers already parsed inputs.
func (g *graphqlContext) Body() []byte {
	if g.body != nil {
		return g.body
	}
	if g.opCtx == nil {
		return nil
	}
	payload := map[string]any{
		"query":         g.opCtx.RawQuery,
		"variables":     g.opCtx.Variables,
		"operationName": g.opCtx.OperationName,
	}
	if data, err := json.Marshal(payload); err == nil {
		g.body = data
	}
	return g.body
}

func (g *graphqlContext) BodyParser(out any) error {
	if g.opCtx == nil || g.opCtx.Variables == nil {
		return nil
	}
	data, err := json.Marshal(g.opCtx.Variables)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (g *graphqlContext) Header(key string) string {
	if g.headers != nil {
		return g.headers.Get(key)
	}
	rc := safeRequestContext(g.ctx)
	if rc != nil && rc.Headers != nil {
		return rc.Headers.Get(key)
	}
	if op := safeOperationContext(g.ctx); op != nil && op.Headers != nil {
		return op.Headers.Get(key)
	}
	return ""
}

func safeOperationContext(ctx context.Context) *graphql.OperationContext {
	if ctx == nil {
		return nil
	}
	defer func() { _ = recover() }()
	return graphql.GetOperationContext(ctx)
}

func safeRequestContext(ctx context.Context) *graphql.RequestContext {
	if ctx == nil {
		return nil
	}
	defer func() { _ = recover() }()
	return graphql.GetRequestContext(ctx)
}

func cloneHeaders(op *graphql.OperationContext, rc *graphql.RequestContext) http.Header {
	src := http.Header(nil)
	if op != nil && op.Headers != nil {
		src = op.Headers
	} else if rc != nil && rc.Headers != nil {
		src = rc.Headers
	}
	if src == nil {
		return nil
	}
	dst := make(http.Header, len(src))
	for k, vals := range src {
		ck := textproto.CanonicalMIMEHeaderKey(k)
		copied := make([]string, len(vals))
		copy(copied, vals)
		dst[ck] = append(dst[ck], copied...)
	}
	return dst
}

func (g *graphqlContext) Status(status int) crud.Response {
	g.status = status
	return g
}

// JSON/SendStatus are no-ops; gqlgen writes the response once resolver execution
// completes. Status is kept for parity with HTTP handlers in tests.
func (g *graphqlContext) JSON(data any, ctype ...string) error {
	if g.status == 0 {
		g.status = http.StatusOK
	}
	return nil
}

func (g *graphqlContext) SendStatus(status int) error {
	g.status = status
	return nil
}
