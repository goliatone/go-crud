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

### Generated Routes

For a `User` struct, the following routes are automatically created:

```
GET    /user/schema       - Get OpenAPI schema for the resource
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

## Features

- **Service Layer Delegation** – plug domain logic between the controller and repository without rewriting handlers. Supply a full `Service[T]` or override selected operations with helpers like `WithServiceFuncs`.
- **Lifecycle Hooks** – register before/after callbacks for single and batch create/update/delete operations to weave in auditing, validation, or side effects.
- **Route/Operation Toggles** – enable/disable or remap individual HTTP verbs when registering routes (e.g., prefer PATCH over PUT, drop batch operations).
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

// Error response
{
    "success": false,
    "error": "Record not found"
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
