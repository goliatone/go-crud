package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/django/v3"
	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
	"github.com/goliatone/go-router/flash"
	flashmw "github.com/goliatone/go-router/middleware/flash"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

// User model for CRUD operations
type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`
	ID            uuid.UUID  `bun:"id,pk,notnull" json:"id"`
	Name          string     `bun:"name,notnull" json:"name"`
	Email         string     `bun:"email,notnull,unique" json:"email"`
	Bio           string     `bun:"bio" json:"bio,omitempty"`
	Active        bool       `bun:"active,notnull" json:"active"`
	DeletedAt     *time.Time `bun:"deleted_at,soft_delete,nullzero" json:"deleted_at,omitempty"`
	CreatedAt     time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt     time.Time  `bun:"updated_at,notnull" json:"updated_at"`
}

func main() {
	// Setup database
	db := setupDatabase()
	defer db.Close()

	// Create repository
	handlers := repository.ModelHandlers[*User]{
		NewRecord: func() *User {
			return &User{}
		},
		GetID: func(u *User) uuid.UUID {
			return u.ID
		},
		SetID: func(u *User, id uuid.UUID) {
			u.ID = id
		},
		GetIdentifier: func() string {
			return "Email"
		},
	}
	repo := repository.NewRepository(db, handlers)

	// Seed database
	seedDatabase(repo)

	// Setup Fiber app with Django templates
	engine := django.New("./views", ".html")
	app := router.NewFiberAdapter(func(a *fiber.App) *fiber.App {
		return fiber.New(
			fiber.Config{
				AppName:           "go-crud Web Demo",
				EnablePrintRoutes: true,
				PassLocalsToViews: true,
				Views:             engine,
			},
		)
	})

	// Serve static files
	app.WrappedRouter().Static("/static", "./views")

	// Create CRUD controller
	controller := crud.NewController(repo)

	// API routes with JSON responses
	api := app.Router().Group("/api")
	apiAdapter := crud.NewGoRouterAdapter(api)
	controller.RegisterRoutes(apiAdapter)

	// Front-end routes with HTML rendering
	front := app.Router().Use(router.ToMiddleware(func(c router.Context) error {
		c.SetHeader(router.HeaderContentType, "text/html")
		return c.Next()
	}))

	// Add flash middleware for front-end routes
	front.Use(flashmw.New())

	// Register front-end routes
	createFrontEndRoutes(front, repo)

	// OpenAPI documentation - serve on root router to see all routes including API
	router.ServeOpenAPI(app.Router(), &router.OpenAPIRenderer{
		Title:   "go-crud Web Demo",
		Version: "v0.1.0",
		Description: `## CRUD Operations Demo
This application demonstrates the go-crud package capabilities with a web interface.

