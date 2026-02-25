package rpc

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/goliatone/go-command"
	commandrpc "github.com/goliatone/go-command/rpc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type compatEnvelopeData struct {
	Name string `json:"name"`
}

func TestSharedRPCContractShapeCompatibility(t *testing.T) {
	t.Run("request meta", func(t *testing.T) {
		assertTypeShapeCompatible(
			t,
			reflect.TypeFor[RequestMeta](),
			reflect.TypeFor[commandrpc.RequestMeta](),
		)
	})

	t.Run("request envelope", func(t *testing.T) {
		assertTypeShapeCompatible(
			t,
			reflect.TypeFor[RequestEnvelope[compatEnvelopeData]](),
			reflect.TypeFor[commandrpc.RequestEnvelope[compatEnvelopeData]](),
		)
	})

	t.Run("response envelope", func(t *testing.T) {
		assertTypeShapeCompatible(
			t,
			reflect.TypeFor[ResponseEnvelope[compatEnvelopeData]](),
			reflect.TypeFor[commandrpc.ResponseEnvelope[compatEnvelopeData]](),
		)
	})

	t.Run("error envelope", func(t *testing.T) {
		assertTypeShapeCompatible(
			t,
			reflect.TypeFor[Error](),
			reflect.TypeFor[commandrpc.Error](),
		)
	})
}

