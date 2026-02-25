package rpc

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

	commandrpc "github.com/goliatone/go-command/rpc"
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

type fakeRegistrar struct {
	endpoints map[string]commandrpc.EndpointDefinition
}

func newFakeRegistrar() *fakeRegistrar {
	return &fakeRegistrar{endpoints: map[string]commandrpc.EndpointDefinition{}}
}

func (f *fakeRegistrar) RegisterEndpoint(def commandrpc.EndpointDefinition) error {
	if def == nil {
		return fmt.Errorf("endpoint definition required")
	}
	method := def.Spec().Method
	if method == "" {
		return fmt.Errorf("method required")
	}
	if _, exists := f.endpoints[method]; exists {
		return fmt.Errorf("method already registered: %s", method)
	}
	f.endpoints[method] = def
	return nil
}

func (f *fakeRegistrar) RegisterEndpoints(defs ...commandrpc.EndpointDefinition) error {
	for _, def := range defs {
		if err := f.RegisterEndpoint(def); err != nil {
			return err
		}
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

func mustEndpoint(t *testing.T, registrar *fakeRegistrar, method string) commandrpc.EndpointDefinition {
	t.Helper()
	entry, ok := registrar.endpoints[method]
	require.True(t, ok, "missing endpoint for method %s", method)
	return entry
}

func mustInvokeEndpoint[Req any, Res any](
	t *testing.T,
	def commandrpc.EndpointDefinition,
	req RequestEnvelope[Req],
) ResponseEnvelope[Res] {
	t.Helper()
	out, err := def.Invoke(context.Background(), &req)
	require.NoError(t, err)
	res, ok := out.(ResponseEnvelope[Res])
	require.True(t, ok)
	return res
}

func TestRegisterResourceEndpointsRegistersExpectedMethods(t *testing.T) {
	controller, _, _ := setupRPCController(t)
	registrar := newFakeRegistrar()

	err := RegisterResourceEndpoints(registrar, controller, ResourceRegistrationOptions{
		Resource: "user",
	})
	require.NoError(t, err)

	assert.Contains(t, registrar.endpoints, "crud.user.create")
	assert.Contains(t, registrar.endpoints, "crud.user.create_batch")
	assert.Contains(t, registrar.endpoints, "crud.user.show")
	assert.Contains(t, registrar.endpoints, "crud.user.index")
	assert.Contains(t, registrar.endpoints, "crud.user.update")
	assert.Contains(t, registrar.endpoints, "crud.user.update_batch")
	assert.Contains(t, registrar.endpoints, "crud.user.delete")
	assert.Contains(t, registrar.endpoints, "crud.user.delete_batch")

	assert.Equal(t, commandrpc.MethodKindCommand, mustEndpoint(t, registrar, "crud.user.create").Spec().Kind)
	assert.Equal(t, commandrpc.MethodKindQuery, mustEndpoint(t, registrar, "crud.user.show").Spec().Kind)
	assert.Equal(t, commandrpc.MethodKindQuery, mustEndpoint(t, registrar, "crud.user.index").Spec().Kind)
	assert.Equal(t, commandrpc.MethodKindCommand, mustEndpoint(t, registrar, "crud.user.delete").Spec().Kind)
}

func TestRegisterResourceEndpointsInfersResourceNameForPointerModels(t *testing.T) {
	controller, _, _ := setupRPCController(t)
	registrar := newFakeRegistrar()

	require.NotPanics(t, func() {
		err := RegisterResourceEndpoints(registrar, controller, ResourceRegistrationOptions{})
		require.NoError(t, err)
	})

	resource, _ := crud.GetResourceName(reflect.TypeFor[*rpcUser]())
	assert.Contains(t, registrar.endpoints, "crud."+resource+".create")
	assert.Contains(t, registrar.endpoints, "crud."+resource+".index")
}

func TestRegisterResourceEndpointsRoundTrip(t *testing.T) {
	controller, repo, _ := setupRPCController(t)
	registrar := newFakeRegistrar()

	err := RegisterResourceEndpoints(registrar, controller, ResourceRegistrationOptions{
		Resource: "user",
	})
	require.NoError(t, err)

	createEndpoint := mustEndpoint(t, registrar, "crud.user.create")
	createReq, ok := createEndpoint.NewRequest().(*RequestEnvelope[CreateData[*rpcUser]])
	require.True(t, ok)
	require.NotNil(t, createReq)

	*createReq = RequestEnvelope[CreateData[*rpcUser]]{
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
	}

	createOut, err := createEndpoint.Invoke(context.Background(), createReq)
	require.NoError(t, err)
	createRes, ok := createOut.(ResponseEnvelope[*rpcUser])
	require.True(t, ok)
	require.NotNil(t, createRes.Data)
	assert.Equal(t, "Alice", createRes.Data.Name)

	showRes := mustInvokeEndpoint[ShowData, *rpcUser](
		t,
		mustEndpoint(t, registrar, "crud.user.show"),
		RequestEnvelope[ShowData]{
			Data: ShowData{ID: createRes.Data.ID.String()},
			Meta: RequestMeta{ActorID: "actor-1"},
		},
	)
	require.NotNil(t, showRes.Data)
	assert.Equal(t, createRes.Data.Email, showRes.Data.Email)

	indexRes := mustInvokeEndpoint[IndexData[crud.ListQueryOptions], ListResult[*rpcUser]](
		t,
		mustEndpoint(t, registrar, "crud.user.index"),
		RequestEnvelope[IndexData[crud.ListQueryOptions]]{
			Data: IndexData[crud.ListQueryOptions]{
				Options: crud.ListQueryOptions{Limit: 10, Offset: 0},
			},
			Meta: RequestMeta{ActorID: "actor-1"},
		},
	)
	assert.GreaterOrEqual(t, indexRes.Data.Count, 1)

	updateRes := mustInvokeEndpoint[UpdateData[*rpcUser], *rpcUser](
		t,
		mustEndpoint(t, registrar, "crud.user.update"),
		RequestEnvelope[UpdateData[*rpcUser]]{
			Data: UpdateData[*rpcUser]{
				ID: createRes.Data.ID.String(),
				Record: &rpcUser{
					Name: "Alice Updated",
				},
			},
			Meta: RequestMeta{ActorID: "actor-1"},
		},
	)
	require.NotNil(t, updateRes.Data)
	assert.Equal(t, "Alice Updated", updateRes.Data.Name)

	deleteRes := mustInvokeEndpoint[DeleteData, DeleteResult](
		t,
		mustEndpoint(t, registrar, "crud.user.delete"),
		RequestEnvelope[DeleteData]{
			Data: DeleteData{ID: createRes.Data.ID.String()},
			Meta: RequestMeta{ActorID: "actor-1"},
		},
	)
	assert.True(t, deleteRes.Data.Deleted)

	createBatchRes := mustInvokeEndpoint[CreateBatchData[*rpcUser], ListResult[*rpcUser]](
		t,
		mustEndpoint(t, registrar, "crud.user.create_batch"),
		RequestEnvelope[CreateBatchData[*rpcUser]]{
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
		},
	)
	require.Len(t, createBatchRes.Data.Items, 2)

	deleteBatchRes := mustInvokeEndpoint[DeleteBatchData[*rpcUser], DeleteBatchResult](
		t,
		mustEndpoint(t, registrar, "crud.user.delete_batch"),
		RequestEnvelope[DeleteBatchData[*rpcUser]]{
			Data: DeleteBatchData[*rpcUser]{
				IDs: []string{
					createBatchRes.Data.Items[0].ID.String(),
					createBatchRes.Data.Items[1].ID.String(),
				},
			},
			Meta: RequestMeta{ActorID: "actor-1"},
		},
	)
	assert.Equal(t, 2, deleteBatchRes.Data.Count)

	records, _, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 0)
}
