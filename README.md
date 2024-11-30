# Go CRUD Controller

A Go package that provides a generic CRUD controller for REST APIs using Fiber and Bun ORM.

## Installation

```bash
go get github.com/yourusername/crud-controller
```

## Quick Start

```go
package main

import (
	"time"
	"database/sql"

    "github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
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
	sqldb, err := sql.Open(sqliteshim.ShimName, "file::memory:?cache=shared")
	if err != nil {
		panic(err)
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())


	server := fiber.New()
	api := server.Group("/api/v1")
	crud.NewController[*model.User](model.NewUserRepository(db)).RegisterRoutes(api)

	log.Fatal(server.Listen(":3000"))
}

```

### Generated Routes

For a `User` struct, the following routes are automatically created:

```
GET    /user/:id      - Get a single user
GET    /users         - List users
POST   /user          - Create a user
PUT    /user/:id      - Update a user
DELETE /user/:id      - Delete a user
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
        "data": map[string]interface{}{
            "type":       "users",
            "id":         getId(data),
            "attributes": data,
        },
    })
}

func (h JSONAPIResponseHandler[T]) OnList(c *fiber.Ctx, data []T, op CrudOperation, total int) error {
    items := make([]map[string]interface{}, len(data))
    for i, item := range data {
        items[i] = map[string]interface{}{
            "type":       "users",
            "id":         getId(item),
            "attributes": item,
        }
    }
    c.Set("Content-type", "application/vnd.api+json")
    return c.JSON(fiber.Map{
        "data": items,
        "meta": map[string]interface{}{
            "total": total,
        },
    })
}

func (h JSONAPIResponseHandler[T]) OnError(c *fiber.Ctx, err error, op CrudOperation) error {
    status := fiber.StatusInternalServerError
    if _, isNotFound := err.(*NotFoundError); isNotFound {
        status = fiber.StatusNotFound
    }
    c.Set("Content-type", "application/vnd.api+json")
    return c.Status(status).JSON(fiber.Map{
        "errors": []map[string]interface{}{
            {
                "status": status,
                "title":  "Error",
                "detail": err.Error(),
            },
        },
    })
}

func (h JSONAPIResponseHandler[T]) OnEmpty(c *fiber.Ctx, op CrudOperation) error {
    c.Set("Content-type", "application/vnd.api+json")
    return c.SendStatus(fiber.StatusNoContent)
}

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
- Pagination: `?limit=10&offset=20`
- Ordering: `?order=name asc,created_at desc`
- Field selection: `?select=id,name,email`
- Relations: `?include=company,profile`
- Filtering:
  - Basic: `?name=John`
  - Operators: `?age__gte=30`
  - Multiple values: `?status__in=active,pending`


## License

MIT

Copyright (c) 2024 goliatone
