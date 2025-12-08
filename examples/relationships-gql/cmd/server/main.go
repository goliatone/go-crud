package main

import (
	"context"
	"errors"
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

	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/generated"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/resolvers"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("relationships-gql server failed: %v", err)
	}
}

func run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := relationships.SetupDatabase(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if client == nil {
			return
		}
		if db := client.DB(); db != nil {
			_ = db.Close()
		}
	}()

	db := client.DB()

	repos := relationships.RegisterRepositories(db)
	if err := relationships.MigrateSchema(ctx, db); err != nil {
		return err
	}
	if err := relationships.SeedDatabase(ctx, client); err != nil {
		return err
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Serve(addr)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	log.Println("Shutting down server...")
	if err := app.Shutdown(context.Background()); err != nil {
		return err
	}
	return nil
}
