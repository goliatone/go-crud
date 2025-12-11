package hooks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/overlay"
)

const (
	OperationGet    = "get"
	OperationList   = "list"
	OperationCreate = "create"
	OperationUpdate = "update"
	OperationDelete = "delete"
)

// Options drive hook construction for templates.
type Options struct {
	Overlay     overlay.Hooks
	AuthPackage string
	AuthGuard   string
	AuthFail    string
}

// TemplateHooks is the template-friendly hook representation.
type TemplateHooks struct {
	Imports  []string
	Entities map[string]TemplateEntityHooks
}

// TemplateEntityHooks groups hooks by operation for an entity.
type TemplateEntityHooks struct {
	Get    TemplateHookSet
	List   TemplateHookSet
	Create TemplateHookSet
	Update TemplateHookSet
	Delete TemplateHookSet
}

// TemplateHookSet contains snippets injected into resolver operations.
type TemplateHookSet struct {
	AuthGuard    string
	ScopeGuard   string
	Preload      string
	WrapRepo     string
	ErrorHandler string
}

// Build constructs template hooks from overlay and CLI defaults.
func Build(doc formatter.Document, opts Options) TemplateHooks {
	defaultOps, baseImports := baseOperationHooks(opts)
	result := TemplateHooks{
		Imports:  dedupeStrings(baseImports),
		Entities: make(map[string]TemplateEntityHooks, len(doc.Entities)),
	}
	for _, entity := range doc.Entities {
		entityHooks := copyOperationHooks(defaultOps)
		if hooks, ok := findEntityHooks(opts.Overlay.Entities, entity); ok {
			if len(hooks.Imports) > 0 {
				result.Imports = append(result.Imports, hooks.Imports...)
			}
			result.Imports = append(result.Imports, hooks.All.Imports...)
			applyHookSetToAll(&entityHooks, hooks.All)
			for opName, set := range hooks.Operations {
				if len(set.Imports) > 0 {
					result.Imports = append(result.Imports, set.Imports...)
				}
				applyHook(&entityHooks, normalizeOp(opName), set)
			}
		}
		result.Entities[entity.Name] = entityHooks
	}

	result.Imports = dedupeStrings(result.Imports)
	sort.Strings(result.Imports)
	return result
}

func baseOperationHooks(opts Options) (TemplateEntityHooks, []string) {
	authDefaults, authImports := buildAuthDefaults(opts)
	imports := append([]string{}, opts.Overlay.Imports...)
	imports = append(imports, authImports...)
	imports = append(imports, opts.Overlay.Default.Imports...)
	imports = dedupeStrings(imports)

	applyHookSetToAll(&authDefaults, opts.Overlay.Default)

	return authDefaults, imports
}

func buildAuthDefaults(opts Options) (TemplateEntityHooks, []string) {
	if strings.TrimSpace(opts.AuthGuard) == "" {
		return TemplateEntityHooks{}, nil
	}

	failExpr := strings.TrimSpace(opts.AuthFail)
	imports := make([]string, 0, 2)
	if failExpr == "" {
		failExpr = `errors.New("unauthorized")`
		imports = append(imports, "errors")
	}

	if opts.AuthPackage != "" {
		imports = append(imports, opts.AuthPackage)
	}

	return TemplateEntityHooks{
		Get:    TemplateHookSet{AuthGuard: BuildAuthSnippet(OperationGet, opts.AuthGuard, failExpr)},
		List:   TemplateHookSet{AuthGuard: BuildAuthSnippet(OperationList, opts.AuthGuard, failExpr)},
		Create: TemplateHookSet{AuthGuard: BuildAuthSnippet(OperationCreate, opts.AuthGuard, failExpr)},
		Update: TemplateHookSet{AuthGuard: BuildAuthSnippet(OperationUpdate, opts.AuthGuard, failExpr)},
		Delete: TemplateHookSet{AuthGuard: BuildAuthSnippet(OperationDelete, opts.AuthGuard, failExpr)},
	}, imports
}

