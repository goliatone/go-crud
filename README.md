# Go CRUD Controller

A Go package that provides a generic CRUD controller for REST APIs using Fiber and Bun ORM.

## Installation

```bash
go get github.com/goliatone/go-crud
```

## Quick Start

```go
package main

import (
	"database/sql"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:cmp"`
	ID            *uuid.UUID `bun:"id,pk,nullzero,type:uuid" json:"id"`
	Name          string     `bun:"name,notnull" json:"name"`
	Email         string     `bun:"email,notnull" json:"email"`
	Password      string     `bun:"password,notnull" json:"password" crud:"-"`
	DeletedAt     *time.Time `bun:"deleted_at,soft_delete,nullzero" json:"deleted_at,omitempty"`
	CreatedAt     *time.Time `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     *time.Time `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}


func NewUserRepository(db bun.IDB) repository.Repository[*User] {
	handlers := repository.ModelHandlers[*User]{
		NewRecord: func() *User {
			return &User{}
		},
		GetID: func(record *User) uuid.UUID {
			return *record.ID
		},
		SetID: func(record *User, id uuid.UUID) {
			record.ID = &id
		},
		GetIdentifier: func() string {
			return "email"
		},
	}
	return repository.NewRepository[*User](db, handlers)
}

func main() {
	_ = sqlite3.Version() // ensure driver is imported
	sqldb, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		panic(err)
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())

	server := fiber.New()
	api := server.Group("/api/v1")

	userRepo := NewUserRepository(db)
	crud.NewController[*User](userRepo).RegisterRoutes(api)

log.Fatal(server.Listen(":3000"))
}

```

### Virtual Attributes (virtual fields)

Virtual attributes let you expose computed or denormalized fields as top-level API fields while storing them in a backing map (e.g., `metadata`). Tag the virtual field with `crud:"virtual:<mapField>"` and `bun:"-"`; the `VirtualFieldHandler` automatically moves values between the struct field and the map on save/load.

```go
type Article struct {
    bun.BaseModel `bun:"table:articles"`
    ID            uuid.UUID      `bun:"id,pk,type:uuid" json:"id"`
    Title         string         `bun:"title" json:"title"`
    Metadata      map[string]any `bun:"metadata,type:jsonb" json:"metadata,omitempty"` // storage

    ReadTime *int `bun:"-" json:"read_time,omitempty" crud:"virtual:metadata"` // virtual
}

vf := crud.NewVirtualFieldHandler[*Article]() // discovers virtuals via tags

