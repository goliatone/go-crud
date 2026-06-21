package querybun

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type filterUser struct {
	bun.BaseModel `bun:"table:filter_users,alias:u"`

	ID     int    `bun:"id,pk" json:"id"`
	Name   string `bun:"name" json:"name"`
	Age    int    `bun:"age" json:"age"`
	Status string `bun:"status" json:"status"`
}

func TestBuildFilterCriteriaFromPredicates_FilterOnly(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "age", Operator: "gte", Values: []string{"30"}},
		{Field: "status", Operator: "in", Values: []string{"active", "pending"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, unsupported)
	require.NotEmpty(t, criteria)

	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"Alice", "Carol"}, []string{rows[0].Name, rows[1].Name})
}

func TestBuildFilterCriteriaFromPredicates_AndOrOperators(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "status", Operator: "or", Values: []string{"active", "pending"}},
		{Field: "name", Operator: "and", Values: []string{"Alice"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, unsupported)

	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 1)
	assert.Equal(t, "Alice", rows[0].Name)
}

func TestBuildFilterCriteriaFromPredicates_SemanticOperators(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "status", Operator: "nin", Values: []string{"active", "pending"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, unsupported)
	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 1)
	assert.Equal(t, "Bob", rows[0].Name)

	criteria, unsupported, err = BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "status", Operator: "$neq", Values: []string{"inactive"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, unsupported)
	rows = executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"Alice", "Carol"}, []string{rows[0].Name, rows[1].Name})
}

func TestBuildFilterCriteriaFromPredicates_ContainsEscapesLiterals(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)
	_, err := db.NewInsert().Model(&filterUser{ID: 4, Name: "A%ice", Age: 50, Status: "active"}).Exec(context.Background())
	require.NoError(t, err)

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "name", Operator: "contains", Values: []string{"A%"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, unsupported)

	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 1)
	assert.Equal(t, "A%ice", rows[0].Name)
}

func TestBuildFilterCriteriaFromPredicates_RawLikeAndILike(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)
	_, err := db.NewInsert().Model(&filterUser{ID: 4, Name: "A%ice", Age: 50, Status: "active"}).Exec(context.Background())
	require.NoError(t, err)

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "name", Operator: "like", Values: []string{"A%"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, unsupported)
	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"Alice", "A%ice"}, []string{rows[0].Name, rows[1].Name})

	criteria, unsupported, err = BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "name", Operator: "ilike", Values: []string{"AL%"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, unsupported)
	rows = executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 1)
	assert.Equal(t, "Alice", rows[0].Name)
}

func TestBuildFilterCriteriaFromPredicates_ContainsMultiValueUsesOr(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "name", Operator: "contains", Values: []string{"li", "ro"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, unsupported)

	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"Alice", "Carol"}, []string{rows[0].Name, rows[1].Name})
}

func TestBuildFilterCriteriaFromPredicates_CustomFieldBuilder(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	cfg := filterConfig(func(cfg *Config) {
		cfg.Predicates.Fields = map[string]PredicateBuilder{
			"app_role": PredicateBuilderFunc(func(in PredicateBuilderInput) (PredicateBuilderResult, error) {
				if in.Operator.Canonical != OperatorEq {
					return PredicateUnhandled(), nil
				}
				return PredicateHandled(in.Helpers.Compare("status", "=", in.Values)), nil
			}),
		}
	})

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "app_role", Operator: OperatorEq, Values: []string{"active"}},
	}, cfg)
	require.NoError(t, err)
	assert.Empty(t, unsupported)

	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 1)
	assert.Equal(t, "Alice", rows[0].Name)
}

func TestBuildFilterCriteriaFromPredicates_CustomOperatorBuilderPrecedesBuiltin(t *testing.T) {
	db := setupQueryBunDB(t)
	seedFilterUsers(t, db)

	cfg := filterConfig(func(cfg *Config) {
		cfg.Predicates.Operators = map[OperatorKind]PredicateBuilder{
			OperatorEq: PredicateBuilderFunc(func(in PredicateBuilderInput) (PredicateBuilderResult, error) {
				return PredicateHandled(in.Helpers.Compare(in.Field.SQLName(), "<>", in.Values)), nil
			}),
		}
	})

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "status", Operator: OperatorEq, Values: []string{"inactive"}},
	}, cfg)
	require.NoError(t, err)
	assert.Empty(t, unsupported)

	rows := executeFilterUserCriteria(t, db, criteria)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"Alice", "Carol"}, []string{rows[0].Name, rows[1].Name})
}

func TestBuildFilterCriteriaFromPredicates_UnhandledCustomBuilderMetadata(t *testing.T) {
	cfg := filterConfig(func(cfg *Config) {
		cfg.Predicates.Fields = map[string]PredicateBuilder{
			"app_role": PredicateBuilderFunc(func(PredicateBuilderInput) (PredicateBuilderResult, error) {
				return PredicateUnhandled(), nil
			}),
		}
	})

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "app_role", Operator: OperatorEq, Values: []string{"active"}},
	}, cfg)
	require.NoError(t, err)
	assert.Empty(t, criteria)
	assert.Equal(t, []UnsupportedPredicate{
		{Field: "app_role", Operator: OperatorEq, Reason: UnsupportedUnhandledPredicate},
	}, unsupported)
}

