package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	cmdrpc "github.com/goliatone/go-command/rpc"
	"github.com/goliatone/go-crud"
	crudrpc "github.com/goliatone/go-crud/rpc"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
	"github.com/goliatone/go-router/rpcfiber"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

//go:embed public/*
var publicFiles embed.FS

type User struct {
	bun.BaseModel `bun:"table:rpc_users"`
	ID            uuid.UUID `bun:"id,pk,notnull" json:"id"`
	Name          string    `bun:"name,notnull" json:"name"`
	Email         string    `bun:"email,notnull,unique" json:"email"`
	TenantID      string    `bun:"tenant_id,notnull" json:"tenant_id"`
	CreatedAt     time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

type App struct {
	server router.Server[*fiber.App]
	rpc    *cmdrpc.Server
	repo   repository.Repository[*User]
	db     *bun.DB
}

func NewApp() (*App, error) {
	sqldb, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	if _, err := db.NewCreateTable().Model((*User)(nil)).IfNotExists().Exec(context.Background()); err != nil {
		_ = db.Close()
		_ = sqldb.Close()
		return nil, fmt.Errorf("create table rpc_users: %w", err)
	}

	repo := repository.NewRepository(db, repository.ModelHandlers[*User]{
		NewRecord: func() *User { return &User{} },
		GetID:     func(u *User) uuid.UUID { return u.ID },
		SetID:     func(u *User, id uuid.UUID) { u.ID = id },
		GetIdentifier: func() string {
			return "Email"
		},
	})

	if err := seedData(repo); err != nil {
		_ = db.Close()
		_ = sqldb.Close()
		return nil, fmt.Errorf("seed users: %w", err)
	}

	controller := crud.NewController[*User](
		repo,
		crud.WithScopeGuard[*User](demoScopeGuard),
		crud.WithLifecycleHooks(crud.LifecycleHooks[*User]{
			BeforeCreate: []crud.HookFunc[*User]{func(hctx crud.HookContext, record *User) error {
				applyCreateDefaults(record, hctx.Actor)
				return nil
			}},
			BeforeCreateBatch: []crud.HookBatchFunc[*User]{func(hctx crud.HookContext, records []*User) error {
				for _, record := range records {
					applyCreateDefaults(record, hctx.Actor)
				}
				return nil
			}},
			BeforeUpdate: []crud.HookFunc[*User]{func(_ crud.HookContext, record *User) error {
				if record != nil {
					record.UpdatedAt = time.Now().UTC()
				}
				return nil
			}},
			BeforeUpdateBatch: []crud.HookBatchFunc[*User]{func(_ crud.HookContext, records []*User) error {
				now := time.Now().UTC()
				for _, record := range records {
					if record != nil {
						record.UpdatedAt = now
					}
				}
				return nil
			}},
		}),
	)

	rpcServer := cmdrpc.NewServer(cmdrpc.WithFailureMode(cmdrpc.FailureModeRecover))
	if err := crudrpc.RegisterResourceEndpoints(rpcServer, controller, crudrpc.ResourceRegistrationOptions{Resource: "user"}); err != nil {
		_ = db.Close()
		_ = sqldb.Close()
		return nil, fmt.Errorf("register resource endpoints: %w", err)
	}

	server := router.NewFiberAdapter(func(_ *fiber.App) *fiber.App {
		return fiber.New(fiber.Config{
			AppName:           "go-crud RPC web debug",
			EnablePrintRoutes: true,
		})
	})

	if err := registerRoutes(server.Router(), repo, rpcServer); err != nil {
		_ = db.Close()
		_ = sqldb.Close()
		return nil, err
	}

	return &App{server: server, rpc: rpcServer, repo: repo, db: db}, nil
}

func registerRoutes(
	r router.Router[*fiber.App],
	repo repository.Repository[*User],
	rpcServer *cmdrpc.Server,
) error {
	publicSub, err := fs.Sub(publicFiles, "public")
	if err != nil {
		return fmt.Errorf("load embedded public files: %w", err)
	}

	publicHandler := router.HandlerFromHTTP(http.StripPrefix("/public/", http.FileServer(http.FS(publicSub))))
	r.Get("/", handleIndex)
	r.Get("/public/*", publicHandler)
	r.Head("/public/*", publicHandler)
	r.Get("/api/state", handleState(repo))

	if err := rpcfiber.MountFiber(r, rpcServer); err != nil {
		return fmt.Errorf("mount rpc fiber routes: %w", err)
	}

	return nil
}

func handleIndex(ctx router.Context) error {
	content, err := publicFiles.ReadFile("public/index.html")
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]any{"error": "failed to load index"})
	}
	ctx.SetHeader(router.HeaderContentType, "text/html; charset=utf-8")
	return ctx.Send(content)
}

