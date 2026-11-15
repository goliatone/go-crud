package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWebHostContracts(t *testing.T) {
	for _, mode := range []string{"native", "socket"} {
		t.Run(mode, func(t *testing.T) {
			app := testApp(t)
			base := "http://example.test"
			jar, err := cookiejar.New(nil)
			require.NoError(t, err)
			var do func(*http.Request) (*http.Response, error)
			if mode == "native" {
				app.server.Init()
				do = func(req *http.Request) (*http.Response, error) { return app.server.WrappedRouter().Test(req) }
			} else {
				run, cancel, done := startObserved(t, app.server.(router.ManagedServer), 3*time.Second, nil)
				await(t, run.Serving())
				base = "http://" + run.Address()
				client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
				do = client.Do
				t.Cleanup(func() { client.CloseIdleConnections(); cancel(); require.NoError(t, receiveResult(t, done)) })
			}
			request := func(method, path, body, contentType string) (int, []byte, http.Header) {
				t.Helper()
				req := httptest.NewRequest(method, base+path, strings.NewReader(body))
				req.RequestURI = ""
				if contentType != "" {
					req.Header.Set("Content-Type", contentType)
				}
				for _, c := range jar.Cookies(req.URL) {
					req.AddCookie(c)
				}
				resp, err := do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				jar.SetCookies(req.URL, resp.Cookies())
				data, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				return resp.StatusCode, data, resp.Header
			}
			jsonRequest := func(method, path, body string) (int, []byte) {
				status, data, h := request(method, path, body, "application/json")
				if status != 204 {
					require.Contains(t, h.Get("Content-Type"), "json")
				}
				return status, data
			}
			for _, path := range []string{"/", "/users/new", "/meta/docs"} {
				status, body, h := request("GET", path, "", "")
				require.Equal(t, 200, status, string(body))
				require.Contains(t, h.Get("Content-Type"), "text/html")
				require.NotEmpty(t, body)
			}
			for _, asset := range []struct{ path, kind string }{{"/static/css/main.css", "text/css"}, {"/static/js/app.js", "javascript"}} {
				status, body, h := request("GET", asset.path, "", "")
				require.Equal(t, 200, status, string(body))
				require.NotEmpty(t, body)
				require.Contains(t, h.Get("Content-Type"), asset.kind)
				status, body, _ = request("HEAD", asset.path, "", "")
				require.Equal(t, 200, status, string(body))
				require.Empty(t, body)
			}
			status, _, _ := request("GET", "/static/missing.css", "", "")
			require.Equal(t, 404, status)
			status, data := jsonRequest("GET", "/api/user/schema", "")
			require.Equal(t, 200, status, string(data))
			require.Contains(t, string(data), "#/components/parameters/Limit")
			status, data = jsonRequest("GET", "/api/users?order=name%20asc", "")
			require.Equal(t, 200, status, string(data))
			var list crud.APIListResponse[*User]
			require.NoError(t, json.Unmarshal(data, &list))
			require.Len(t, list.Data, 2)
			for _, u := range list.Data {
				require.Equal(t, "tenant-alpha", u.TenantID)
			}
			status, data = jsonRequest("GET", "/api/users?tenant_id=tenant-beta", "")
			require.Equal(t, 200, status, string(data))
			require.NoError(t, json.Unmarshal(data, &list))
			require.Len(t, list.Data, 1)
			beta := list.Data[0]
			status, _ = jsonRequest("PUT", "/api/user/"+beta.ID.String(), `{"name":"denied cross tenant"}`)
			require.Equal(t, 404, status)
			unchanged, err := app.repo.GetByID(context.Background(), beta.ID.String())
			require.NoError(t, err)
			require.Equal(t, beta.Name, unchanged.Name)
			status, data = jsonRequest("POST", "/api/user", `{"name":"API Created","email":"api@example.com","tenant_id":"tenant-alpha","active":true}`)
			require.Equal(t, 201, status, string(data))
			var created User
			require.NoError(t, json.Unmarshal(data, &created))
			require.NotEqual(t, uuid.Nil, created.ID)
			status, data = jsonRequest("PUT", "/api/user/"+created.ID.String(), `{"name":"API Updated"}`)
			require.Equal(t, 200, status, string(data))
			var updated crud.APIResponse[*User]
			require.NoError(t, json.Unmarshal(data, &updated))
			require.Equal(t, "API Updated", updated.Data.Name)
			require.Equal(t, created.Email, updated.Data.Email)
			status, _ = jsonRequest("DELETE", "/api/user/"+created.ID.String(), "")
			require.Equal(t, 204, status)
			status, data = jsonRequest("GET", "/api/user/"+created.ID.String(), "")
			require.Equal(t, 404, status)
			require.Contains(t, string(data), "not_found")
			status, data = jsonRequest("POST", "/api/user/batch", `[{"name":"Batch A","email":"batch-a@example.com","tenant_id":"tenant-alpha"},{"name":"Batch B","email":"batch-b@example.com","tenant_id":"tenant-alpha"}]`)
			require.Equal(t, 200, status, string(data))
			var batch crud.APIListResponse[*User]
			require.NoError(t, json.Unmarshal(data, &batch))
			require.Len(t, batch.Data, 2)
			batch.Data[0].Name = "Batch Updated"
			payload, err := json.Marshal(batch.Data)
			require.NoError(t, err)
			status, data = jsonRequest("PUT", "/api/user/batch", string(payload))
			require.Equal(t, 200, status, string(data))
			require.NoError(t, json.Unmarshal(data, &batch))
			require.Len(t, batch.Data, 2)
			require.Equal(t, "Batch Updated", batch.Data[0].Name)
			payload, err = json.Marshal([]string{batch.Data[0].ID.String(), batch.Data[1].ID.String()})
			require.NoError(t, err)
			status, data = jsonRequest("DELETE", "/api/user/batch", string(payload))
			require.Equal(t, 204, status, string(data))
			// HTML remains the original unscoped demonstration, with flash data consumed once.
			before, err := app.repo.Count(context.Background())
			require.NoError(t, err)
			status, data, _ = request("POST", "/users/", "name=&email=", "application/x-www-form-urlencoded")
			require.Equal(t, 200, status, string(data))
			require.Contains(t, string(data), "Name is required")
			after, err := app.repo.Count(context.Background())
			require.NoError(t, err)
			require.Equal(t, before, after)
			form := url.Values{"name": {"HTML Created"}, "email": {"html@example.com"}, "bio": {"Form body"}}
			status, data, h := request("POST", "/users/", form.Encode(), "application/x-www-form-urlencoded")
			require.Equal(t, 302, status, string(data))
			location := h.Get("Location")
			require.True(t, strings.HasPrefix(location, "/users/"))
			require.Contains(t, h.Get("Set-Cookie"), "router-app-flash")
			status, data, _ = request("GET", location, "", "")
			require.Equal(t, 200, status, string(data))
			require.Contains(t, string(data), "HTML Created")
			require.Contains(t, string(data), "has been created successfully")
			_, data, _ = request("GET", location, "", "")
			require.NotContains(t, string(data), "has been created successfully")
			status, data, _ = request("GET", location+"/edit", "", "")
			require.Equal(t, 200, status, string(data))
			require.Contains(t, string(data), "html@example.com")
			status, _, _ = request("POST", location, url.Values{"name": {"HTML Updated"}, "bio": {"Updated body"}}.Encode(), "application/x-www-form-urlencoded")
			require.Equal(t, 302, status)
			_, data, _ = request("GET", location, "", "")
			require.Contains(t, string(data), "HTML Updated")
			require.Contains(t, string(data), "has been updated successfully")
			status, _, _ = request("POST", location+"/delete", "", "application/x-www-form-urlencoded")
			require.Equal(t, 302, status)
			_, data, _ = request("GET", "/", "", "")
			require.Contains(t, string(data), "has been deleted successfully")
			_, data, _ = request("GET", "/users/"+uuid.NewString(), "", "")
			require.Contains(t, string(data), "User Not Found")
		})
	}
}
