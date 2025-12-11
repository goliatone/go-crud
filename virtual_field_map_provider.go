package crud

import (
	"reflect"
)

// NewVirtualFieldMapProvider builds a FieldMapProvider that merges the base provider with virtual field expressions.
// It inspects model tags to discover virtual fields and emits dialect-aware expressions.
func NewVirtualFieldMapProvider(cfg VirtualFieldHandlerConfig, base FieldMapProvider) FieldMapProvider {
	cfg = normalizeVirtualConfig(cfg)
	return func(t reflect.Type) map[string]string {
		out := map[string]string{}
		if base != nil {
			if m := base(t); len(m) > 0 {
				for k, v := range m {
					out[k] = v
				}
			}
		}
		defs := extractVirtualFieldDefsForType(t, cfg.AllowZeroTag)
		for _, def := range defs {
			out[def.JSONName] = VirtualFieldExpr(cfg.Dialect, def.SourceField, def.JSONName, false)
		}
		return out
	}
}
