# Go CRUD Controller

A Go package that provides a generic CRUD controller for REST APIs using Fiber and Bun ORM.

## Installation

```bash
go get github.com/yourusername/crud-controller
```

## Quick Start

```go
type User struct {
    bun.BaseModel `bun:"table:users,alias:u"`
    ID     uuid.UUID `bun:"id,pk,notnull" json:"id"`
    Name   string    `bun:"name,notnull" json:"name"`
    Email  string    `bun:"email,notnull" json:"email"`
}

func main() {
    db := setupDatabase()
    app := fiber.New()

    repo := NewUserRepository(db)
    crud.NewController[*User](repo).RegisterRoutes(app)

    app.Listen(":3000")
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
