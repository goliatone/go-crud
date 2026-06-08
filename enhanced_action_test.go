package crud

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

type enhancedRequestContext struct {
	*stubContext
	headers map[string]string
}

func newEnhancedRequestContext(headers map[string]string) *enhancedRequestContext {
	return &enhancedRequestContext{
		stubContext: newStubContext(),
		headers:     headers,
	}
}

func (c *enhancedRequestContext) Header(key string) string {
	if c == nil {
		return ""
	}
	return c.headers[key]
}

func TestIsEnhancedRequestDetectsExplicitHeader(t *testing.T) {
	assert.Equal(t, "X-Enhanced-Action", EnhancedRequestHeader)
	assert.Equal(t, "application/vnd.crud.enhanced+json", EnhancedMutationMediaType)

	ctx := newEnhancedRequestContext(map[string]string{
		EnhancedRequestHeader: EnhancedRequestHeaderValue,
		"Accept":              "application/json",
	})

	assert.True(t, IsEnhancedRequest(ctx))
	assert.True(t, DetectMutationRequest(ctx).Enhanced)
	assert.Equal(t, MutationResponseModeEnhanced, DetectMutationRequest(ctx).Mode)
}

func TestIsEnhancedRequestDetectsVendorAccept(t *testing.T) {
	ctx := newEnhancedRequestContext(map[string]string{
		"Accept": "text/html, " + EnhancedMutationMediaType + "; version=1",
	})

	assert.True(t, IsEnhancedRequest(ctx))
	assert.True(t, AcceptsEnhancedMutation(ctx))
}

func TestDetectMutationRequestUsesCustomNegotiationConfig(t *testing.T) {
	cfg := MutationNegotiationConfig{
		EnhancedHeader:      "X-App-Action",
		EnhancedHeaderValue: "opaque-marker",
		EnhancedMediaTypes:  []string{"application/vnd.example.action+json"},
	}
	headerCtx := newEnhancedRequestContext(map[string]string{
		"X-App-Action": "opaque-marker",
		"Accept":       "application/json",
	})
	acceptCtx := newEnhancedRequestContext(map[string]string{
		"Accept": "text/html, application/vnd.example.action+json",
	})
	defaultCtx := newEnhancedRequestContext(map[string]string{
		EnhancedRequestHeader: EnhancedRequestHeaderValue,
		"Accept":              EnhancedMutationMediaType,
	})

	assert.Equal(t, MutationResponseModeEnhanced, DetectMutationRequestWithConfig(headerCtx, cfg).Mode)
	assert.True(t, IsEnhancedRequestWithConfig(acceptCtx, cfg))
	assert.True(t, AcceptsEnhancedMutationWithConfig(acceptCtx, cfg))
	assert.False(t, IsEnhancedRequestWithConfig(defaultCtx, cfg), "custom config should not accept unrelated default markers")
}

func TestDetectMutationRequestPreservesJSONCompatibility(t *testing.T) {
	ctx := newEnhancedRequestContext(map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
	})

	req := DetectMutationRequest(ctx)

	assert.False(t, req.Enhanced)
	assert.True(t, AcceptsJSON(ctx))
	assert.Equal(t, MutationResponseModeJSON, req.Mode)
}

func TestDetectMutationRequestDetectsNormalBrowserForm(t *testing.T) {
	ctx := newEnhancedRequestContext(map[string]string{
		"Accept":       "text/html,application/xhtml+xml",
		"Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
	})

	req := DetectMutationRequest(ctx)

	assert.False(t, req.Enhanced)
	assert.True(t, AcceptsHTML(ctx))
	assert.True(t, IsFormContentType(ctx))
	assert.Equal(t, MutationResponseModeHTML, req.Mode)
}

func TestMutationResponseCarriesTypedResultAndOptions(t *testing.T) {
	res := NewMutationResponse(
		map[string]string{"id": "user-1"},
		CrudOperation("action:assign"),
		WithMutationStatus[map[string]string](http.StatusAccepted),
		WithMutationMeta[map[string]string]("redirect", "/admin/users/user-1"),
	)

	assert.Equal(t, map[string]string{"id": "user-1"}, res.Data)
	assert.Equal(t, CrudOperation("action:assign"), res.Operation)
	assert.Equal(t, http.StatusAccepted, res.Status)
	assert.Equal(t, "/admin/users/user-1", res.Meta["redirect"])
}

func TestMutationNegotiationCanFollowSharedMutationPath(t *testing.T) {
	var calls int
	mutate := func(ctx Context) MutationResponse[string] {
		calls++
		return NewMutationResponse("done", CrudOperation("action:assign"))
	}

	normalCtx := newEnhancedRequestContext(map[string]string{
		"Accept":       "text/html",
		"Content-Type": "application/x-www-form-urlencoded",
	})
	enhancedCtx := newEnhancedRequestContext(map[string]string{
		EnhancedRequestHeader: EnhancedRequestHeaderValue,
		"Accept":              EnhancedMutationMediaType,
	})

	normalResult := mutate(normalCtx)
	normalReq := DetectMutationRequest(normalCtx)
	enhancedResult := mutate(enhancedCtx)
	enhancedReq := DetectMutationRequest(enhancedCtx)

	assert.Equal(t, 2, calls)
	assert.Equal(t, normalResult.Data, enhancedResult.Data)
	assert.Equal(t, MutationResponseModeHTML, normalReq.Mode)
	assert.Equal(t, MutationResponseModeEnhanced, enhancedReq.Mode)
}
