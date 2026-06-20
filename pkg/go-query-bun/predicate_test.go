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
		operator string
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
	assert.Equal(t, "or", predicates[0].Operator)
	assert.Equal(t, []string{"active", "pending"}, predicates[0].Values)
}

func TestResolveOperator(t *testing.T) {
	t.Run("canonical operators work with custom aliases", func(t *testing.T) {
		operator, err := ResolveOperator("gte", "age", Config{
			OperatorMap: map[string]string{"$eq": "="},
		})
		require.NoError(t, err)
		assert.Equal(t, Operator{Token: "gte", Canonical: "gte", SQL: ">="}, operator)
	})

	t.Run("custom alias maps to canonical operator", func(t *testing.T) {
		operator, err := ResolveOperator("$like", "name", Config{
			OperatorMap: map[string]string{"$like": "LIKE"},
		})
		require.NoError(t, err)
		assert.Equal(t, Operator{Token: "$like", Canonical: "like", SQL: "LIKE"}, operator)
	})

	t.Run("strict unsupported operator includes field and operator", func(t *testing.T) {
		_, err := ResolveOperator("unknown", "name", Config{StrictValidation: true})
		require.Error(t, err)

		var validationErr *ValidationError
		require.True(t, errors.As(err, &validationErr))
		assert.Equal(t, ValidationUnsupportedOperator, validationErr.Code)
		assert.Equal(t, "name", validationErr.Field)
		assert.Equal(t, "unknown", validationErr.Operator)
	})

	t.Run("non strict unsupported operator falls back to eq", func(t *testing.T) {
		operator, err := ResolveOperator("unknown", "name", Config{})
		require.NoError(t, err)
		assert.Equal(t, Operator{Token: "eq", Canonical: "eq", SQL: "="}, operator)
	})
}

func TestOperatorMapsAreCopied(t *testing.T) {
	defer SetDefaultOperatorMap(CanonicalOperatorMap())

	aliases := map[string]string{"$eq": "="}
	SetDefaultOperatorMap(aliases)
	aliases["$eq"] = "BAD"

	operator, err := ResolveOperator("$eq", "name", Config{})
	require.NoError(t, err)
	assert.Equal(t, Operator{Token: "$eq", Canonical: "eq", SQL: "="}, operator)

	defaults := DefaultOperatorMap()
	defaults["$eq"] = "BAD"

	operator, err = ResolveOperator("$eq", "name", Config{})
	require.NoError(t, err)
	assert.Equal(t, Operator{Token: "$eq", Canonical: "eq", SQL: "="}, operator)
}