type appState struct {
	Count      int            `json:"count"`
	LastID     string         `json:"lastId,omitempty"`
	LastName   string         `json:"lastName,omitempty"`
	LastTenant string         `json:"lastTenant,omitempty"`
	Tenants    map[string]int `json:"tenants"`
}

func handleState(repo repository.Repository[*User]) router.HandlerFunc {
	return func(ctx router.Context) error {
		records, _, err := repo.List(ctx.Context())
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}

		state := appState{
			Count:   len(records),
			Tenants: map[string]int{},
		}

		for _, record := range records {
			if record == nil {
				continue
			}
			state.Tenants[record.TenantID]++
		}

		sort.Slice(records, func(i, j int) bool {
			left := records[i]
			right := records[j]
			if left == nil {
				return false
			}
			if right == nil {
				return true
			}
			return left.UpdatedAt.After(right.UpdatedAt)
		})

		if len(records) > 0 && records[0] != nil {
			state.LastID = records[0].ID.String()
			state.LastName = records[0].Name
			state.LastTenant = records[0].TenantID
		}

		return ctx.JSON(http.StatusOK, map[string]any{"state": state})
	}
}

func seedData(repo repository.Repository[*User]) error {
	now := time.Now().UTC()
	samples := []*User{
		{
			ID:        uuid.New(),
			Name:      "Ada Lovelace",
			Email:     "ada@example.com",
			TenantID:  "tenant-alpha",
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Grace Hopper",
			Email:     "grace@example.com",
			TenantID:  "tenant-alpha",
			CreatedAt: now.Add(-90 * time.Minute),
			UpdatedAt: now.Add(-90 * time.Minute),
		},
		{
			ID:        uuid.New(),
			Name:      "Linus Torvalds",
			Email:     "linus@example.com",
			TenantID:  "tenant-beta",
			CreatedAt: now.Add(-30 * time.Minute),
			UpdatedAt: now.Add(-30 * time.Minute),
		},
	}

	for _, record := range samples {
		if _, err := repo.Create(context.Background(), record); err != nil {
			return err
		}
	}

	return nil
}

func applyCreateDefaults(record *User, actor crud.ActorContext) {
	if record == nil {
		return
	}
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	if strings.TrimSpace(record.TenantID) == "" {
		record.TenantID = strings.TrimSpace(actor.TenantID)
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}
}

func demoScopeGuard(ctx crud.Context, _ crud.CrudOperation) (crud.ActorContext, crud.ScopeFilter, error) {
	actor := crud.ActorFromContext(ctx.UserContext())
	if strings.TrimSpace(actor.ActorID) == "" {
		return crud.ActorContext{}, crud.ScopeFilter{}, fmt.Errorf("meta.actorId is required")
	}

	scope := crud.ScopeFilter{}
	if tenant := strings.TrimSpace(actor.TenantID); tenant != "" {
		scope.AddColumnFilter("tenant_id", "=", tenant)
	}

	return actor, scope, nil
}

func (a *App) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	return a.db.Close()
}

func (a *App) Serve(addr string) error {
	if a == nil || a.server == nil {
		return fmt.Errorf("app server not initialized")
	}
	return a.server.Serve(addr)
}

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}
	defer func() {
		if err := app.Close(); err != nil {
			log.Printf("failed to close app resources: %v", err)
		}
	}()

	app.server.Router().PrintRoutes()

	addr := ":8092"
	log.Printf("go-crud RPC web debug demo listening on http://localhost%s", addr)
	log.Printf("RPC invoke endpoint: POST http://localhost%s/api/rpc", addr)
	log.Printf("RPC discovery endpoint: GET http://localhost%s/api/rpc/endpoints", addr)

	if err := app.Serve(addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
