package querybun

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapterContract_UserInventoryPlannerBehavior(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Fields.Resolver = ComposeResolvers(
		NewMapResolver(map[string]string{
			"id":       "id",
			"username": "username",
			"email":    "email",
			"status":   "status",
		}),
		SafeIdentifierResolver{},
	)
	cfg.Search.Fields = []string{"username", "email"}
	cfg.Predicates.Fields = map[string]PredicateBuilder{
		"app_role": PredicateBuilderFunc(func(in PredicateBuilderInput) (PredicateBuilderResult, error) {
			if in.Operator.Canonical != OperatorEq {
				return PredicateUnhandled(), nil
			}
			return PredicateHandled(in.Helpers.Compare("role_name", "=", in.Values)), nil
		}),
	}

	plan, err := BuildQueryPlan(ListOptions{
		Limit:     25,
		LimitSet:  true,
		Offset:    10,
		OffsetSet: true,
		Page:      3,
		PerPage:   100,
		Order:     "username desc",
		Search:    "explicit-search",
		Filters: map[string]any{
			"q": "legacy-search",
		},
		Predicates: []Predicate{
			{Field: "email", Operator: OperatorContains, Values: []string{"ops_%@example.com"}},
			{Field: "app_role", Operator: OperatorEq, Values: []string{"admin"}},
		},
		Select: []string{"id,username"},
	}, cfg)
	require.NoError(t, err)
	assert.Empty(t, plan.Unsupported)
	assert.Equal(t, "explicit-search", plan.Metadata.Search)
	assert.Equal(t, 25, plan.Metadata.Limit)
	assert.Equal(t, 10, plan.Metadata.Offset)
	assert.Equal(t, 1, plan.Metadata.Page)
	assert.Len(t, plan.CountCriteria(), 2)
	assert.Len(t, plan.PageCriteria(), 1)
	assert.NotEmpty(t, plan.ListCriteria())
	assert.Equal(t, []Order{{Field: "username", Dir: "DESC"}}, plan.Metadata.Order)
	assert.Equal(t, []string{"id", "username"}, plan.Metadata.Fields)
}

func TestAdapterContract_FilterSearchAliasAfterExplicitPredicates(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Fields.Resolver = NewMapResolver(map[string]string{
		"username": "username",
		"email":    "email",
		"status":   "status",
	})
	cfg.Search.Fields = []string{"username", "email"}

	plan, err := BuildQueryPlan(ListOptions{
		Filters: map[string]any{
			"search": "from-filter",
		},
		Predicates: []Predicate{
			{Field: "status", Operator: OperatorEq, Values: []string{"active"}},
		},
	}, cfg)
	require.NoError(t, err)
	assert.Equal(t, "from-filter", plan.Metadata.Search)
	assert.NotEmpty(t, plan.Filters)
	assert.NotEmpty(t, plan.Search)
}

func TestAdapterContract_SafeDynamicFieldsRejectUnsafeInput(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Fields.Resolver = SafeIdentifierResolver{}

	plan, err := BuildQueryPlan(ListOptions{
		Filters: map[string]any{
			"username":       "alice",
			"users.email":    "alice@example.com",
			"LOWER(status)":  "active",
			"status -- drop": "active",
		},
	}, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, plan.Filters)
	assert.ElementsMatch(t, []UnsupportedPredicate{
		{Field: "LOWER(status)", Operator: OperatorEq, RawKey: "LOWER(status)", RawValue: "active", Reason: UnsupportedDisallowedField},
		{Field: "status -- drop", Operator: OperatorEq, RawKey: "status -- drop", RawValue: "active", Reason: UnsupportedDisallowedField},
		{Field: "users.email", Operator: OperatorEq, RawKey: "users.email", RawValue: "alice@example.com", Reason: UnsupportedDisallowedField},
	}, plan.Unsupported)
}