### Features
- Full CRUD operations (Create, Read, Update, Delete)
- RESTful API endpoints
- Web interface with responsive design
- Advanced query capabilities (filtering, pagination, sorting)
- Batch operations support
		`,
		Contact: &router.OpenAPIFieldContact{
			Email: "info@example.com",
			Name:  "go-crud Demo",
			URL:   "https://github.com/goliatone/go-crud",
		},
	})

	// Print routes
	app.Router().PrintRoutes()

	// Start server
	go func() {
		log.Println("Starting server on :9090")
		log.Println("Open http://localhost:9090 in your browser")
		if err := app.Serve(":9090"); err != nil {
			log.Panic(err)
		}
	}()

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down server...")
	ctx := context.TODO()
	if err := app.Shutdown(ctx); err != nil {
		log.Panic(err)
	}
}

func setupDatabase() *bun.DB {
	sqldb, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		log.Fatal(err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Create schema
	ctx := context.Background()
	_, err = db.NewCreateTable().Model((*User)(nil)).IfNotExists().Exec(ctx)
	if err != nil {
		log.Fatal(err)
	}

	return db
}

func seedDatabase(repo repository.Repository[*User]) {
	ctx := context.Background()

	users := []*User{
		{
			Name:   "Alice Johnson",
			Email:  "alice.johnson@example.com",
			Bio:    "Software engineer passionate about Go and distributed systems",
			Active: true,
		},
		{
			Name:   "Bob Smith",
			Email:  "bob.smith@example.com",
			Bio:    "DevOps specialist with expertise in cloud infrastructure",
			Active: true,
		},
		{
			Name:   "Carol Williams",
			Email:  "carol.williams@example.com",
			Bio:    "Product manager focused on developer tools",
			Active: true,
		},
	}

	for _, user := range users {
		_, err := repo.Create(ctx, user)
		if err != nil {
			log.Printf("Failed to seed user %s: %v", user.Name, err)
		}
	}

	log.Println("Database seeded with sample data")
}

// Front-end route handlers
func createFrontEndRoutes[T any](front router.Router[T], repo repository.Repository[*User]) {
	users := front.Group("/users")

	front.Get("/", renderUserList(repo)).
		SetName("web:users:list").
		SetSummary("User Directory").
		SetDescription("HTML view that lists all users").
		AddTags("Front-End", "Users")

	users.Get("/new", renderCreateForm()).
		SetName("web:users:new").
		SetSummary("Create User Form").
		SetDescription("Displays the form to create a new user").
		AddTags("Front-End", "Users")

	users.Get("/:id", renderUserDetail(repo)).
		SetName("web:users:detail").
		SetSummary("User Detail Page").
		SetDescription("Shows details for a single user").
		AddTags("Front-End", "Users")

	users.Get("/:id/edit", renderEditForm(repo)).
		SetName("web:users:edit").
		SetSummary("Edit User Form").
		SetDescription("Displays the form to edit an existing user").
		AddTags("Front-End", "Users")

	users.Post("/", handleCreateUser(repo)).
		SetName("web:users:create").
		SetSummary("Submit Create Form").
		SetDescription("Processes the HTML form submission to create a user").
		AddTags("Front-End", "Users")

	users.Post("/:id", handleUpdateUser(repo)).
		SetName("web:users:update").
		SetSummary("Submit Update Form").
		SetDescription("Processes the HTML form submission to update an existing user").
		AddTags("Front-End", "Users")

	users.Post("/:id/delete", handleDeleteUser(repo)).
		SetName("web:users:delete").
		SetSummary("Submit Delete Form").
		SetDescription("Processes the HTML form submission to delete a user").
		AddTags("Front-End", "Users")
}

func renderUserList(repo repository.Repository[*User]) router.HandlerFunc {
	return func(c router.Context) error {
		ctx := c.Context()
		users, _, err := repo.List(ctx)
		if err != nil {
			return c.Render("error", map[string]any{
				"error_code":    500,
				"error_title":   "Error Loading Users",
				"error_message": err.Error(),
			})
		}

		return c.Render("index", map[string]any{
			"users": users,
		})
	}
}

func renderCreateForm() router.HandlerFunc {
	return func(c router.Context) error {
		return c.Render("user-form", map[string]any{
			"mode": "create",
		})
	}
}

func renderUserDetail(repo repository.Repository[*User]) router.HandlerFunc {
	return func(c router.Context) error {
		id := c.Param("id", "")
		ctx := c.Context()

		user, err := repo.GetByID(ctx, id)
		if err != nil {
			return c.Render("error", map[string]any{
				"error_code":    404,
				"error_title":   "User Not Found",
				"error_message": fmt.Sprintf("The user with ID %s does not exist.", id),
			})
		}

		return c.Render("user-detail", map[string]any{
			"user": user,
		})
	}
}

func renderEditForm(repo repository.Repository[*User]) router.HandlerFunc {
	return func(c router.Context) error {
		id := c.Param("id", "")
		ctx := c.Context()

		user, err := repo.GetByID(ctx, id)
		if err != nil {
			return c.Render("error", map[string]any{
				"error_code":    404,
				"error_title":   "User Not Found",
				"error_message": fmt.Sprintf("The user with ID %s does not exist.", id),
			})
		}

		return c.Render("user-form", map[string]any{
			"mode": "edit",
			"user": user,
		})
	}
}

func handleCreateUser(repo repository.Repository[*User]) router.HandlerFunc {
	return func(c router.Context) error {
		name := c.FormValue("name")
		email := c.FormValue("email")
		bio := c.FormValue("bio")

		// Validation
		var validationErrors []map[string]string
		if name == "" {
			validationErrors = append(validationErrors, map[string]string{
				"field":   "name",
				"message": "Name is required",
			})
		}
		if email == "" {
			validationErrors = append(validationErrors, map[string]string{
				"field":   "email",
				"message": "Email is required",
			})
		}

		if len(validationErrors) > 0 {
			return c.Render("user-form", map[string]any{
				"mode":   "create",
				"errors": validationErrors,
				"user": map[string]string{
					"name":  name,
					"email": email,
					"bio":   bio,
				},
			})
		}

		// Create user
		user := &User{
			Name:   name,
			Email:  email,
			Bio:    bio,
			Active: true,
		}

		ctx := c.Context()
		createdUser, err := repo.Create(ctx, user)
		if err != nil {
			return c.Render("user-form", map[string]any{
				"mode": "create",
				"errors": []map[string]string{
					{
						"field":   "email",
						"message": "Email already exists or invalid",
					},
				},
				"user": map[string]string{
					"name":  name,
					"email": email,
					"bio":   bio,
				},
			})
		}

		// Set flash message and redirect
		return flash.Redirect(c, fmt.Sprintf("/users/%s", createdUser.ID.String()), router.ViewContext{
			"success":         true,
			"success_message": fmt.Sprintf("User '%s' has been created successfully", name),
		})
	}
}

func handleUpdateUser(repo repository.Repository[*User]) router.HandlerFunc {
	return func(c router.Context) error {
		id := c.Param("id", "")
		name := c.FormValue("name")
		bio := c.FormValue("bio")

		ctx := c.Context()

		// Get existing user
		user, err := repo.GetByID(ctx, id)
		if err != nil {
			return c.Render("error", map[string]any{
				"error_code":    404,
				"error_title":   "User Not Found",
				"error_message": fmt.Sprintf("The user with ID %s does not exist.", id),
			})
		}

		// Validation
		if name == "" {
			return c.Render("user-form", map[string]any{
				"mode": "edit",
				"user": user,
				"errors": []map[string]string{
					{
						"field":   "name",
						"message": "Name is required",
					},
				},
			})
		}

		// Update user
		user.Name = name
		user.Bio = bio
		user.UpdatedAt = time.Now()

		updatedUser, err := repo.Update(ctx, user)
		if err != nil {
			return c.Render("user-form", map[string]any{
				"mode": "edit",
				"user": user,
				"errors": []map[string]string{
					{
						"field":   "general",
						"message": "Failed to update user",
					},
				},
			})
		}

		// Set flash message and redirect
		return flash.Redirect(c, fmt.Sprintf("/users/%s", updatedUser.ID.String()), router.ViewContext{
			"success":         true,
			"success_message": fmt.Sprintf("User '%s' has been updated successfully", name),
		})
	}
}

func handleDeleteUser(repo repository.Repository[*User]) router.HandlerFunc {
	return func(c router.Context) error {
		id := c.Param("id", "")
		ctx := c.Context()

		user, err := repo.GetByID(ctx, id)
		if err != nil {
			return flash.Redirect(c, "/", router.ViewContext{
				"error":         true,
				"error_message": "User not found",
			})
		}

		err = repo.Delete(ctx, user)
		if err != nil {
			return flash.Redirect(c, "/", router.ViewContext{
				"error":         true,
				"error_message": "Failed to delete user",
			})
		}

		// Set flash message and redirect
		return flash.Redirect(c, "/", router.ViewContext{
			"success":         true,
			"success_message": fmt.Sprintf("User '%s' has been deleted successfully", user.Name),
		})
	}
}
