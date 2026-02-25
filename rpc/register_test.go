package rpc

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/goliatone/go-command"
	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type rpcUser struct {
	bun.BaseModel `bun:"table:rpc_users"`
	ID            uuid.UUID `bun:"id,pk,notnull" json:"id"`
	Name          string    `bun:"name,notnull" json:"name"`
	Email         string    `bun:"email,notnull,unique" json:"email"`
	CreatedAt     time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

type registeredHandler struct {
	opts    command.RPCConfig
	handler any
	meta    command.CommandMeta
}

type fakeRegistrar struct {
	handlers map[string]registeredHandler
}

func newFakeRegistrar() *fakeRegistrar {
	return &fakeRegistrar{handlers: map[string]registeredHandler{}}
}

func (f *fakeRegistrar) Register(opts command.RPCConfig, handler any, meta command.CommandMeta) error {
	if opts.Method == "" {
		return fmt.Errorf("method required")
	}
	if _, exists := f.handlers[opts.Method]; exists {
		return fmt.Errorf("method already registered: %s", opts.Method)
	}
	f.handlers[opts.Method] = registeredHandler{
		opts:    opts,
		handler: handler,
		meta:    meta,
	}
	return nil
}

func setupRPCController(t *testing.T) (*crud.Controller[*rpcUser], repository.Repository[*rpcUser], *bun.DB) {
	t.Helper()

	sqldb, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		_ = db.Close()
		_ = sqldb.Close()
	})

	_, err = db.NewCreateTable().Model((*rpcUser)(nil)).IfNotExists().Exec(context.Background())
	require.NoError(t, err)

	repo := repository.NewRepository(db, repository.ModelHandlers[*rpcUser]{
		NewRecord: func() *rpcUser { return &rpcUser{} },
		GetID:     func(record *rpcUser) uuid.UUID { return record.ID },
		SetID:     func(record *rpcUser, id uuid.UUID) { record.ID = id },
		GetIdentifier: func() string {
			return "Email"
		},
	})

	controller := crud.NewController[*rpcUser](
		repo,
		crud.WithScopeGuard[*rpcUser](func(ctx crud.Context, _ crud.CrudOperation) (crud.ActorContext, crud.ScopeFilter, error) {
			actor := crud.ActorFromContext(ctx.UserContext())
			if actor.ActorID == "" {
				return crud.ActorContext{}, crud.ScopeFilter{}, fmt.Errorf("actor required")
			}
			return actor, crud.ScopeFilter{}, nil
		}),
	)

	return controller, repo, db
}

func mustHandler[T any](t *testing.T, registrar *fakeRegistrar, method string) T {
	t.Helper()
	entry, ok := registrar.handlers[method]
	require.True(t, ok, "missing handler for method %s", method)
	handler, ok := entry.handler.(T)
	require.True(t, ok, "unexpected handler type for method %s", method)
	return handler
}

func TestRegisterResourceEndpointsRegistersExpectedMethods(t *testing.T) {
	controller, _, _ := setupRPCController(t)
	registrar := newFakeRegistrar()

	err := RegisterResourceEndpoints(registrar, controller, ResourceRegistrationOptions{
		Resource: "user",
	})
	require.NoError(t, err)

	assert.Contains(t, registrar.handlers, "crud.user.create")
	assert.Contains(t, registrar.handlers, "crud.user.create_batch")
	assert.Contains(t, registrar.handlers, "crud.user.show")
	assert.Contains(t, registrar.handlers, "crud.user.index")
	assert.Contains(t, registrar.handlers, "crud.user.update")
	assert.Contains(t, registrar.handlers, "crud.user.update_batch")
	assert.Contains(t, registrar.handlers, "crud.user.delete")
	assert.Contains(t, registrar.handlers, "crud.user.delete_batch")
}

func TestRegisterResourceEndpointsInfersResourceNameForPointerModels(t *testing.T) {
	controller, _, _ := setupRPCController(t)
	registrar := newFakeRegistrar()

	require.NotPanics(t, func() {
		err := RegisterResourceEndpoints(registrar, controller, ResourceRegistrationOptions{})
		require.NoError(t, err)
	})

	resource, _ := crud.GetResourceName(reflect.TypeFor[*rpcUser]())
	assert.Contains(t, registrar.handlers, "crud."+resource+".create")
	assert.Contains(t, registrar.handlers, "crud."+resource+".index")
}

