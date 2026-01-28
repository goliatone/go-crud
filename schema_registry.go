package crud

import (
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goliatone/go-router"
)

// SchemaEntry holds the cached OpenAPI document for a controller.
type SchemaEntry struct {
	Resource  string         `json:"resource"`
	Plural    string         `json:"plural"`
	Document  map[string]any `json:"document"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// SchemaListener receives notifications whenever a schema entry changes.
type SchemaListener func(SchemaEntry)

type schemaRegistry struct {
	mu        sync.RWMutex
	entries   map[string]SchemaEntry
	listeners []SchemaListener
}

var globalSchemaRegistry = &schemaRegistry{
	entries: make(map[string]SchemaEntry),
}

func registerSchemaEntry(meta router.ResourceMetadata, doc map[string]any) {
	_ = RegisterSchemaEntry(SchemaEntry{
		Resource: meta.Name,
		Plural:   meta.PluralName,
		Document: doc,
	})
}

func (sr *schemaRegistry) upsert(entry SchemaEntry) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.entries[entry.Resource] = entry.clone()
	if len(sr.listeners) == 0 {
		return
	}
	for _, listener := range sr.listeners {
		listener(entry.clone())
	}
}

func (sr *schemaRegistry) list() []SchemaEntry {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	out := make([]SchemaEntry, 0, len(sr.entries))
	for _, entry := range sr.entries {
		out = append(out, entry.clone())
	}
	return out
}

func (sr *schemaRegistry) get(resource string) (SchemaEntry, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	entry, ok := sr.entries[resource]
	if !ok {
		return SchemaEntry{}, false
	}
	return entry.clone(), true
}

func (sr *schemaRegistry) addListener(listener SchemaListener) {
	if listener == nil {
		return
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.listeners = append(sr.listeners, listener)
}

func (entry SchemaEntry) clone() SchemaEntry {
	entry.Document = cloneSchemaDocument(entry.Document)
	return entry
}

func cloneSchemaDocument(doc map[string]any) map[string]any {
	if doc == nil {
		return nil
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil
	}
	var clone map[string]any
	if err := json.Unmarshal(raw, &clone); err != nil {
		return nil
	}
	return clone
}

type schemaExportConfig struct {
	indent string
}

// SchemaExportOption configures registry export output.
type SchemaExportOption func(*schemaExportConfig)

// WithSchemaExportIndent controls JSON indentation when exporting schema registry entries.
func WithSchemaExportIndent(indent string) SchemaExportOption {
	return func(cfg *schemaExportConfig) {
		cfg.indent = indent
	}
}

// RegisterSchemaEntry registers an OpenAPI document in the schema registry.
func RegisterSchemaEntry(entry SchemaEntry) bool {
	resource := strings.TrimSpace(entry.Resource)
	if resource == "" || len(entry.Document) == 0 {
		return false
	}
	entry.Resource = resource
	entry.Document = cloneSchemaDocument(entry.Document)
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now().UTC()
	} else {
		entry.UpdatedAt = entry.UpdatedAt.UTC()
	}
	globalSchemaRegistry.upsert(entry)
	return true
}

// RegisterSchemaDocument registers a projected OpenAPI document without a controller/router.
func RegisterSchemaDocument(resource, plural string, doc map[string]any) bool {
	return RegisterSchemaEntry(SchemaEntry{
		Resource: resource,
		Plural:   plural,
		Document: doc,
	})
}

// ExportSchemas writes the schema registry entries as JSON, sorted by resource name.
func ExportSchemas(w io.Writer, opts ...SchemaExportOption) error {
	if w == nil {
		return errors.New("schema export writer is nil")
	}

	entries := ListSchemas()
	if len(entries) == 0 {
		return errors.New("no schemas registered")
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Resource < entries[j].Resource
	})

	cfg := schemaExportConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}

	enc := json.NewEncoder(w)
	if cfg.indent != "" {
		enc.SetIndent("", cfg.indent)
	}
	return enc.Encode(entries)
}

// ListSchemas returns all registered schema documents.
func ListSchemas() []SchemaEntry {
	return globalSchemaRegistry.list()
}

// GetSchema retrieves the schema for the given resource name.
func GetSchema(resource string) (SchemaEntry, bool) {
	return globalSchemaRegistry.get(resource)
}

// RegisterSchemaListener subscribes to schema updates.
func RegisterSchemaListener(listener SchemaListener) {
	globalSchemaRegistry.addListener(listener)
}

// resetSchemaRegistry clears the registry; used only in tests.
func resetSchemaRegistry() {
	globalSchemaRegistry.mu.Lock()
	defer globalSchemaRegistry.mu.Unlock()
	globalSchemaRegistry.entries = make(map[string]SchemaEntry)
	globalSchemaRegistry.listeners = nil
}
