package crud

import (
	persistence "github.com/goliatone/go-persistence-bun"
)

const (
	VirtualDialectPostgres = persistence.VirtualDialectPostgres
	VirtualDialectSQLite   = persistence.VirtualDialectSQLite
)

// VirtualFieldExpr returns a SQL snippet for the given dialect to access a virtual field.
// When asJSON is false, text extraction is used (suitable for comparisons/order-by).
// When asJSON is true, the raw JSON value is returned.
func VirtualFieldExpr(dialect, sourceField, key string, asJSON bool) string {
	return persistence.VirtualFieldExpr(dialect, sourceField, key, asJSON)
}

func buildVirtualFieldMapExpressions(defs []VirtualFieldDef, cfg VirtualFieldHandlerConfig) map[string]string {
	if len(defs) == 0 {
		return nil
	}
	dialect := cfg.Dialect
	if dialect == "" {
		dialect = VirtualDialectPostgres
	}
	out := make(map[string]string, len(defs))
	for _, def := range defs {
		out[def.JSONName] = VirtualFieldExpr(dialect, def.SourceField, def.JSONName, false)
	}
	return out
}
