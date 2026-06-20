package querybun

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVirtualFieldExpr(t *testing.T) {
	assert.Equal(t, "metadata->>'author'", VirtualFieldExpr(VirtualDialectPostgres, "metadata", "author", false))
	assert.Equal(t, "metadata->'author'", VirtualFieldExpr(VirtualDialectPostgres, "metadata", "author", true))
	assert.Equal(t, "json_extract(metadata, '$.author')", VirtualFieldExpr(VirtualDialectSQLite, "metadata", "author", false))
}
