package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/goliatone/go-router"

	"github.com/goliatone/go-crud/examples/relationships-gql/graph/generated"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/resolvers"
	"github.com/goliatone/go-crud/examples/relationships-gql/internal/data"
)

func main() {
	ctx := context.Background()

	client, err := data.SetupDatabase(ctx)
	if err != nil {
		log.Fatalf("failed to setup database: %v", err)
	}
	defer client.Close()

	db := client.DB()

	repos := data.RegisterRepositories(db)
	if err := data.MigrateSchema(ctx, db); err != nil {
		log.Fatalf("failed to migrate schema: %v", err)
	}
	if err := data.SeedDatabase(ctx, db, repos); err != nil {
		log.Fatalf("failed to seed database: %v", err)
	}

	resolver := resolvers.NewResolver(repos)
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	app := router.NewFiberAdapter(func(_ *fiber.App) *fiber.App {
		return fiber.New(fiber.Config{
			AppName:           "go-crud relationships-gql",
			EnablePrintRoutes: true,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
		})
	})

	// Ensure the adapter router is initialized even if we only use the wrapped Fiber app.
	_ = app.Router()

	fiberApp := app.WrappedRouter()
	fiberApp.Post("/graphql", adaptor.HTTPHandler(srv))
	fiberApp.Get("/playground", adaptor.HTTPHandler(playground.Handler("GraphQL playground", "/graphql")))
	fiberApp.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/playground", fiber.StatusTemporaryRedirect)
	})

	addr := ":9091"
	log.Printf("GraphQL endpoint ready at http://localhost%s/graphql", addr)
	log.Printf("Playground available at http://localhost%s/playground", addr)

	go func() {
		if err := app.Serve(addr); err != nil {
			log.Panicf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down server...")
	if err := app.Shutdown(context.Background()); err != nil {
		log.Panicf("failed to shut down: %v", err)
	}
}