func findEntityHooks(entities map[string]overlay.EntityHooks, entity formatter.Entity) (overlay.EntityHooks, bool) {
	if len(entities) == 0 {
		return overlay.EntityHooks{}, false
	}

	if hooks, ok := entities[entity.Name]; ok {
		return hooks, true
	}
	if hooks, ok := entities[entity.RawName]; ok {
		return hooks, true
	}
	key := strings.ToLower(entity.RawName)
	for name, hooks := range entities {
		if strings.ToLower(name) == key {
			return hooks, true
		}
	}
	return overlay.EntityHooks{}, false
}

func copyOperationHooks(src TemplateEntityHooks) TemplateEntityHooks {
	return TemplateEntityHooks{
		Get:    src.Get,
		List:   src.List,
		Create: src.Create,
		Update: src.Update,
		Delete: src.Delete,
	}
}

func applyHookSetToAll(target *TemplateEntityHooks, set overlay.HookSet) {
	if target == nil {
		return
	}
	target.Get = mergeTemplateHooks(target.Get, toTemplateHookSet(set))
	target.List = mergeTemplateHooks(target.List, toTemplateHookSet(set))
	target.Create = mergeTemplateHooks(target.Create, toTemplateHookSet(set))
	target.Update = mergeTemplateHooks(target.Update, toTemplateHookSet(set))
	target.Delete = mergeTemplateHooks(target.Delete, toTemplateHookSet(set))
}

func applyHook(target *TemplateEntityHooks, op string, set overlay.HookSet) {
	if target == nil {
		return
	}

	switch op {
	case OperationGet:
		target.Get = mergeTemplateHooks(target.Get, toTemplateHookSet(set))
	case OperationList:
		target.List = mergeTemplateHooks(target.List, toTemplateHookSet(set))
	case OperationCreate:
		target.Create = mergeTemplateHooks(target.Create, toTemplateHookSet(set))
	case OperationUpdate:
		target.Update = mergeTemplateHooks(target.Update, toTemplateHookSet(set))
	case OperationDelete:
		target.Delete = mergeTemplateHooks(target.Delete, toTemplateHookSet(set))
	}
}

func mergeTemplateHooks(base TemplateHookSet, extra TemplateHookSet) TemplateHookSet {
	out := base
	if extra.AuthGuard != "" {
		out.AuthGuard = extra.AuthGuard
	}
	if extra.ScopeGuard != "" {
		out.ScopeGuard = extra.ScopeGuard
	}
	if extra.Preload != "" {
		out.Preload = extra.Preload
	}
	if extra.WrapRepo != "" {
		out.WrapRepo = extra.WrapRepo
	}
	if extra.ErrorHandler != "" {
		out.ErrorHandler = extra.ErrorHandler
	}
	return out
}

func toTemplateHookSet(set overlay.HookSet) TemplateHookSet {
	return TemplateHookSet{
		AuthGuard:    strings.TrimSpace(set.AuthGuard),
		ScopeGuard:   strings.TrimSpace(set.ScopeGuard),
		Preload:      strings.TrimSpace(set.Preload),
		WrapRepo:     strings.TrimSpace(set.WrapRepo),
		ErrorHandler: strings.TrimSpace(set.ErrorHandler),
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		key := strings.TrimSpace(v)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func normalizeOp(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "get", "show":
		return OperationGet
	case "list", "index":
		return OperationList
	case "create":
		return OperationCreate
	case "update":
		return OperationUpdate
	case "delete", "destroy", "remove":
		return OperationDelete
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

// BuildAuthSnippet composes a full auth guard snippet (with return statements) for a given operation.
func BuildAuthSnippet(op, authGuard, authFail string) string {
	op = normalizeOp(op)
	authGuard = strings.TrimSpace(authGuard)
	authFail = strings.TrimSpace(authFail)
	if authGuard == "" || authFail == "" {
		return ""
	}
	first := "nil"
	if op == OperationDelete {
		first = "false"
	}
	return fmt.Sprintf(`user, ok := %s
if !ok || user == nil {
	return %s, %s
}
_ = user`, authGuard, first, authFail)
}