func TestBuildFilterCriteriaFromPredicates_UnsupportedMetadata(t *testing.T) {
	predicates := []Predicate{
		{Field: "missing", Operator: "eq", Values: []string{"value"}, RawKey: "missing", RawValue: "value"},
		{Field: "name", Operator: "unknown", Values: []string{"Alice"}, RawKey: "name__unknown", RawValue: "Alice"},
		{Field: "status", Operator: "eq", Values: nil, RawKey: "status", RawValue: ""},
	}

	criteria, unsupported, err := BuildFilterCriteriaFromPredicates(predicates, Config{
		Fields: FieldConfig{Resolver: NewMapResolver(filterAllowedFields())},
	})
	require.NoError(t, err)
	assert.Empty(t, criteria)
	assert.ElementsMatch(t, []UnsupportedPredicate{
		{Field: "missing", Operator: "eq", RawKey: "missing", RawValue: "value", Reason: UnsupportedDisallowedField},
		{Field: "name", Operator: "unknown", RawKey: "name__unknown", RawValue: "Alice", Reason: UnsupportedOperator},
		{Field: "status", Operator: "eq", RawKey: "status", RawValue: "", Reason: UnsupportedEmptyValue},
	}, unsupported)
}

func TestBuildFilterCriteriaFromPredicates_StrictUnsupportedOperator(t *testing.T) {
	_, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "name", Operator: "unknown", Values: []string{"Alice"}, RawKey: "name__unknown", RawValue: "Alice"},
	}, filterConfig(func(cfg *Config) {
		cfg.Validation.Mode = ValidationStrict
	}))
	require.Error(t, err)
	assert.Equal(t, []UnsupportedPredicate{
		{Field: "name", Operator: "unknown", RawKey: "name__unknown", RawValue: "Alice", Reason: UnsupportedOperator},
	}, unsupported)

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr))
	assert.Equal(t, ValidationUnsupportedOperator, validationErr.Code)
	assert.Equal(t, "name", validationErr.Field)
	assert.Equal(t, "unknown", validationErr.Operator)
}

func TestNormalizePredicatesWithUnsupported_ValueShapes(t *testing.T) {
	predicates, unsupported := NormalizePredicatesWithUnsupported(ListOptions{
		Filters: map[string]any{
			"name":   "Alice",
			"bad":    map[string]string{"value": "shape"},
			"empty":  "  ",
			"__oops": "missing field",
		},
	})

	assert.Equal(t, []Predicate{
		{Field: "name", Operator: "eq", Values: []string{"Alice"}, RawKey: "name", RawValue: "Alice"},
	}, predicates)
	assert.ElementsMatch(t, []UnsupportedPredicate{
		{Field: "bad", Operator: "eq", RawKey: "bad", RawValue: map[string]string{"value": "shape"}, Reason: UnsupportedValueShape},
		{Field: "empty", Operator: "eq", RawKey: "empty", RawValue: "  ", Reason: UnsupportedEmptyValue},
		{Field: "", Operator: "oops", RawKey: "__oops", RawValue: "missing field", Reason: UnsupportedUnknownField},
	}, unsupported)
}

func TestBuildFilterCriteriaFromPredicates_UnknownOperatorDoesNotFallbackToEquality(t *testing.T) {
	criteria, unsupported, err := BuildFilterCriteriaFromPredicates([]Predicate{
		{Field: "name", Operator: "unknown", Values: []string{"Alice"}},
	}, filterConfig())
	require.NoError(t, err)
	assert.Empty(t, criteria)
	assert.Equal(t, []UnsupportedPredicate{
		{Field: "name", Operator: "unknown", Reason: UnsupportedOperator},
	}, unsupported)
}

func setupQueryBunDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func seedFilterUsers(t *testing.T, db *bun.DB) {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, db.ResetModel(ctx, (*filterUser)(nil)))

	users := []filterUser{
		{ID: 1, Name: "Alice", Age: 30, Status: "active"},
		{ID: 2, Name: "Bob", Age: 25, Status: "inactive"},
		{ID: 3, Name: "Carol", Age: 40, Status: "pending"},
	}
	for _, user := range users {
		_, err := db.NewInsert().Model(&user).Exec(ctx)
		require.NoError(t, err)
	}
}

func executeFilterUserCriteria(t *testing.T, db *bun.DB, criteria []Criteria) []filterUser {
	t.Helper()

	var rows []filterUser
	query := db.NewSelect().Model(&rows)
	for _, criterion := range criteria {
		query = criterion(query)
	}
	require.NoError(t, query.Order("id ASC").Scan(context.Background()))
	return rows
}

func filterAllowedFields() map[string]string {
	return map[string]string{
		"id":     "id",
		"name":   "name",
		"age":    "age",
		"status": "status",
	}
}

func filterConfig(mutators ...func(*Config)) Config {
	cfg := DefaultConfig()
	cfg.Fields.Resolver = NewMapResolver(filterAllowedFields())
	for _, mutate := range mutators {
		mutate(&cfg)
	}
	return cfg
}
