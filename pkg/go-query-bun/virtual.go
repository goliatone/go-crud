package querybun

import (
	"fmt"
	"strings"
)

const (
	VirtualDialectPostgres = "postgres"
	VirtualDialectSQLite   = "sqlite"
)

// VirtualFieldExpr returns a SQL snippet for the given dialect to access a virtual field.
// When asJSON is false, text extraction is used for comparisons and ordering.
// When asJSON is true, the raw JSON value is returned.
func VirtualFieldExpr(dialect, sourceField, key string, asJSON bool) string {
	switch strings.ToLower(dialect) {
	case VirtualDialectSQLite:
		return fmt.Sprintf("json_extract(%s, '$.%s')", sourceField, key)
	case VirtualDialectPostgres:
		fallthrough
	default:
		if asJSON {
			return fmt.Sprintf("%s->'%s'", sourceField, key)
		}
		return fmt.Sprintf("%s->>'%s'", sourceField, key)
	}
}
