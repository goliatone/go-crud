package crud

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func TestVirtualFieldExpression_Dialects(t *testing.T) {
	pg := VirtualFieldExpr(VirtualDialectPostgres, "metadata", "author", false)
	assert.Equal(t, "metadata->>'author'", pg)

	sqlite := VirtualFieldExpr(VirtualDialectSQLite, "metadata", "author", false)
	assert.Equal(t, "json_extract(metadata, '$.author')", sqlite)

	pgJSON := VirtualFieldExpr(VirtualDialectPostgres, "metadata", "author", true)
	assert.Equal(t, "metadata->'author'", pgJSON)
}

func TestBuildQueryCriteria_VirtualFieldFilterAndOrder(t *testing.T) {
	ctx := newMockRequest()
	ctx.queryMap = map[string]string{
		"author__ilike": "john",
		"order":         "author asc",
	}

	allowed := map[string]string{
		"author": VirtualFieldExpr(VirtualDialectSQLite, "metadata", "author", false),
	}

	criteria, _, err := BuildQueryCriteria[*TestUser](ctx, OpList, WithAllowedFields(allowed))
	require.NoError(t, err)

	sqldb, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	defer db.Close()

	q := db.NewSelect().Model((*TestUser)(nil))
	for _, c := range criteria {
		q = c(q)
	}

	sqlStr := q.String()
	assert.Contains(t, sqlStr, "json_extract(metadata, '$.author') ILIKE 'john'")
	assert.Contains(t, sqlStr, "ORDER BY json_extract(metadata, '$.author') ASC")
}

func TestBuildQueryCriteria_VirtualFieldInOperator(t *testing.T) {
	ctx := newMockRequest()
	ctx.queryMap = map[string]string{
		"author__in": "john,jane",
	}

	allowed := map[string]string{
		"author": VirtualFieldExpr(VirtualDialectSQLite, "metadata", "author", false),
	}

	criteria, _, err := BuildQueryCriteria[*TestUser](ctx, OpList, WithAllowedFields(allowed))
	require.NoError(t, err)

	sqldb, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	defer db.Close()

	q := db.NewSelect().Model((*TestUser)(nil))
	for _, c := range criteria {
		q = c(q)
	}

	sqlStr := q.String()
	assert.Contains(t, sqlStr, "json_extract(metadata, '$.author') IN ('john', 'jane')")
}
