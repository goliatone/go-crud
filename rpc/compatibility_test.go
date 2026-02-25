package rpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	commandrpc "github.com/goliatone/go-command/rpc"
	"github.com/goliatone/go-router"
	"github.com/goliatone/go-router/rpcfiber"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type compatEnvelopeData struct {
	Name string `json:"name"`
}

func TestSharedRPCContractAliasesGoCommandTypes(t *testing.T) {
	assert.Equal(t, reflect.TypeFor[RequestMeta](), reflect.TypeFor[commandrpc.RequestMeta]())
	assert.Equal(t, reflect.TypeFor[RequestEnvelope[compatEnvelopeData]](), reflect.TypeFor[commandrpc.RequestEnvelope[compatEnvelopeData]]())
	assert.Equal(t, reflect.TypeFor[ResponseEnvelope[compatEnvelopeData]](), reflect.TypeFor[commandrpc.ResponseEnvelope[compatEnvelopeData]]())
	assert.Equal(t, reflect.TypeFor[Error](), reflect.TypeFor[commandrpc.Error]())
}

func TestRegisterResourceEndpointsCompatibleWithGoCommandRPCServer(t *testing.T) {
	controller, _, _ := setupRPCController(t)
	server := commandrpc.NewServer()

	err := RegisterResourceEndpoints(server, controller, ResourceRegistrationOptions{Resource: "user"})
	require.NoError(t, err)

	createMsg, err := server.NewRequestForMethod("crud.user.create")
	require.NoError(t, err)
	_, ok := createMsg.(*RequestEnvelope[CreateData[*rpcUser]])
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

func TestRegistrarInterfaceCompatibleWithGoCommandRPCServer(t *testing.T) {
	var _ Registrar = (*commandrpc.Server)(nil)
}

func TestRegisterResourceEndpointsWithGoRouterFiberMount(t *testing.T) {
	controller, _, _ := setupRPCController(t)
	srv := commandrpc.NewServer()

	require.NoError(t, RegisterResourceEndpoints(srv, controller, ResourceRegistrationOptions{Resource: "user"}))

	adapter := router.NewFiberAdapter()
	r := adapter.Router()
	require.NoError(t, rpcfiber.MountFiber(r, srv))
	app := adapter.WrappedRouter()

	endpointsResp := testRPCRequest(t, app, http.MethodGet, "/api/rpc/endpoints", "", nil)
	require.Equal(t, http.StatusOK, endpointsResp.StatusCode)

	var discovery struct {
		Endpoints []commandrpc.Endpoint `json:"endpoints"`
	}
	decodeRPCResponse(t, endpointsResp, &discovery)
	assert.GreaterOrEqual(t, len(discovery.Endpoints), 8)
	assert.Contains(t, endpointMethods(discovery.Endpoints), "crud.user.create")
	assert.Contains(t, endpointMethods(discovery.Endpoints), "crud.user.show")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	createBody := `{"method":"crud.user.create","params":{"data":{"record":{"id":"` + uuid.NewString() + `","name":"Fiber User","email":"fiber-user@example.com","created_at":"` + now + `","updated_at":"` + now + `"}},"meta":{"actorId":"payload-actor","requestId":"req-payload"}}}`
	createResp := testRPCRequest(t, app, http.MethodPost, "/api/rpc?tenant=query-tenant", createBody, map[string]string{
		"X-Correlation-ID": "corr-header",
	})
	require.Equal(t, http.StatusOK, createResp.StatusCode)

	var createOut ResponseEnvelope[*rpcUser]
	decodeRPCResponse(t, createResp, &createOut)
	require.Nil(t, createOut.Error)
	require.NotNil(t, createOut.Data)
	assert.Equal(t, "Fiber User", createOut.Data.Name)

	showPayload := `{"method":"crud.user.show","params":{"data":{"id":"` + createOut.Data.ID.String() + `"},"meta":{"actorId":"payload-actor"}}}`
	showResp := testRPCRequest(t, app, http.MethodPost, "/api/rpc", showPayload, nil)
	require.Equal(t, http.StatusOK, showResp.StatusCode)

	var showOut ResponseEnvelope[*rpcUser]
	decodeRPCResponse(t, showResp, &showOut)
	require.Nil(t, showOut.Error)
	require.NotNil(t, showOut.Data)
	assert.Equal(t, createOut.Data.Email, showOut.Data.Email)
}

func endpointMethods(endpoints []commandrpc.Endpoint) []string {
	out := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		out = append(out, endpoint.Method)
	}
	return out
}

func testRPCRequest(
	t *testing.T,
	app interface {
		Test(req *http.Request, msTimeout ...int) (*http.Response, error)
	},
	method string,
	target string,
	body string,
	headers map[string]string,
) *http.Response {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)
	return resp
}

func decodeRPCResponse(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, target), string(raw))
}
