package querybun

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldResolvers_MapSafeAndComposed(t *testing.T) {
	mapResolver := NewMapResolver(map[string]string{
		"name": "users.name",
	})
	resolved, ok := mapResolver.ResolveField(FieldRequest{Field: "name", Use: FieldUseFilter})
	require.True(t, ok)
	assert.Equal(t, "users.name", resolved.SQLName())
	assert.True(t, resolved.Trusted)

	safe := SafeIdentifierResolver{}
	for _, unsafe := range []string{"user name", "name;drop", "name--", "LOWER(name)", `"name"`, "users.name"} {
		_, ok := safe.ResolveField(FieldRequest{Field: unsafe, Use: FieldUseOrder})
		assert.False(t, ok, unsafe)
	}

	resolved, ok = safe.ResolveField(FieldRequest{Field: "user_name", Use: FieldUseOrder})
	require.True(t, ok)
	assert.Equal(t, "user_name", resolved.SQLName())

	dotted := SafeIdentifierResolver{AllowDotted: true}
	resolved, ok = dotted.ResolveField(FieldRequest{Field: "users.name", Use: FieldUseOrder})
	require.True(t, ok)
	assert.Equal(t, "users.name", resolved.SQLName())

	composed := ComposeResolvers(mapResolver, safe)
	resolved, ok = composed.ResolveField(FieldRequest{Field: "name", Use: FieldUseSelect})
	require.True(t, ok)
	assert.Equal(t, "users.name", resolved.SQLName())
	resolved, ok = composed.ResolveField(FieldRequest{Field: "created_at", Use: FieldUseSelect})
	require.True(t, ok)
	assert.Equal(t, "created_at", resolved.SQLName())
}

func TestBuildFilterCriteriaFromPredicates_FieldValidationModes(t *testing.T) {
	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "missing", Operator: OperatorEq, Values: []string{"x"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, criteria)
	assert.Equal(t, []UnsupportedPredicate{
		{Field: "missing", Operator: OperatorEq, Reason: UnsupportedDisallowedField},
	}, unsupported)

	_, unsupported, err = BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "missing", Operator: OperatorEq, Values: []string{"x"}},
	}, filterConfig(func(cfg *Config) {
		cfg.Validation.Mode = ValidationStrict
		cfg.Validation.StrictFields = true
	}))
	require.Error(t, err)
	assert.Equal(t, []UnsupportedPredicate{
		{Field: "missing", Operator: OperatorEq, Reason: UnsupportedDisallowedField},
	}, unsupported)

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr))
	assert.Equal(t, ValidationFieldNotAllowed, validationErr.Code)
	assert.Equal(t, "missing", validationErr.Field)
}
