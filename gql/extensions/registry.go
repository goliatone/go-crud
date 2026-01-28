package extensions

import (
	"strings"
	"sync"

	"github.com/goliatone/go-router"
)

// SchemaExtensionContext provides mutable schema metadata for extension handlers.
type SchemaExtensionContext struct {
	Doc        map[string]any
	SchemaName string
	Meta       *router.SchemaMetadata
	NameMap    map[string]string
}

// SchemaExtensionHandler can mutate schema metadata based on custom extensions.
type SchemaExtensionHandler interface {
	Name() string
	ApplyDoc(doc map[string]any, ctx *SchemaExtensionContext)
	ApplySchema(schemaName string, rawSchema map[string]any, ctx *SchemaExtensionContext)
}

var registry = struct {
	mu       sync.RWMutex
	handlers []SchemaExtensionHandler
}{}

// RegisterSchemaExtension adds a handler to the registry.
func RegisterSchemaExtension(handler SchemaExtensionHandler) {
	if handler == nil {
		return
	}
	if strings.TrimSpace(handler.Name()) == "" {
		return
	}
	registry.mu.Lock()
	registry.handlers = append(registry.handlers, handler)
	registry.mu.Unlock()
}

// ListSchemaExtensions returns registered handlers in registration order.
func ListSchemaExtensions() []SchemaExtensionHandler {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if len(registry.handlers) == 0 {
		return nil
	}
	out := make([]SchemaExtensionHandler, len(registry.handlers))
	copy(out, registry.handlers)
	return out
}