// Attach via controller or service factory (REST + GraphQL share the same handler)
crud.NewController(articleRepo, crud.WithVirtualFields(vf))
// or:
crud.NewService(crud.ServiceConfig[*Article]{Repository: articleRepo, VirtualFields: vf})
```

Pointers are recommended for virtual fields so presence can be distinguished from zero values; unknown metadata keys remain intact.

### GraphQL package (shared service layer)

The `gql` module reuses the same service layer as REST so hooks, scope guards, field policies, validation, activity, and virtual attributes all apply uniformly.

- `gql/helpers.GraphQLToCrudContext` adapts gqlgen contexts to `crud.Context`.
- `gql/internal/templates` + `gql/cmd/graphqlgen` generate schema, gqlgen config, resolvers, and dataloaders. Auth is opt‑in via `--auth-package`/`--auth-guard`, which emits a `GraphQLContext` helper that bridges go-auth actors into `crud.ActorContext`.
- Example regeneration:
  ```bash
  go run ./gql/cmd/graphqlgen \
    --schema-package ./examples/relationships-gql/registrar \
    --out ./examples/relationships-gql/graph \
    --config ./examples/relationships-gql/gqlgen.yml \
    --emit-subscriptions --emit-dataloader \
    --auth-package github.com/goliatone/go-auth \
    --auth-guard "auth.FromContext(ctx)"
  ```

### Generated Routes

For a `User` struct, the following routes are automatically created:

```
GET    /user/schema       - Get the OpenAPI bundle for this resource
GET    /user/:id          - Get a single user
GET    /users             - List users (with pagination, filtering, ordering)
POST   /user              - Create a user
POST   /user/batch        - Create multiple users
PUT    /user/:id          - Update a user
PUT    /user/batch        - Update multiple users
DELETE /user/:id          - Delete a user
DELETE /user/batch        - Delete multiple users
```

### Resource Naming Convention

The controller automatically generates resource names following these rules:

1. If a `crud:"resource:name"` tag is present, it uses that name:
   ```go
   type Company struct {
       bun.BaseModel `bun:"table:companies" crud:"resource:organization"`
       // This generates /organization and /organizations endpoints
   }
   ```

2. Otherwise, it converts the struct name to kebab-case and handles pluralization:
   - `UserProfile` becomes `/user-profile` (singular) and `/user-profiles` (plural)
   - `Company` becomes `/company` and `/companies`
   - `BusinessAddress` becomes `/business-address` and `/business-addresses`

The package uses proper pluralization rules, handling common irregular cases correctly:
- `Person` → `/person` and `/people`
- `Category` → `/category` and `/categories`
- `Bus` → `/bus` and `/buses`

### Schema Endpoint Output

Each controller exposes a `/resource/schema` endpoint that returns a self-contained OpenAPI 3.0 document for that entity. The payload mirrors the format produced by `go-router`'s `MetadataAggregator`, including the controller's paths, tags, and component schema:

```json
{
  "openapi": "3.0.3",
  "paths": {
    "/user": {"post": { "summary": "Create User" }},
    "/users": {"get": { "summary": "List Users" }},
    "/user/{id}": {"get": { "summary": "Get User" }}
  },
  "tags": [
    { "name": "User" }
  ],
  "components": {
    "schemas": {
      "User": {
        "type": "object",
        "required": ["id", "email"],
        "properties": {
          "id": {"type": "string", "format": "uuid"},
          "email": {"type": "string"}
        }
      }
    }
  }
}
```

You can feed this JSON directly into Swagger UI, Stoplight Elements, or any OpenAPI tooling to visualize or validate the resource contract. Relationship metadata is embedded automatically via the generated schema.

### Schema Metadata Hints

The generated OpenAPI includes vendor extensions that help downstream form builders choose sensible defaults:

- **Display label** – mark the property that should appear in option lists with `crud:"label"` (or `crud:"label:alternate"` if the JSON name differs). The schema will include `x-formgen-label-field` with the resolved field name.
- **Relation includes** – define Bun relations (e.g. `bun:"rel:has-many,join:id=user_id"`) so `x-formgen-relations` can expose valid include paths, fields, and filter hints.
- **Shared parameters** – collection routes reuse `#/components/parameters/{Limit|Offset|Select|Include|Order}` so clients can pull defaults (limit `25`, offset `0`) straight from the spec.
- **Pruning hooks** – register `crud.WithRelationFilter` (or use `router.RegisterRelationFilter`) to hide sensitive relations from both the schema extension and runtime responses.
- **Admin extensions** – the same schema object also carries:
  - `x-admin-scope` (from `WithAdminScopeMetadata`) summarizing the expected tenant/org level, required claims, or descriptive notes.
  - `x-admin-actions` (populated automatically from `WithActions`) describing the custom endpoints, HTTP methods, and paths available.
  - `x-admin-menu` (via `WithAdminMenuMetadata`) hinting how CMS navigation should categorize the resource (group, label, icon, order, custom path, hidden flag).
  - `x-admin-row-filters` (via `WithRowFilterHints`) documenting guard/policy criteria so operators know why records are filtered.

Example:

```go
type User struct {
    bun.BaseModel `bun:"table:users"`

    ID       uuid.UUID         `bun:"id,pk,notnull" json:"id"`
    Name     string            `bun:"name,notnull" json:"name" crud:"label"`
    Email    string            `bun:"email,notnull,unique" json:"email"`
    Profiles []*UserProfile    `bun:"rel:has-many,join:id=user_id" json:"profiles,omitempty"`
}
```

The corresponding schema fragment contains:

