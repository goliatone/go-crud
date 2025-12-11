package dataloader

// {{ Notice }}

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/uptrace/bun"

	"{{ ModelPackage }}"
)

type contextKey struct{}

// Services bundles the CRUD services used by dataloaders.
type Services struct {
{% for entity in DataloaderEntities %}	{{ entity.Name }} crud.Service[model.{{ entity.Name }}]
{% endfor %}}

// Option configures a Loader.
type Option func(*Loader)

// WithDB provides a bun database or transaction for pivot-aware joins.
func WithDB(db bun.IDB) Option {
	return func(l *Loader) {
		l.db = db
	}
}

// WithContextFactory sets a factory used to build crud.Context values from request contexts.
func WithContextFactory(factory func(context.Context) crud.Context) Option {
	return func(l *Loader) {
		l.contextFactory = factory
	}
}

// Loader batches lookups to avoid N+1 database queries.
type Loader struct {
	services       Services
	db             bun.IDB
	contextFactory func(context.Context) crud.Context

{% for entity in DataloaderEntities %}	{{ entity.Name }}ByID *entityLoader[model.{{ entity.Name }}]
{% for rel in entity.Relations %}{% if rel.RelationType != "belongsTo" %}	{{ entity.Name }}{{ rel.FieldName }} *groupLoader[model.{{ rel.Target }}]
{% endif %}{% endfor %}{% endfor %}}

// New builds a Loader backed by the provided services.
func New(services Services, opts ...Option) *Loader {
	loader := &Loader{
		services: services,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(loader)
		}
	}

{% for entity in DataloaderEntities %}	loader.{{ entity.Name }}ByID = newEntityLoader(func(ctx context.Context, keys []string) (map[string]*model.{{ entity.Name }}, error) {
		return loader.fetch{{ entity.Name }}ByID(ctx, loader.crudContext(ctx), loader.services.{{ entity.Name }}, "{{ entity.PK.Column }}", keys)
	})
{% for rel in entity.Relations %}{% if rel.RelationType != "belongsTo" %}	loader.{{ entity.Name }}{{ rel.FieldName }} = newGroupLoader(func(ctx context.Context, keys []string) (map[string][]*model.{{ rel.Target }}, error) {
		return loader.fetch{{ entity.Name }}{{ rel.FieldName }}(ctx, keys)
	})
{% endif %}{% endfor %}{% endfor %}	return loader
}

// Inject stores the loader in the provided context.
func Inject(ctx context.Context, loader *Loader) context.Context {
	return context.WithValue(ctx, contextKey{}, loader)
}

// FromContext extracts a loader if one was previously injected.
func FromContext(ctx context.Context) (*Loader, bool) {
	if ctx == nil {
		return nil, false
	}
	loader, ok := ctx.Value(contextKey{}).(*Loader)
	return loader, ok
}

func (l *Loader) crudContext(ctx context.Context) crud.Context {
	if l.contextFactory != nil {
		return l.contextFactory(ctx)
	}
	return nil
}

{% for entity in DataloaderEntities %}
func (l *Loader) fetch{{ entity.Name }}ByID(ctx context.Context, crudCtx crud.Context, svc crud.Service[model.{{ entity.Name }}], column string, keys []string) (map[string]*model.{{ entity.Name }}, error) {
	result := make(map[string]*model.{{ entity.Name }}, len(keys))
	dedup := uniqueKeys(keys)
	if len(dedup) == 0 {
		return result, nil
	}

	records, _, err := svc.Index(crudCtx, []repository.SelectCriteria{
		repository.SelectColumnIn(column, dedup),
	})
	if err != nil {
		return nil, err
	}

	for i := range records {
		record := records[i]
		result[stringValue(record.{{ entity.PK.FieldName }})] = &record
	}

	return result, nil
}

{% for rel in entity.Relations %}{% if rel.RelationType == "hasMany" or rel.RelationType == "hasOne" %}
func (l *Loader) fetch{{ entity.Name }}{{ rel.FieldName }}(ctx context.Context, keys []string) (map[string][]*model.{{ rel.Target }}, error) {
	result := make(map[string][]*model.{{ rel.Target }}, len(keys))
	dedup := uniqueKeys(keys)
	if len(dedup) == 0 {
		return result, nil
	}

	records, _, err := l.services.{{ rel.Target }}.Index(l.crudContext(ctx), []repository.SelectCriteria{
		repository.SelectColumnIn("{{ rel.TargetColumn }}", dedup),
	})
	if err != nil {
		return nil, err
	}

	for _, key := range dedup {
		result[key] = result[key]
	}

	for i := range records {
		record := records[i]
		group := stringValue(record.{{ rel.TargetFieldKey.FieldName }})
		result[group] = append(result[group], &record)
	}

	for key := range result {
		sort.SliceStable(result[key], func(i, j int) bool {
			left := result[key][i]
			right := result[key][j]
			return stringValue(left.{{ rel.TargetFieldKey.FieldName }}) < stringValue(right.{{ rel.TargetFieldKey.FieldName }})
		})
	}

	return result, nil
}

