package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	cmdrpc "github.com/goliatone/go-command/rpc"
	"github.com/goliatone/go-crud"
	crudrpc "github.com/goliatone/go-crud/rpc"
	repository "github.com/goliatone/go-repository-bun"
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
	rpc  *cmdrpc.Server
	repo repository.Repository[*User]
	db   *bun.DB
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

	return &App{rpc: rpcServer, repo: repo, db: db}, nil
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

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.Handle("/public/", http.StripPrefix("/public/", a.staticFiles()))
	mux.HandleFunc("/api/endpoints", a.handleEndpoints)
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/rpc", a.handleRPC)
	return mux
}

func (a *App) staticFiles() http.Handler {
	sub, err := fs.Sub(publicFiles, "public")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	content, err := publicFiles.ReadFile("public/index.html")
	if err != nil {
		http.Error(w, "failed to load index", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}

func (a *App) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeRPCError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"endpoints": a.rpc.EndpointsMeta()})
}

type appState struct {
	Count      int            `json:"count"`
	LastID     string         `json:"lastId,omitempty"`
	LastName   string         `json:"lastName,omitempty"`
	LastTenant string         `json:"lastTenant,omitempty"`
	Tenants    map[string]int `json:"tenants"`
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeRPCError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records, _, err := a.repo.List(r.Context())
	if err != nil {
		a.writeRPCError(w, http.StatusInternalServerError, err.Error())
		return
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

	a.writeJSON(w, http.StatusOK, map[string]any{"state": state})
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id,omitempty"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (a *App) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeJSON(w, http.StatusMethodNotAllowed, jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: -32600, Message: "method not allowed"},
		})
		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeJSON(w, http.StatusBadRequest, jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: -32700, Message: "invalid JSON payload", Data: err.Error()},
		})
		return
	}
	if req.Method == "" {
		a.writeJSON(w, http.StatusBadRequest, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32600, Message: "method is required"},
		})
		return
	}

	prototype, err := a.rpc.NewRequestForMethod(req.Method)
	if err != nil {
		a.writeJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32601,
				Message: "method not found",
				Data:    err.Error(),
			},
		})
		return
	}

	payload, err := decodeRPCPayload(req.Params, prototype)
	if err != nil {
		a.writeJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32602,
				Message: "invalid method params",
				Data:    err.Error(),
			},
		})
		return
	}

	result, err := a.rpc.Invoke(r.Context(), req.Method, payload)
	if err != nil {
		a.writeJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32000,
				Message: "rpc invocation failed",
				Data:    err.Error(),
			},
		})
		return
	}

	a.writeJSON(w, http.StatusOK, jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	})
}

func decodeRPCPayload(raw json.RawMessage, prototype any) (any, error) {
	if prototype == nil {
		if hasPayload(raw) {
			return nil, errors.New("method does not accept params")
		}
		return nil, nil
	}
	if !hasPayload(raw) {
		return prototype, nil
	}

	value := reflect.ValueOf(prototype)
	if !value.IsValid() {
		return nil, errors.New("invalid method request type")
	}

	if value.Kind() == reflect.Ptr {
		if err := json.Unmarshal(raw, prototype); err != nil {
			return nil, err
		}
		return prototype, nil
	}

	target := reflect.New(value.Type())
	if err := json.Unmarshal(raw, target.Interface()); err != nil {
		return nil, err
	}
	return target.Elem().Interface(), nil
}

func hasPayload(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func (a *App) writeRPCError(w http.ResponseWriter, status int, msg string) {
	a.writeJSON(w, status, map[string]any{"error": msg})
}

func (a *App) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
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

	addr := ":8092"
	log.Printf("go-crud RPC web debug demo listening on http://localhost%s", addr)
	log.Printf("Use the UI to call methods like crud.user.create and crud.user.index")

	if err := http.ListenAndServe(addr, app.Routes()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