```yaml
components:
  schemas:
    user:
      x-formgen-label-field: "name"
      x-formgen-relations:
        includes:
          - profiles
      x-admin-scope:
        level: tenant
        claims: ["users:write"]
      x-admin-actions:
        - name: Deactivate
          slug: deactivate
          method: POST
          target: resource
      x-admin-menu:
        group: Directory
        label: Users
        order: 10
      x-admin-row-filters:
        - field: tenant_id
          operator: "="
          description: Matches actor tenant

Expose these hints with:

- `WithAdminScopeMetadata` – describe the default enforcement level/claims.
- `WithAdminMenuMetadata` – provide menu grouping, ordering, or icons for go-cms/go-admin ingestion.
- `WithRowFilterHints` – advertise guard/policy criteria (e.g., owner filters, tenant locking).
- `WithActions` – declare the custom endpoints so they show up in both routing and `x-admin-actions`.

### Schema Registry & Aggregation

Every controller registers itself with the in-memory schema registry when routes are mounted. This gives admin services a single discovery point for building `/admin/schemas` without crawling each `/{resource}/schema` endpoint:

```go
entries := crud.ListSchemas() // snapshot of every registered controller

if users, ok := crud.GetSchema("user"); ok {
	log.Println(users.Document["openapi"])
}

crud.RegisterSchemaListener(func(entry crud.SchemaEntry) {
	log.Printf("schema updated: %s at %s", entry.Resource, entry.UpdatedAt)
	// e.g., push the document to go-cms
})
```

You can also register OpenAPI documents generated outside of a controller (for example, content types managed by go-cms):

```go
doc := map[string]any{
	"openapi": "3.0.3",
	"info": map[string]any{
		"title":   "Article",
		"version": "1.0.0",
	},
}