func TestRegisterResourceEndpointsRoundTrip(t *testing.T) {
	controller, repo, _ := setupRPCController(t)
	registrar := newFakeRegistrar()

	err := RegisterResourceEndpoints(registrar, controller, ResourceRegistrationOptions{
		Resource: "user",
	})
	require.NoError(t, err)

	create := mustHandler[func(context.Context, RequestEnvelope[CreateData[*rpcUser]]) (ResponseEnvelope[*rpcUser], error)](
		t, registrar, "crud.user.create",
	)

	createOut, err := create(context.Background(), RequestEnvelope[CreateData[*rpcUser]]{
		Data: CreateData[*rpcUser]{
			Record: &rpcUser{
				ID:        uuid.New(),
				Name:      "Alice",
				Email:     "alice@example.com",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		},
		Meta: RequestMeta{
			ActorID: "actor-1",
			Roles:   []string{"admin"},
			Tenant:  "acme",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.Data)
	assert.Equal(t, "Alice", createOut.Data.Name)

	show := mustHandler[func(context.Context, RequestEnvelope[ShowData]) (ResponseEnvelope[*rpcUser], error)](
		t, registrar, "crud.user.show",
	)
	showOut, err := show(context.Background(), RequestEnvelope[ShowData]{
		Data: ShowData{ID: createOut.Data.ID.String()},
		Meta: RequestMeta{ActorID: "actor-1"},
	})
	require.NoError(t, err)
	require.NotNil(t, showOut.Data)
	assert.Equal(t, createOut.Data.Email, showOut.Data.Email)

	index := mustHandler[func(context.Context, RequestEnvelope[IndexData[crud.ListQueryOptions]]) (ResponseEnvelope[ListResult[*rpcUser]], error)](
		t, registrar, "crud.user.index",
	)
	indexOut, err := index(context.Background(), RequestEnvelope[IndexData[crud.ListQueryOptions]]{
		Data: IndexData[crud.ListQueryOptions]{
			Options: crud.ListQueryOptions{Limit: 10, Offset: 0},
		},
		Meta: RequestMeta{ActorID: "actor-1"},
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, indexOut.Data.Count, 1)

	update := mustHandler[func(context.Context, RequestEnvelope[UpdateData[*rpcUser]]) (ResponseEnvelope[*rpcUser], error)](
		t, registrar, "crud.user.update",
	)
	updateOut, err := update(context.Background(), RequestEnvelope[UpdateData[*rpcUser]]{
		Data: UpdateData[*rpcUser]{
			ID: createOut.Data.ID.String(),
			Record: &rpcUser{
				Name: "Alice Updated",
			},
		},
		Meta: RequestMeta{ActorID: "actor-1"},
	})
	require.NoError(t, err)
	require.NotNil(t, updateOut.Data)
	assert.Equal(t, "Alice Updated", updateOut.Data.Name)

	deleteOne := mustHandler[func(context.Context, RequestEnvelope[DeleteData]) (ResponseEnvelope[DeleteResult], error)](
		t, registrar, "crud.user.delete",
	)
	deleteOut, err := deleteOne(context.Background(), RequestEnvelope[DeleteData]{
		Data: DeleteData{ID: createOut.Data.ID.String()},
		Meta: RequestMeta{ActorID: "actor-1"},
	})
	require.NoError(t, err)
	assert.True(t, deleteOut.Data.Deleted)

	createBatch := mustHandler[func(context.Context, RequestEnvelope[CreateBatchData[*rpcUser]]) (ResponseEnvelope[ListResult[*rpcUser]], error)](
		t, registrar, "crud.user.create_batch",
	)
	batchOut, err := createBatch(context.Background(), RequestEnvelope[CreateBatchData[*rpcUser]]{
		Data: CreateBatchData[*rpcUser]{
			Records: []*rpcUser{
				{
					ID:        uuid.New(),
					Name:      "Bob",
					Email:     "bob@example.com",
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				},
				{
					ID:        uuid.New(),
					Name:      "Carol",
					Email:     "carol@example.com",
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				},
			},
		},
		Meta: RequestMeta{ActorID: "actor-1"},
	})
	require.NoError(t, err)
	require.Len(t, batchOut.Data.Items, 2)

	deleteBatch := mustHandler[func(context.Context, RequestEnvelope[DeleteBatchData[*rpcUser]]) (ResponseEnvelope[DeleteBatchResult], error)](
		t, registrar, "crud.user.delete_batch",
	)
	deleteBatchOut, err := deleteBatch(context.Background(), RequestEnvelope[DeleteBatchData[*rpcUser]]{
		Data: DeleteBatchData[*rpcUser]{
			IDs: []string{
				batchOut.Data.Items[0].ID.String(),
				batchOut.Data.Items[1].ID.String(),
			},
		},
		Meta: RequestMeta{ActorID: "actor-1"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, deleteBatchOut.Data.Count)

	records, _, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 0)
}