{% elif rel.RelationType == "manyToMany" %}
func (l *Loader) fetch{{ entity.Name }}{{ rel.FieldName }}(ctx context.Context, keys []string) (map[string][]*model.{{ rel.Target }}, error) {
	result := make(map[string][]*model.{{ rel.Target }}, len(keys))
	dedup := uniqueKeys(keys)
	if len(dedup) == 0 {
		return result, nil
	}
	if l.db == nil {
		return nil, fmt.Errorf("dataloader: db is required to load relation {{ entity.Name }}.{{ rel.Name }}")
	}

	links, err := fetchPivotLinks(ctx, l.db, "{{ rel.PivotTable }}", "{{ rel.SourcePivot }}", "{{ rel.TargetPivot }}", dedup)
	if err != nil {
		return nil, err
	}

	targetIDs := make([]string, 0, len(links))
	for _, link := range links {
		targetIDs = append(targetIDs, link.Target)
	}

	loaded, err := l.{{ rel.Target }}ByID.LoadMany(ctx, targetIDs)
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		item := loaded[link.Target]
		if item == nil {
			continue
		}
		result[link.Source] = append(result[link.Source], item)
	}

	for _, key := range dedup {
		result[key] = result[key]
	}

	for key := range result {
		sort.SliceStable(result[key], func(i, j int) bool {
			left := result[key][i]
			right := result[key][j]
			return stringValue(left.{{ rel.TargetFieldKey.FieldName }}) < stringValue(right.{{ rel.TargetFieldKey.FieldName }})
		})
	}

	return result, nil
}

{% endif %}{% endfor %}{% endfor %}type pivotLink struct {
	Source string `bun:"source_id"`
	Target string `bun:"target_id"`
}

func fetchPivotLinks(ctx context.Context, db bun.IDB, table, sourceColumn, targetColumn string, keys []string) ([]pivotLink, error) {
	rows := make([]pivotLink, 0, len(keys))
	if len(keys) == 0 {
		return rows, nil
	}

	err := db.NewSelect().
		Table(table).
		ColumnExpr("? AS source_id", bun.Ident(sourceColumn)).
		ColumnExpr("? AS target_id", bun.Ident(targetColumn)).
		Where(fmt.Sprintf("%s IN (?)", bun.Ident(sourceColumn)), bun.In(uniqueKeys(keys))).
		Scan(ctx, &rows)

	return rows, err
}

type entityLoader[T any] struct {
	fetch func(context.Context, []string) (map[string]*T, error)
	mu    sync.Mutex
	cache map[string]*T
}

func newEntityLoader[T any](fetch func(context.Context, []string) (map[string]*T, error)) *entityLoader[T] {
	return &entityLoader[T]{
		fetch: fetch,
		cache: make(map[string]*T),
	}
}

func (l *entityLoader[T]) Load(ctx context.Context, key string) (*T, error) {
	items, err := l.LoadMany(ctx, []string{key})
	if err != nil {
		return nil, err
	}
	return items[key], nil
}

func (l *entityLoader[T]) LoadMany(ctx context.Context, keys []string) (map[string]*T, error) {
	result := make(map[string]*T, len(keys))
	missing := make([]string, 0, len(keys))
	dedup := uniqueKeys(keys)

	l.mu.Lock()
	for _, key := range dedup {
		if val, ok := l.cache[key]; ok {
			result[key] = val
			continue
		}
		missing = append(missing, key)
	}
	l.mu.Unlock()

	if len(missing) > 0 {
		fetched, err := l.fetch(ctx, missing)
		if err != nil {
			return nil, err
		}
		l.mu.Lock()
		for key, val := range fetched {
			l.cache[key] = val
			result[key] = val
		}
		l.mu.Unlock()
	}

	for _, key := range dedup {
		if _, ok := result[key]; !ok {
			result[key] = nil
		}
	}

	return result, nil
}

type groupLoader[T any] struct {
	fetch func(context.Context, []string) (map[string][]*T, error)
	mu    sync.Mutex
	cache map[string][]*T
}

func newGroupLoader[T any](fetch func(context.Context, []string) (map[string][]*T, error)) *groupLoader[T] {
	return &groupLoader[T]{
		fetch: fetch,
		cache: make(map[string][]*T),
	}
}

func (l *groupLoader[T]) Load(ctx context.Context, key string) ([]*T, error) {
	items, err := l.LoadMany(ctx, []string{key})
	if err != nil {
		return nil, err
	}
	return items[key], nil
}

func (l *groupLoader[T]) LoadMany(ctx context.Context, keys []string) (map[string][]*T, error) {
	result := make(map[string][]*T, len(keys))
	missing := make([]string, 0, len(keys))
	dedup := uniqueKeys(keys)

	l.mu.Lock()
	for _, key := range dedup {
		if val, ok := l.cache[key]; ok {
			result[key] = val
			continue
		}
		missing = append(missing, key)
	}
	l.mu.Unlock()

	if len(missing) > 0 {
		fetched, err := l.fetch(ctx, missing)
		if err != nil {
			return nil, err
		}

		l.mu.Lock()
		for key, val := range fetched {
			l.cache[key] = val
			result[key] = val
		}
		l.mu.Unlock()
	}

	for _, key := range dedup {
		if _, ok := result[key]; !ok {
			result[key] = nil
		}
	}

	return result, nil
}

func uniqueKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}

func stringValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case *string:
		if v == nil {
			return ""
		}
		return *v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}
