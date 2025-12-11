package helpers

import (
	"context"
	"net/http"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphQLContext_ParamsAndBodyParser(t *testing.T) {
	op := &graphql.OperationContext{
		RawQuery:      "query($id: ID!, $limit: Int){ post(id: $id) { id } }",
		Variables:     map[string]any{"id": "abc-123", "limit": 5},
		OperationName: "Post",
	}
	ctx := graphql.WithOperationContext(context.Background(), op)

	crudCtx := GraphQLToCrudContext(ctx)

	assert.Equal(t, "abc-123", crudCtx.Params("id"))
	assert.Equal(t, "default", crudCtx.Params("missing", "default"))

	var payload struct {
		Limit int `json:"limit"`
	}
	require.NoError(t, crudCtx.BodyParser(&payload))
	assert.Equal(t, 5, payload.Limit)

	body := crudCtx.Body()
	require.NotNil(t, body)
	assert.Contains(t, string(body), `"query"`)
	assert.Contains(t, string(body), `"limit":5`)
	assert.Contains(t, string(body), `"operationName":"Post"`)
}

func TestGraphQLContext_HeadersPassthrough(t *testing.T) {
	op := &graphql.OperationContext{
		Headers: http.Header{
			"X-Request-ID": []string{"req-123"},
			"Authorization": []string{
				"Bearer token",
			},
		},
	}
	ctx := graphql.WithOperationContext(context.Background(), op)

	crudCtx := GraphQLToCrudContext(ctx)

	hc, ok := crudCtx.(interface{ Header(string) string })
	require.True(t, ok, "adapter should expose Header for request metadata")

	assert.Equal(t, "req-123", hc.Header("X-Request-ID"))
	assert.Equal(t, "Bearer token", hc.Header("Authorization"))
	assert.Equal(t, "", hc.Header("Missing"))
}

func TestGraphQLContext_QueryDefaults(t *testing.T) {
	crudCtx := GraphQLToCrudContext(context.Background())

	assert.Equal(t, "fallback", crudCtx.Query("any", "fallback"))
	assert.Equal(t, 42, crudCtx.QueryInt("num", 42))

	var out struct {
		Value string `json:"value"`
	}
	// No variables present should be a no-op, not an error.
	require.NoError(t, crudCtx.BodyParser(&out))
	assert.Equal(t, "", out.Value)
}
