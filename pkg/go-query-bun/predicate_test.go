package querybun

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePredicateKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		field    string
		operator OperatorKind
	}{
		{name: "default operator", key: "age", field: "age", operator: "eq"},
		{name: "explicit operator", key: "path__ilike", field: "path", operator: "ilike"},
		{name: "trim and lower operator", key: " name__GTE ", field: "name", operator: "gte"},
		{name: "empty operator", key: "email__", field: "email", operator: "eq"},
		{name: "empty field", key: "__eq", field: "", operator: "eq"},
		{name: "split once", key: "user_name__like", field: "user_name", operator: "like"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, operator := ParsePredicateKey(tt.key)
			assert.Equal(t, tt.field, field)
			assert.Equal(t, tt.operator, operator)
		})
	}
}

func TestNormalizePredicates(t *testing.T) {
	predicates := NormalizePredicates(ListOptions{
		Filters: map[string]any{
			"name__ilike": " alice, bob ",
			"age__gte":    30,
			"active":      true,
			"ignored":     map[string]string{"bad": "shape"},
		},
	})

	assert.ElementsMatch(t, []Predicate{
		{Field: "name", Operator: "ilike", Values: []string{"alice", "bob"}, RawKey: "name__ilike", RawValue: " alice, bob "},
		{Field: "age", Operator: "gte", Values: []string{"30"}, RawKey: "age__gte", RawValue: 30},
		{Field: "active", Operator: "eq", Values: []string{"true"}, RawKey: "active", RawValue: true},
	}, predicates)
}

func TestNormalizePredicatesExplicitPredicatesTakePrecedence(t *testing.T) {
	predicates := NormalizePredicates(ListOptions{
		Filters: map[string]any{"name": "ignored"},
		Predicates: []Predicate{
			{Field: "status", Operator: "or", Values: []string{"active,pending"}},
		},
	})

	require.Len(t, predicates, 1)
	assert.Equal(t, "status", predicates[0].Field)
	assert.Equal(t, OperatorOr, predicates[0].Operator)
	assert.Equal(t, []string{"active", "pending"}, predicates[0].Values)
}

func TestNormalizePredicates_SyntaxDecoders(t *testing.T) {
	predicates, unsupported := NormalizePredicatesWithUnsupported(ListOptions{
		Filters: map[string]any{
			"age__gte": "30",
			"status": map[string]any{
				"$in": []string{"active", "pending"},
			},
		},
	}, DefaultConfig())

	assert.Empty(t, unsupported)
	assert.ElementsMatch(t, []Predicate{
		{Field: "age", Operator: OperatorGte, Values: []string{"30"}, RawKey: "age__gte", RawValue: "30"},
		{Field: "status", Operator: OperatorIn, Values: []string{"active", "pending"}, RawKey: "status.$in", RawValue: []string{"active", "pending"}},
	}, predicates)
}

func TestNormalizePredicates_CustomDelimiterAndAlias(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Syntax.PredicateDelimiter = "::"
	cfg.Syntax.OperatorAliases["above"] = OperatorGt

	predicates := NormalizePredicates(ListOptions{
		Filters: map[string]any{"age::above": 30},
	}, cfg)

	require.Len(t, predicates, 1)
	assert.Equal(t, Predicate{Field: "age", Operator: OperatorGt, Values: []string{"30"}, RawKey: "age::above", RawValue: 30}, predicates[0])
}

func TestNormalizePredicates_NestedLogicalShapeUnsupported(t *testing.T) {
	predicates, unsupported := NormalizePredicatesWithUnsupported(ListOptions{
		Filters: map[string]any{
			"$or": []any{
				map[string]any{"name": "Alice"},
				map[string]any{"status": "active"},
			},
		},
	}, DefaultConfig())

	assert.Empty(t, predicates)
	assert.Equal(t, []UnsupportedPredicate{
		{Field: "$or", Operator: "$or", RawKey: "$or", RawValue: []any{map[string]any{"name": "Alice"}, map[string]any{"status": "active"}}, Reason: UnsupportedValueShape},
	}, unsupported)
}

func TestResolveOperator(t *testing.T) {
	t.Run("canonical operators work with custom aliases", func(t *testing.T) {
		operator, err := ResolveOperator("gte", "age", Config{
			Operators: DefaultOperatorSet().WithAlias("$equals", OperatorEq),
		})
		require.NoError(t, err)
		assert.Equal(t, "gte", operator.Token)
		assert.Equal(t, OperatorGte, operator.Canonical)
		assert.Equal(t, ">=", operator.SQL)
	})

	t.Run("custom alias maps to canonical operator", func(t *testing.T) {
		operator, err := ResolveOperator("$like", "name", Config{
			Operators: DefaultOperatorSet().WithAlias("$matches", OperatorLike),
		})
		require.NoError(t, err)
		assert.Equal(t, "$like", operator.Token)
		assert.Equal(t, OperatorLike, operator.Canonical)
		assert.Equal(t, "LIKE", operator.SQL)

		operator, err = ResolveOperator("$matches", "name", Config{
			Operators: DefaultOperatorSet().WithAlias("$matches", OperatorLike),
		})
		require.NoError(t, err)
		assert.Equal(t, OperatorLike, operator.Canonical)
	})

	t.Run("strict unsupported operator includes field and operator", func(t *testing.T) {
		_, err := ResolveOperator("unknown", "name", Config{Validation: ValidationConfig{Mode: ValidationStrict}})
		require.Error(t, err)

		var validationErr *ValidationError
		require.True(t, errors.As(err, &validationErr))
		assert.Equal(t, ValidationUnsupportedOperator, validationErr.Code)
		assert.Equal(t, "name", validationErr.Field)
		assert.Equal(t, "unknown", validationErr.Operator)
	})

	t.Run("unsupported operator never falls back to eq", func(t *testing.T) {
		operator, err := ResolveOperator("unknown", "name", Config{})
		require.Error(t, err)
		assert.Empty(t, operator.Token)
	})
}

func TestOperatorSetsAreCopied(t *testing.T) {
	base := DefaultOperatorSet()
	withAlias := base.WithAlias("$same", OperatorEq)

	_, ok := base.Resolve("$same")
	assert.False(t, ok)

	operator, err := ResolveOperator("$same", "name", Config{Operators: withAlias})
	require.NoError(t, err)
	assert.Equal(t, OperatorEq, operator.Canonical)
}

func TestResolveOperator_PerPlanAliases(t *testing.T) {
	cfg := Config{Operators: DefaultOperatorSet().WithAlias("$matches", OperatorLike)}

	operator, err := ResolveOperator("$matches", "name", cfg)
	require.NoError(t, err)
	assert.Equal(t, OperatorLike, operator.Canonical)

	operator, err = ResolveOperator("nin", "status", cfg)
	require.NoError(t, err)
	assert.Equal(t, OperatorNin, operator.Canonical)
	assert.Equal(t, "NOT IN", operator.SQL)
}
