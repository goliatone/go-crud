package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	cmdrpc "github.com/goliatone/go-command/rpc"
	crudrpc "github.com/goliatone/go-crud/rpc"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRPCContracts(t *testing.T) {
	for _, mode := range []string{"native", "socket"} {
		t.Run(mode, func(t *testing.T) {
			app, err := NewApp()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, app.Close()) })
			// Test-only endpoint checks metadata forwarding through the production mount.
			require.NoError(t, app.rpc.RegisterEndpoint(cmdrpc.NewEndpoint[struct{}, cmdrpc.RequestMeta](cmdrpc.EndpointSpec{Method: "qualification.metadata"}, func(_ context.Context, req cmdrpc.RequestEnvelope[struct{}]) (cmdrpc.ResponseEnvelope[cmdrpc.RequestMeta], error) {
				return cmdrpc.ResponseEnvelope[cmdrpc.RequestMeta]{Data: req.Meta}, nil
			})))
			base := "http://example.test"
			var do func(*http.Request) (*http.Response, error)
			if mode == "native" {
				app.server.Init()
				do = func(req *http.Request) (*http.Response, error) { return app.server.WrappedRouter().Test(req) }
			} else {
				run, err := app.server.(router.ManagedServer).StartManaged(context.Background(), router.ManagedOptions{Address: "127.0.0.1:0", Network: "tcp4", DrainTimeout: 3 * time.Second})
				require.NoError(t, err)
				select {
				case <-run.Serving():
				case <-time.After(time.Second):
					t.Fatal("host did not serve")
				}
				base = "http://" + run.Address()
				client := &http.Client{Timeout: 3 * time.Second}
				do = client.Do
				t.Cleanup(func() {
					client.CloseIdleConnections()
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					require.NoError(t, run.Stop(ctx))
					require.NoError(t, run.Wait(ctx))
				})
			}
			request := func(method, path string, body []byte) (int, []byte, http.Header) {
				req := httptest.NewRequest(method, base+path, bytes.NewReader(body))
				req.RequestURI = ""
				if body != nil {
					req.Header.Set("Content-Type", "application/json")
				}
				resp, err := do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				data, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				return resp.StatusCode, data, resp.Header
			}
			for _, path := range []string{"/", "/public/app.js", "/public/styles.css"} {
				status, data, _ := request("GET", path, nil)
				require.Equal(t, 200, status)
				require.NotEmpty(t, data)
			}
			status, data, _ := request("HEAD", "/public/app.js", nil)
			require.Equal(t, 200, status)
			require.Empty(t, data)
			status, _, _ = request("GET", "/public/missing.js", nil)
			require.Equal(t, 404, status)
			status, data, _ = request("GET", "/api/rpc/endpoints", nil)
			require.Equal(t, 200, status)
			var discovery struct {
				Endpoints []struct {
					Method string `json:"method"`
				} `json:"endpoints"`
			}
			require.NoError(t, json.Unmarshal(data, &discovery))
			var methods []string
			for _, e := range discovery.Endpoints {
				if strings.HasPrefix(e.Method, "crud.user.") {
					methods = append(methods, e.Method)
				}
			}
			sort.Strings(methods)
			require.Equal(t, []string{"crud.user.create", "crud.user.create_batch", "crud.user.delete", "crud.user.delete_batch", "crud.user.index", "crud.user.show", "crud.user.update", "crud.user.update_batch"}, methods)
			meta := map[string]any{"actorId": "demo-admin", "tenant": "tenant-alpha", "requestId": "rpc-contract", "correlationId": "rpc-correlation"}
			invoke := func(method string, input any, m map[string]any) (int, cmdrpc.ResponseEnvelope[json.RawMessage]) {
				payload, err := json.Marshal(map[string]any{"method": method, "params": map[string]any{"data": input, "meta": m}})
				require.NoError(t, err)
				status, body, headers := request("POST", "/api/rpc", payload)
				require.Contains(t, headers.Get("Content-Type"), "application/json")
				var out cmdrpc.ResponseEnvelope[json.RawMessage]
				require.NoError(t, json.Unmarshal(body, &out), string(body))
				return status, out
			}
			ok := func(method string, input any) json.RawMessage {
				status, out := invoke(method, input, meta)
				require.Equal(t, 200, status, string(out.Data))
				require.Nil(t, out.Error, "%+v", out.Error)
				return out.Data
			}
			var echoed cmdrpc.RequestMeta
			require.NoError(t, json.Unmarshal(ok("qualification.metadata", struct{}{}), &echoed))
			require.Equal(t, "demo-admin", echoed.ActorID)
			require.Equal(t, "tenant-alpha", echoed.Tenant)
			require.Equal(t, "rpc-contract", echoed.RequestID)
			require.Equal(t, "rpc-correlation", echoed.CorrelationID)
			var list crudrpc.ListResult[*User]
			require.NoError(t, json.Unmarshal(ok("crud.user.index", map[string]any{"options": map[string]any{"limit": 10, "order": "name asc"}}), &list))
			require.Equal(t, 2, list.Count)
			require.Len(t, list.Items, 2)
			for _, u := range list.Items {
				require.Equal(t, "tenant-alpha", u.TenantID)
			}
			countBefore, err := app.repo.Count(t.Context())
			require.NoError(t, err)
			status, denied := invoke("crud.user.create", map[string]any{"record": map[string]any{"name": "denied", "email": "denied@example.com"}}, map[string]any{"tenant": "tenant-alpha"})
			require.NotNil(t, denied.Error)
			// Business failures retain the published RPC envelope/status contract.
			require.Equal(t, 200, status)
			countAfter, err := app.repo.Count(t.Context())
			require.NoError(t, err)
			require.Equal(t, countBefore, countAfter)
			var created User
			require.NoError(t, json.Unmarshal(ok("crud.user.create", map[string]any{"record": map[string]any{"name": "New User", "email": "new@example.com"}}), &created))
			require.NotEqual(t, uuid.Nil, created.ID)
			require.Equal(t, "tenant-alpha", created.TenantID)
			require.False(t, created.CreatedAt.IsZero())
			var shown User
			require.NoError(t, json.Unmarshal(ok("crud.user.show", map[string]any{"id": created.ID.String()}), &shown))
			require.Equal(t, created.Email, shown.Email)
			_, hidden := invoke("crud.user.show", map[string]any{"id": created.ID.String()}, map[string]any{"actorId": "beta-admin", "tenant": "tenant-beta"})
			require.NotNil(t, hidden.Error)
			var updated User
			require.NoError(t, json.Unmarshal(ok("crud.user.update", map[string]any{"id": created.ID.String(), "record": map[string]any{"name": "Updated"}}), &updated))
			require.Equal(t, "Updated", updated.Name)
			require.Equal(t, created.Email, updated.Email)
			var batch crudrpc.ListResult[*User]
			require.NoError(t, json.Unmarshal(ok("crud.user.create_batch", map[string]any{"records": []map[string]any{{"name": "Batch A", "email": "batch-a@example.com"}, {"name": "Batch B", "email": "batch-b@example.com"}}}), &batch))
			require.Len(t, batch.Items, 2)
			batch.Items[0].Name = "Batch Updated"
			require.NoError(t, json.Unmarshal(ok("crud.user.update_batch", map[string]any{"records": batch.Items}), &batch))
			require.Len(t, batch.Items, 2)
			require.Equal(t, "Batch Updated", batch.Items[0].Name)
			ids := []string{batch.Items[0].ID.String(), batch.Items[1].ID.String()}
			var deletedBatch crudrpc.DeleteBatchResult
			require.NoError(t, json.Unmarshal(ok("crud.user.delete_batch", map[string]any{"ids": ids}), &deletedBatch))
			require.Equal(t, 2, deletedBatch.Count)
			var deleted crudrpc.DeleteResult
			require.NoError(t, json.Unmarshal(ok("crud.user.delete", map[string]any{"id": created.ID.String()}), &deleted))
			require.True(t, deleted.Deleted)
			status, data, _ = request("GET", "/api/state", nil)
			require.Equal(t, 200, status)
			var state struct {
				State appState `json:"state"`
			}
			require.NoError(t, json.Unmarshal(data, &state))
			require.Equal(t, 3, state.State.Count)
			require.Equal(t, map[string]int{"tenant-alpha": 2, "tenant-beta": 1}, state.State.Tenants)
			for _, body := range [][]byte{[]byte(`{`), []byte(`{"method":"does.not.exist","params":{"data":{}}}`)} {
				status, data, _ := request("POST", "/api/rpc", body)
				require.GreaterOrEqual(t, status, 400)
				var out cmdrpc.ResponseEnvelope[json.RawMessage]
				require.NoError(t, json.Unmarshal(data, &out))
				require.NotNil(t, out.Error)
				require.NotContains(t, string(data), "sqlite")
			}
		})
	}
}
