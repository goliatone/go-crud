package rpc

import (
	"context"
	"testing"

	"github.com/goliatone/go-crud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActorFromMetaIncludesPermissionsWithoutRoles(t *testing.T) {
	actor := actorFromMeta(RequestMeta{
		ActorID:     "actor-1",
		Tenant:      "acme",
		Permissions: []string{" users:read ", "", "users:write"},
	})

	assert.Equal(t, "actor-1", actor.ActorID)
	assert.Equal(t, "acme", actor.TenantID)
	assert.Equal(t, "", actor.Role)

	require.NotNil(t, actor.Metadata)
	permissions, ok := actor.Metadata["permissions"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"users:read", "users:write"}, permissions)

	_, hasRoles := actor.Metadata["roles"]
	assert.False(t, hasRoles)
}

func TestScopeFromMetaParsesTypedColumnFilterPayloads(t *testing.T) {
	scope := scopeFromMeta(map[string]any{
		"bypass": "true",
		"labels": map[string]any{
			"plan": "pro",
			"tier": 2,
		},
		"columnFilters": []map[string]any{
			{
				"column":   "tenant_id",
				"operator": "=",
				"value":    "acme",
			},
			{
				"column":   "age",
				"operator": ">=",
				"value":    21,
			},
		},
		"column_filters": []map[string]string{
			{
				"column":   "active",
				"operator": "=",
				"value":    "true",
			},
		},
	})

	assert.True(t, scope.Bypass)
	assert.Equal(t, map[string]string{"plan": "pro", "tier": "2"}, scope.Labels)
	require.Len(t, scope.ColumnFilters, 3)

	byColumn := toScopeFilterMap(scope)
	assert.Equal(t, []string{"acme"}, byColumn["tenant_id"].Values)
	assert.Equal(t, "=", byColumn["tenant_id"].Operator)
	assert.Equal(t, []string{"21"}, byColumn["age"].Values)
	assert.Equal(t, ">=", byColumn["age"].Operator)
	assert.Equal(t, []string{"true"}, byColumn["active"].Values)
}

func TestScopeFromMetaFallbackParsesPrimitiveValues(t *testing.T) {
	scope := scopeFromMeta(map[string]any{
		"tenant_id": 123,
		"active":    true,
		"region":    "us-east-1",
	})

	require.Len(t, scope.ColumnFilters, 3)
	byColumn := toScopeFilterMap(scope)
	assert.Equal(t, []string{"123"}, byColumn["tenant_id"].Values)
	assert.Equal(t, []string{"true"}, byColumn["active"].Values)
	assert.Equal(t, []string{"us-east-1"}, byColumn["region"].Values)
}

func TestRequestContextHeaderLookupCaseInsensitive(t *testing.T) {
	ctx := newRequestContext(context.Background(), RequestMeta{
		Headers: map[string]string{
			"x-request-id":     "req-1",
			"x-correlation-id": "corr-1",
		},
	})

	assert.Equal(t, "req-1", ctx.Header("X-Request-ID"))
	assert.Equal(t, "req-1", ctx.Header("x-request-id"))
	assert.Equal(t, "corr-1", ctx.Header("X-Correlation-ID"))
}

func toScopeFilterMap(scope crud.ScopeFilter) map[string]crud.ScopeColumnFilter {
	out := make(map[string]crud.ScopeColumnFilter, len(scope.ColumnFilters))
	for _, filter := range scope.ColumnFilters {
		out[filter.Column] = filter
	}
	return out
}
