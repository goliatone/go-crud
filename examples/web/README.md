# go-crud Web UI Example

A complete web application demonstrating the `go-crud` package capabilities with a responsive web interface.

## Overview

This example showcases:
- Full CRUD operations (Create, Read, Update, Delete) with a web UI
- RESTful API endpoints using `go-crud` controller
- `go-router` with Fiber adapter for routing
- Django templates for server-side rendering
- Responsive design (desktop and mobile)
- Flash messages for user feedback
- In-memory SQLite database
- OpenAPI documentation

## Features

### Web Interface
- **User List**: Searchable, sortable table with pagination-ready design
- **User Detail**: View complete user information
- **Create User**: Form to add new users with validation
- **Edit User**: Update existing user information
- **Delete User**: Remove users with confirmation modal

### API Endpoints
All CRUD operations are available via RESTful API endpoints:

- `GET /api/users` - List all users (plural)
- `GET /api/user/:id` - Get user by ID (singular)
- `POST /api/user` - Create new user (singular)
- `PUT /api/user/:id` - Update user (singular)
- `DELETE /api/user/:id` - Delete user (singular)
- `POST /api/user/batch` - Batch create users
- `PUT /api/user/batch` - Batch update users
- `DELETE /api/user/batch` - Batch delete users

**Note**: The go-crud controller automatically generates resource names. List endpoints use the plural form (`/api/users`), while individual CRUD operations use the singular form (`/api/user/:id`).

### Advanced Query Features
The API supports advanced querying capabilities:

```bash
# Pagination
GET /api/users?limit=10&offset=20

# Ordering
GET /api/users?order=name asc,created_at desc

# Field selection
GET /api/users?select=id,name,email

# Filtering
GET /api/users?name__ilike=Alice%
GET /api/users?active__eq=true
GET /api/users?email__or=alice@example.com,bob@example.com
```

## Prerequisites

- Go 1.21 or higher
- No external database required (uses in-memory SQLite via `github.com/mattn/go-sqlite3`)
- CGO enabled (required for SQLite driver)

## Running the Example

1. Navigate to the example directory:
```bash
cd examples/web
```

2. Install dependencies:
```bash
go mod tidy
```

3. Run the application:
```bash
go run main.go
```

4. Open your browser to:
```
http://localhost:9090
```

## Project Structure

```
examples/web/
├── main.go                 # Application entry point
├── views/                  # HTML templates
│   ├── layout.html        # Base layout template
│   ├── index.html         # User list page
│   ├── user-detail.html   # User detail page
│   ├── user-form.html     # Create/Edit form
│   ├── error.html         # Error page
│   ├── css/
│   │   └── main.css       # Styles
│   └── js/
│       └── app.js         # Client-side JavaScript
└── README.md              # This file
```

## User Model

```go
type User struct {
    ID        uuid.UUID  // Non-nullable UUID
    Name      string     // Required field
    Email     string     // Required, unique field
    Bio       string     // Optional field
    Active    bool       // Boolean flag
    DeletedAt *time.Time // Soft delete support (nullable)
    CreatedAt time.Time  // Auto-managed timestamp
    UpdatedAt time.Time  // Auto-managed timestamp
}
```

The model uses value types for `ID`, `CreatedAt`, and `UpdatedAt` to work with SQLite. The repository automatically manages ID generation and timestamps.

## API Examples

### Create a User
```bash
curl -X POST http://localhost:9090/api/user \
  -H "Content-Type: application/json" \
  -d '{
    "name": "John Doe",
    "email": "john@example.com",
    "bio": "Software engineer",
    "active": true
  }'
```

**Note**: The `id`, `created_at`, and `updated_at` fields are automatically managed by the repository and should not be included in create requests.

### List Users
```bash
curl http://localhost:9090/api/users
```

### Get User by ID
```bash
curl http://localhost:9090/api/user/{id}
```

### Update User
```bash
curl -X PUT http://localhost:9090/api/user/{id} \
  -H "Content-Type: application/json" \
  -d '{
    "name": "John Smith",
    "bio": "Senior software engineer"
  }'
```

### Delete User
```bash
curl -X DELETE http://localhost:9090/api/user/{id}
```

### Batch Create Users
```bash
curl -X POST http://localhost:9090/api/user/batch \
  -H "Content-Type: application/json" \
  -d '[
    {"name": "User 1", "email": "user1@example.com"},
    {"name": "User 2", "email": "user2@example.com"}
  ]'
```

## OpenAPI Documentation

Access the interactive API documentation at:
```
http://localhost:9090/meta/docs
```

For raw machine-readable metadata, each resource exposes its schema as an OpenAPI 3.0 document. For example:
```
curl http://localhost:9090/api/user/schema | jq
```
returns the generated paths, tags, and `components.schemas.User` definition that you can feed into Swagger or Stoplight tooling.

## Customization

### Changing the Port

Edit `main.go` and modify the port in the `Serve` call:

```go
app.Serve(":8080")  // Change to your desired port
```

### Adding Fields

1. Add fields to the `User` struct in `main.go`
2. Update the HTML templates to display/edit the new fields
3. The API will automatically include the new fields

### Custom Validation

Add validation logic in the form handlers:

```go
func handleCreateUser(repo repository.Repository[*User]) router.HandlerFunc {
    return func(c router.Context) error {
        name := c.FormValue("name")

        // Add custom validation here
        if len(name) < 3 {
            return c.Render("user-form", map[string]any{
                "mode": "create",
                "errors": []map[string]string{
                    {"field": "name", "message": "Name must be at least 3 characters"},
                },
            })
        }
        // ... rest of handler
    }
}
```

Note: The repository uses pointer types (`Repository[*User]`) as required by the go-repository-bun package.

## Technologies Used

- **[go-crud](https://github.com/goliatone/go-crud)**: Generic CRUD controller
- **[go-router](https://github.com/goliatone/go-router)**: Router abstraction layer
- **[go-repository-bun](https://github.com/goliatone/go-repository-bun)**: Repository pattern with Bun ORM
- **[Fiber v2](https://gofiber.io/)**: HTTP framework
- **[Bun](https://bun.uptrace.dev/)**: SQL ORM
- **[Django Templates](https://github.com/gofiber/template)**: Template engine

## License

This example is provided as-is for demonstration purposes.