crud.RegisterSchemaDocument("article", "articles", doc)
```

To expose the aggregated registry payload directly from an admin service, stream it as JSON:

```go
if err := crud.ExportSchemas(w, crud.WithSchemaExportIndent("  ")); err != nil {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
```

Each `SchemaEntry` carries the compiled OpenAPI document (including all vendor extensions), so you can expose the aggregated payload directly or enrich it with service-specific metadata before returning it to go-admin/go-cms consumers.
        tree:
          name: user
          children:
            profiles:
              name: profiles
  parameters:
    Limit:
      description: Maximum number of records to return (default 25)
```

These hints eliminate duplicated configuration in consumers such as `go-formgen`.

### Shared Query Parameters

List endpoints in the generated OpenAPI document reference reusable query components built into `go-router`:

- `#/components/parameters/Limit` – caps the result size (defaults to `25`).
- `#/components/parameters/Offset` – skips records before pagination begins (defaults to `0`).
- `#/components/parameters/Include` – comma-separated relations to join (e.g. `profiles,company`).
- `#/components/parameters/Select` – comma-separated fields to project (e.g. `id,name,email`).

## Scope Guards & Request Metadata

Controllers can opt into multi-tenant enforcement by registering a guard adapter via `crud.WithScopeGuard`. The adapter receives the incoming request, resolves the actor, and returns a `ScopeFilter` that injects `WHERE` clauses before the repository executes.

```go
controller := crud.NewController(userRepo,
	crud.WithScopeGuard(userScopeGuard(scopeGuard)),
)

func userScopeGuard(g scope.Guard) crud.ScopeGuardFunc[*User] {
	actionMap := map[crud.CrudOperation]types.PolicyAction{
		crud.OpList: types.PolicyActionRead,
		crud.OpRead: types.PolicyActionRead,
		crud.OpCreate: types.PolicyActionCreate,
		crud.OpUpdate: types.PolicyActionUpdate,
		crud.OpDelete: types.PolicyActionDelete,
	}

	return func(ctx crud.Context, op crud.CrudOperation) (crud.ActorContext, crud.ScopeFilter, error) {
		tenantID := strings.TrimSpace(ctx.Query("tenant_id"))
		actor := crud.ActorContext{
			ActorID:  ctx.Query("actor_id", "system"),
			TenantID: tenantID,
		}

		requested := crud.ScopeFilter{}
		if tenantID != "" {
			requested.AddColumnFilter("tenant_id", "=", tenantID)
		}

		resolved, err := g.Enforce(
			ctx.UserContext(),
			toTypesActor(actor),
			toTypesScope(requested),
			actionMap[op],
			uuid.Nil,
		)
		if err != nil {
			return actor, crud.ScopeFilter{}, err
		}

		return actor, fromTypesScope(resolved), nil
	}
}
```

Helper functions (not shown) simply convert between `crud.ActorContext`/`crud.ScopeFilter` and the `types.ActorRef`/`types.ScopeFilter` structs used by `go-users`.

`ActorContext` mirrors the payload emitted by `go-auth` middleware (ID, tenant/org IDs, resource roles, impersonation flags). `ScopeFilter` collects guard-enforced column filters, and helper methods like `AddColumnFilter` make it easy to append `tenant_id = ?` clauses without touching Bun primitives. Column filters are applied automatically to `Index`, `Show`, and the `Show` read performed before `Update`/`Delete`.

Once the guard runs, go-crud stores the resolved metadata on the standard context so downstream services and repositories can reuse it:

- `crud.ContextWithActor` / `crud.ActorFromContext`
- `crud.ContextWithScope` / `crud.ScopeFromContext`
- `crud.ContextWithRequestID` / `crud.RequestIDFromContext`
- `crud.ContextWithCorrelationID` / `crud.CorrelationIDFromContext`

Lifecycle hooks now receive the same information via `HookContext.Actor`, `HookContext.Scope`, `HookContext.RequestID`, and `HookContext.CorrelationID`, so emitters can log activity without reparsing headers. Request IDs are inferred automatically from `X-Request-ID`/`Request-ID` headers (or can be pre-populated by middleware using the helpers above).
- `#/components/parameters/Order` – comma-separated ordering with optional direction (e.g. `name asc,created_at desc`).

Additional filter parameters follow the `{field}__{operator}` convention emitted by the spec (for example: `?email__ilike=@example.com`, `?age__gte=21`, `?status__or=active,pending`). These placeholders in the OpenAPI document are a reminder that **any** model field can be paired with the supported operators (`eq`, `ne`, `gt`, `lt`, `gte`, `lte`, `like`, `ilike`, `and`, `or`) to build expressive queries.

### Typed List Criteria (Non-HTTP)

For service-layer integrations, you can build repository criteria without creating a synthetic `crud.Context`:

```go
opts := crud.ListQueryOptions{
	Limit:  25,
	Offset: 0,
	Order:  "created_at desc",
	Search: "alice",
	Predicates: []crud.ListQueryPredicate{
		{Field: "status", Operator: "in", Values: []string{"active", "pending"}},
	},
}

criteria, filters, err := crud.BuildListCriteriaFromOptions[*User](
	opts,
	crud.WithSearchColumns("name", "email"),
	crud.WithStrictQueryValidation(false), // keep current fallback behavior by default
)
```

Parity notes:

- Typed and HTTP query builders share operator parsing, aliases (`SetOperatorMap`), and strict-mode validation.
- `_search` is opt-in: configure searchable columns with `WithSearchColumns`.
- Search matches OR across configured columns and ANDs with other predicates.
- Without search columns, `_search` is a no-op unless strict search checks are enabled (`WithStrictSearchColumns(true)` + strict validation).

### Options Response Shortcut

When a client appends `?format=options` to the list endpoint (e.g. `GET /users?format=options`), the controller returns a simplified payload:

```json
[
  {"value": "f5c5…", "label": "Jane Doe"},
  {"value": "23b9…", "label": "John Smith"}
]
```

- The `value` comes from the repository handler’s `GetID` (or `GetIdentifierValue` fallback).
- The `label` prefers the schema label field (`crud:"label"`), then falls back to the identifier or `value`.
- Existing pagination parameters (`limit`, `offset`, `order`, etc.) still apply before the projection occurs—fetch a page, then the controller trims each record into `{value,label}`.
- Batch create/update endpoints honour the same query parameter if callers need the refreshed options immediately after a mutation.

Omit the query parameter to receive the default envelope (`{"data":[...],"$meta":{...}}`).

## Features

- **Service Layer Delegation** – plug domain logic between the controller and repository without rewriting handlers. Supply a full `Service[T]` or override selected operations with helpers like `WithServiceFuncs`.
- **Lifecycle Hooks** – register before/after callbacks for single and batch create/update/delete operations to weave in auditing, validation, or side effects.
- **Route/Operation Toggles** – enable/disable or remap individual HTTP verbs when registering routes (e.g., prefer PATCH over PUT, drop batch operations).
- **Field Policies** – restrict/deny/mask columns per actor and append row-level filters after guard enforcement while emitting structured policy logs.
- **Custom Actions** – mount guard-aware resource or collection endpoints (e.g., “Deactivate user”) without leaving the controller by using `WithActions`.
- **Schema Registry** – aggregate every controller’s OpenAPI document via `ListSchemas`, `GetSchema`, or `RegisterSchemaListener` to power `/admin/schemas` endpoints.
- **Advanced Query Builder** – field-mapped filtering with AND/OR operators, pagination, ordering, and nested relation includes.
- **OpenAPI integration** – automatic schema and path generation, with metadata propagated from struct tags and route definitions.
- **Batch Operations & Soft Deletes** – first-class support for bulk create/update/delete and Bun’s soft-delete conventions.
- **Flexible Responses & Logging** – swap response handlers (JSON API, HAL, etc.) and wire custom loggers to trace query building.
- **Router Adapters** – ships with Fiber adapter and can be extended for other router implementations.

The repository also ships with a **web demo** (`examples/web`) that shows a combined API + HTML interface, complete with OpenAPI docs and frontend routes annotated with metadata. Run `go run ./examples/web` to explore the UI and generated documentation.

## Configuration

### Field Visibility

Use `crud:"-"` to exclude fields from API responses:

```go
type User struct {
    bun.BaseModel `bun:"table:users"`
    ID       uuid.UUID `bun:"id,pk,notnull" json:"id"`
    Password string    `bun:"password,notnull" json:"-" crud:"-"`
}
```


### Custom Response Handlers

The controller supports custom response handlers to control how data and errors are formatted. Here are some examples:

#### Default Response Format

```go
// Default responses
GET /users/123
{
    "success": true,
    "data": {
        "id": "...",
        "name": "John Doe",
        "email": "john@example.com"
    }
}

GET /users
{
    "success": true,
    "data": [...],
    "$meta": {
        "count": 10
    }
}

// Error response (problem+json via go-errors)
HTTP/1.1 404 Not Found
Content-Type: application/problem+json

{
    "error": {
        "category": "not_found",
        "code": 404,
        "text_code": "NOT_FOUND",
        "message": "Record not found",
        "metadata": {
            "operation": "read"
        }
    }
}
```

#### Custom Response Handler Example

```go
// JSONAPI style response handler
type JSONAPIResponseHandler[T any] struct{}

func (h JSONAPIResponseHandler[T]) OnData(c *fiber.Ctx, data T, op CrudOperation) error {
    c.Set("Content-type", "application/vnd.api+json")
    return c.JSON(fiber.Map{
        "data": map[string]any{
            "type":       "users",
            "id":         getId(data),
            "attributes": data,
        },
    })
}

func (h JSONAPIResponseHandler[T]) OnList(c *fiber.Ctx, data []T, op CrudOperation, total int) error {
    items := make([]map[string]any, len(data))
    for i, item := range data {
        items[i] = map[string]any{
            "type":       "users",
            "id":         getId(item),
            "attributes": item,
        }
    }
    c.Set("Content-type", "application/vnd.api+json")
    return c.JSON(fiber.Map{
        "data": items,
        "meta": map[string]any{
            "total": total,
        },
    })
}

func (h JSONAPIResponseHandler[T]) OnEmpty(c *fiber.Ctx, op CrudOperation) error {
    c.Set("Content-type", "application/vnd.api+json")
    return c.SendStatus(fiber.StatusNoContent)
}

func (h JSONAPIResponseHandler[T]) OnError(c *fiber.Ctx, err error, op CrudOperation) error {
    status := fiber.StatusInternalServerError
    if _, isNotFound := err.(*NotFoundError); isNotFound {
        status = fiber.StatusNotFound
    }
    c.Set("Content-type", "application/vnd.api+json")
    return c.Status(status).JSON(fiber.Map{
        "errors": []map[string]any{
            {
                "status": status,
                "title":  "Error",
                "detail": err.Error(),
            },
        },
    })
}

#### Error Encoders

go-crud now emits [RFC‑7807](https://datatracker.ietf.org/doc/html/rfc7807) problem+json payloads by default using [github.com/goliatone/go-errors](https://github.com/goliatone/go-errors). This keeps error categories, codes, text codes, timestamps, and metadata consistent across all controllers.

If you have existing clients that still expect the legacy `{success:false,error:string}` envelope, use the new `WithErrorEncoder` option to swap encoders without rewriting your response handler:

```go
controller := crud.NewController(repo,
    crud.WithScopeGuard(demoScopeGuard()),
    crud.WithErrorEncoder[*User](crud.LegacyJSONErrorEncoder()),
)
```

You can also build encoders with `crud.ProblemJSONErrorEncoder(...)` to set custom mappers, stack-trace behavior, or status resolvers while keeping the go-errors schema.
```

### Delegating to a Service Layer

Controllers can delegate every CRUD operation to a domain service. Provide a complete implementation via `WithService` or override specific operations with `WithServiceFuncs`, which layers on top of the default repository-backed service.

```go
type UserService struct {
	repo repository.Repository[*User]
}

func (s *UserService) Create(ctx crud.Context, record *User) (*User, error) {
	record.CreatedAt = timePtr(time.Now())
	return s.repo.Create(ctx.UserContext(), record)
}

// ...implement remaining methods or embed crud.NewRepositoryService(...)

controller := crud.NewController(
	userRepo,
	crud.WithServiceFuncs[*User](crud.ServiceFuncs[*User]{
		Create: func(ctx crud.Context, record *User) (*User, error) {
			now := time.Now()
			record.CreatedAt = &now
			return crud.NewRepositoryService(userRepo).Create(ctx, record)
		},
	}),
)

### Split Read/Write Services

When reads and writes must route to different services, configure the controller with `WithReadService` and/or `WithWriteService`. Read routes (`Index`/`Show`) use the read service when provided, while write routes (`Create`/`Update`/`Delete` and batch variants) use the write service.

```go
controller := crud.NewController(
	userRepo,
	crud.WithReadService[*User](readSvc),
	crud.WithWriteService[*User](writeSvc),
)
```

For partial implementations, adapt smaller interfaces with the helpers below:

```go
controller := crud.NewController(
	userRepo,
	crud.WithReadService[*User](crud.ReadOnlyService(readSvc)),
	crud.WithWriteService[*User](crud.WriteOnlyService(writeSvc)),
)
```

### Context Factory

Use `WithContextFactory` to inject default context values (locale, environment, tenant) before controller work runs. The factory executes for every operation and can wrap the incoming `crud.Context`.

```go
controller := crud.NewController(
	userRepo,
	crud.WithContextFactory[*User](func(base crud.Context) crud.Context {
		return &myContextWrapper{
			Context: base,
			userCtx: crud.ContextWithRequestID(base.UserContext(), "req-default"),
		}
	}),
)
```

### Custom Actions

Expose admin-only commands (approve, deactivate, sync, etc.) without forking controllers by registering custom actions. Handlers receive `ActionContext`, which embeds the router context plus actor/scope metadata resolved by your guard:

```go
controller := crud.NewController(
	userRepo,
	crud.WithActions(crud.Action[*User]{
		Name:        "Deactivate",
		Target:      crud.ActionTargetResource, // or crud.ActionTargetCollection
		Summary:     "Deactivate a user account",
		Description: "Marks the account as inactive and emits notifications",
		Handler: func(actx crud.ActionContext[*User]) error {
			id := actx.Params("id")
			if err := service.Deactivate(actx.UserContext(), id, actx.Actor); err != nil {
				return err
			}
			return actx.Status(http.StatusAccepted).JSON(fiber.Map{"ok": true})
		},
	}),
)
```

Record actions mount under `/{singular}/:id/actions/{slug}` while collection actions use `/{plural}/actions/{slug}`. Each action automatically appears in the generated OpenAPI (including the `x-admin-actions` vendor extension) so admin clients can discover the extra endpoints.

// or wrap the default repository service with command adapters:
controller := crud.NewController(
	userRepo,
	crud.WithCommandService(func(defaults crud.Service[*User]) crud.Service[*User] {
		return crud.ComposeService(defaults, crud.ServiceFuncs[*User]{
			Create: func(ctx crud.Context, user *User) (*User, error) {
				if err := lifecycleCmd.Execute(ctx.UserContext(), user); err != nil {
					return nil, err
				}
				return defaults.Show(ctx, user.ID.String(), nil)
			},
		})
	}),
)
```

### Lifecycle Hooks

Register before/after callbacks without implementing a full service:

```go
controller := crud.NewController(
	userRepo,
	crud.WithLifecycleHooks(crud.LifecycleHooks[*User]{
		BeforeCreate: func(hctx crud.HookContext, user *User) error {
			user.CreatedBy = hctx.Context.UserContext().Value(authKey).(string)
			return nil
		},
		AfterDelete: func(_ crud.HookContext, user *User) error {
			audit.LogDeletion(user.ID)
			return nil
		},
	}),
)
```

Upgrading from legacy hooks that accepted `crud.Context`? Wrap them with `crud.HookFromContext` (or `crud.HookBatchFromContext`) so they keep compiling while gaining access to the enriched metadata:

```go
crud.WithLifecycleHooks(crud.LifecycleHooks[*User]{
	BeforeCreate: crud.HookFromContext(func(ctx crud.Context, user *User) error {
		user.CreatedBy = ctx.Query("actor_id", "system")
		return nil
	}),
})
```

#### Activity & Notification Emitters

Configure `crud.WithActivityHooks` to emit structured activity for every CRUD success/failure using the shared `pkg/activity` module. The controller handles emission automatically (including batch events and failures), defaulting the channel to `crud` unless you override it.

```go
import (
	"context"

	crudactivity "github.com/goliatone/go-crud/pkg/activity"
	crudusersink "github.com/goliatone/go-crud/pkg/activity/usersink"
	usertypes "github.com/goliatone/go-users/pkg/types"
)

sink := &myUsersActivitySink{} // implements usertypes.ActivitySink
hooks := crudactivity.Hooks{
	crudusersink.Hook{Sink: sink}, // maps to go-users ActivityRecord
	crudactivity.HookFunc(func(ctx context.Context, evt crudactivity.Event) error {
		metrics.Count("crud", evt.Verb)
		return nil
	}),
}

controller := crud.NewController(
	userRepo,
	crud.WithActivityHooks(hooks, crudactivity.Config{
		Enabled: true,
		Channel: "crud", // optional; defaults to "crud"
	}),
)
```

Emitted events follow the `crud.<resource>.<op>` verb convention (append `.batch` for batch routes and `.failed` on errors) and include `route_name`, `route_path`, `method`, `request_id`, `correlation_id`, actor IDs/roles, scope labels/raw, and `error` on failures. Batch emissions carry `batch_size`, `batch_index`, and `batch_ids` when available. Timestamps default when missing, and the go-users adapter stores `definition_code`/`recipients` inside the `Data` map.

Migration from legacy helpers:
- `EmitActivity`, `ActivityEvent`, and `WithActivityEmitter` were removed; the controller now emits automatically when `WithActivityHooks` is configured.
- Drop manual helper calls in lifecycle hooks. If you need to enrich or transform events, wrap that logic inside an `activity.Hook` before forwarding to sinks.

Notifications still use `SendNotification` with `WithNotificationEmitter`:

```go
controller := crud.NewController(
	userRepo,
	crud.WithNotificationEmitter[*User](emitter),
	crud.WithLifecycleHooks(crud.LifecycleHooks[*User]{
		AfterUpdate: func(hctx crud.HookContext, user *User) error {
			return crud.SendNotification(hctx, crud.ActivityPhaseAfter, user,
				crud.WithNotificationChannel("email"),
				crud.WithNotificationTemplate("user-updated"),
				crud.WithNotificationRecipients("ops@example.com"))
		},
	}),
)
```

`SendNotification` no-ops when the emitter isn’t configured, so shared hooks can run across services that opt out of notifications.

#### Field Policies

Controllers can enforce per-actor column visibility by wiring a `FieldPolicyProvider`. The provider receives the current operation, actor, scope, and resource metadata, then returns allow/deny lists, mask functions, and optional row filters:

```go
policy := func(req crud.FieldPolicyRequest[*User]) (crud.FieldPolicy, error) {
	if req.Actor.Role == "support" {
		filter := crud.ScopeFilter{}
		filter.AddColumnFilter("tenant_id", "=", req.Actor.TenantID)

		return crud.FieldPolicy{
			Name:      "support:limited",
			Allow:     []string{"id", "name", "email"},
			Deny:      []string{"password", "ssn"},
			Mask:      map[string]crud.FieldMaskFunc{"email": func(v any) any { return "hidden@example.com" }},
			RowFilter: filter, // appended after guard criteria
		}, nil
	}
	return crud.FieldPolicy{}, nil
}

controller := crud.NewController(
	userRepo,
	crud.WithFieldPolicyProvider(policy),
)
```

Key behaviors:
- `Allow`/`Deny` determine which JSON fields can be selected, filtered, or ordered. The query builder drops disallowed fields automatically.
- `Mask` runs before the response is serialized so secrets can be obfuscated without mutating the record in storage.
- `RowFilter` appends additional criteria (e.g., `owner_id = actor_id`) after guard-enforced tenant/org filters.
- Every resolved policy is logged via `LogFieldPolicyDecision`, which attaches operation/resource/allow/deny/mask metadata to your logger implementation for auditing.

### Route/Operation Toggles

Fine-tune which routes get registered and which HTTP verbs they use:

```go
controller := crud.NewController(
	userRepo,
	crud.WithRouteConfig(crud.RouteConfig{
		Operations: map[crud.CrudOperation]crud.RouteOptions{
			crud.OpUpdate:      {Method: http.MethodPatch}, // use PATCH instead of PUT
			crud.OpDeleteBatch: {Enabled: crud.BoolPtr(false)}, // disable batch delete
		},
	}),
)

// Using the custom handler
controller := crud.NewController[*User](
    repo,
    crud.WithResponseHandler[*User](JSONAPIResponseHandler[*User]{}),
)
```

The above handler would produce responses in JSONAPI format:

```json
{
    "data": {
        "type": "users",
        "id": "123",
        "attributes": {
            "name": "John Doe",
            "email": "john@example.com"
        }
    }
}
```

### Query Parameters

The List endpoint supports:
- Pagination: `?limit=10&offset=20` (default limit: 25, offset: 0)
- Ordering: `?order=name asc,created_at desc`
- Field selection: `?select=id,name,email`
- Relations: `?include=Company,Profile` (supports filtering: `?include=Profile.status=outdated`)
- Nested relations & filters: `?include=Blocks.Translations.locale__eq=es` (any depth, multiple clauses)
- Filtering:
  - Basic: `?name=John`
  - Operators: `?age__gte=30`, `?name__ilike=john%`
  - Available operators: `eq`, `ne`, `gt`, `lt`, `gte`, `lte`, `like`, `ilike`, `and`, `or`
  - Multiple values: `?name__or=John,Jack`


## License

MIT

Copyright (c) 2024 goliatone
