package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/goliatone/go-crud"
)

type graphqlContext struct {
	ctx    context.Context
	opCtx  *graphql.OperationContext
	status int
	body   []byte
}

// GraphQLToCrudContext adapts a gqlgen context.Context into a crud.Context.
// The adapter is read-only: Params resolve from GraphQL variables, Query/Queries
// return empty values, BodyParser copies variables into the target, and JSON/Status
// are no-ops suitable for resolver usage.
func GraphQLToCrudContext(ctx context.Context) crud.Context {
	var op *graphql.OperationContext
	if graphql.HasOperationContext(ctx) {
		op = graphql.GetOperationContext(ctx)
	}
	return &graphqlContext{
		ctx:   ctx,
		opCtx: op,
	}
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
	if g.opCtx != nil && g.opCtx.Headers != nil {
		return g.opCtx.Headers.Get(key)
	}
	return ""
}

func (g *graphqlContext) Status(status int) crud.Response {
	g.status = status
	return g
}

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