func TestRegisterResourceEndpointsCompatibleWithGoCommandRPCServer(t *testing.T) {
	controller, _, _ := setupRPCController(t)
	server := commandrpc.NewServer()

	err := RegisterResourceEndpoints(server, controller, ResourceRegistrationOptions{Resource: "user"})
	require.NoError(t, err)

	createMsg, err := server.NewRequestForMethod("crud.user.create")
	require.NoError(t, err)
	_, ok := createMsg.(RequestEnvelope[CreateData[*rpcUser]])
	require.True(t, ok)

	now := time.Now().UTC()
	createPayload := RequestEnvelope[CreateData[*rpcUser]]{
		Data: CreateData[*rpcUser]{
			Record: &rpcUser{
				ID:        uuid.New(),
				Name:      "Integration User",
				Email:     "integration-user@example.com",
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		Meta: RequestMeta{ActorID: "actor-1"},
	}

	out, err := server.Invoke(context.Background(), "crud.user.create", createPayload)
	require.NoError(t, err)

	createRes, ok := out.(ResponseEnvelope[*rpcUser])
	require.True(t, ok)
	require.NotNil(t, createRes.Data)
	assert.Equal(t, "Integration User", createRes.Data.Name)

	showPayload := RequestEnvelope[ShowData]{
		Data: ShowData{ID: createRes.Data.ID.String()},
		Meta: RequestMeta{ActorID: "actor-1"},
	}

	out, err = server.Invoke(context.Background(), "crud.user.show", showPayload)
	require.NoError(t, err)

	showRes, ok := out.(ResponseEnvelope[*rpcUser])
	require.True(t, ok)
	require.NotNil(t, showRes.Data)
	assert.Equal(t, createRes.Data.Email, showRes.Data.Email)
}

func TestGoCommandServerRegisterSignatureCompatibility(t *testing.T) {
	var _ Registrar = (*commandrpc.Server)(nil)

	serverType := reflect.TypeFor[*commandrpc.Server]()
	method, ok := serverType.MethodByName("Register")
	require.True(t, ok)

	require.Equal(t, 4, method.Type.NumIn())
	assert.Equal(t, reflect.TypeFor[command.RPCConfig](), method.Type.In(1))
	assert.Equal(t, reflect.TypeFor[any](), method.Type.In(2))
	assert.Equal(t, reflect.TypeFor[command.CommandMeta](), method.Type.In(3))

	require.Equal(t, 1, method.Type.NumOut())
	assert.Equal(t, reflect.TypeFor[error](), method.Type.Out(0))
}

func TestGoCommandServerRegisterAcceptsGoCrudFunctionHandlerPath(t *testing.T) {
	server := commandrpc.NewServer()
	method := "compat.delete"

	handler := func(_ context.Context, _ RequestEnvelope[DeleteData]) (ResponseEnvelope[DeleteResult], error) {
		return ResponseEnvelope[DeleteResult]{Data: DeleteResult{Deleted: true}}, nil
	}

	err := server.Register(command.RPCConfig{Method: method}, handler, command.CommandMeta{})
	require.NoError(t, err)

	endpoint, ok := server.Endpoint(method)
	require.True(t, ok)
	assert.Equal(t, method, endpoint.Method)
	assert.Equal(t, commandrpc.HandlerKindQuery, endpoint.HandlerKind)

	out, err := server.Invoke(context.Background(), method, RequestEnvelope[DeleteData]{Data: DeleteData{ID: "1"}})
	require.NoError(t, err)

	res, ok := out.(ResponseEnvelope[DeleteResult])
	require.True(t, ok)
	assert.True(t, res.Data.Deleted)
}

func assertTypeShapeCompatible(t *testing.T, left reflect.Type, right reflect.Type) {
	t.Helper()
	require.NotNil(t, left)
	require.NotNil(t, right)

	if left.Kind() != right.Kind() {
		t.Fatalf("kind mismatch at root: %s != %s", left.Kind(), right.Kind())
	}

	assertTypeShapeCompatibleAtPath(t, left, right, left.String())
}

func assertTypeShapeCompatibleAtPath(t *testing.T, left reflect.Type, right reflect.Type, path string) {
	t.Helper()
	require.NotNil(t, left)
	require.NotNil(t, right)
	require.Equalf(t, left.Kind(), right.Kind(), "kind mismatch at %s", path)

	switch left.Kind() {
	case reflect.Struct:
		require.Equalf(t, left.NumField(), right.NumField(), "field count mismatch at %s", path)
		for i := range left.NumField() {
			lf := left.Field(i)
			rf := right.Field(i)
			require.Equalf(t, lf.Name, rf.Name, "field name mismatch at %s[%d]", path, i)
			require.Equalf(t, lf.Tag.Get("json"), rf.Tag.Get("json"), "json tag mismatch at %s.%s", path, lf.Name)
			require.Equalf(t, lf.Anonymous, rf.Anonymous, "anonymous field mismatch at %s.%s", path, lf.Name)
			assertTypeShapeCompatibleAtPath(t, lf.Type, rf.Type, path+"."+lf.Name)
		}
	case reflect.Pointer, reflect.Slice, reflect.Array:
		assertTypeShapeCompatibleAtPath(t, left.Elem(), right.Elem(), path+"[]")
	case reflect.Map:
		assertTypeShapeCompatibleAtPath(t, left.Key(), right.Key(), path+"<key>")
		assertTypeShapeCompatibleAtPath(t, left.Elem(), right.Elem(), path+"<value>")
	case reflect.Interface:
		require.Equalf(t, left.NumMethod(), right.NumMethod(), "interface method count mismatch at %s", path)
		for i := range left.NumMethod() {
			lm := left.Method(i)
			rm := right.Method(i)
			require.Equalf(t, lm.Name, rm.Name, "interface method mismatch at %s[%d]", path, i)
			assertTypeShapeCompatibleAtPath(t, lm.Type, rm.Type, path+"."+lm.Name)
		}
	case reflect.Func:
		require.Equalf(t, left.NumIn(), right.NumIn(), "func input mismatch at %s", path)
		require.Equalf(t, left.NumOut(), right.NumOut(), "func output mismatch at %s", path)
		for i := range left.NumIn() {
			assertTypeShapeCompatibleAtPath(t, left.In(i), right.In(i), path+".in")
		}
		for i := range left.NumOut() {
			assertTypeShapeCompatibleAtPath(t, left.Out(i), right.Out(i), path+".out")
		}
	default:
		require.Equalf(t, left.String(), right.String(), "type mismatch at %s", path)
	}
}
