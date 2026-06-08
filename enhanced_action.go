package crud

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	// EnhancedRequestHeader marks a request that wants an enhanced mutation response.
	EnhancedRequestHeader = "X-Enhanced-Action"
	// EnhancedRequestHeaderValue is the canonical truthy value for EnhancedRequestHeader.
	EnhancedRequestHeaderValue = "1"
	// EnhancedMutationMediaType is the vendor media type used by enhanced SSR actions.
	EnhancedMutationMediaType = "application/vnd.crud.enhanced+json"
)

// RequestHeaderProvider is implemented by Context adapters that can expose request headers.
type RequestHeaderProvider interface {
	Header(string) string
}

// MutationResponseMode describes the response shape a presenter should produce.
type MutationResponseMode string

const (
	MutationResponseModeJSON     MutationResponseMode = "json"
	MutationResponseModeHTML     MutationResponseMode = "html"
	MutationResponseModeEnhanced MutationResponseMode = "enhanced"
)

// MutationRequest captures transport-adjacent response negotiation for mutations.
type MutationRequest struct {
	Enhanced    bool
	Accept      string
	ContentType string
	Mode        MutationResponseMode
}

// MutationNegotiationConfig configures how a presenter detects enhanced
// mutation requests for a host application.
type MutationNegotiationConfig struct {
	EnhancedHeader      string
	EnhancedHeaderValue string
	EnhancedMediaTypes  []string
}

// DefaultMutationNegotiationConfig returns the generic enhanced-action markers
// used by go-crud helpers when no host-specific config is provided.
func DefaultMutationNegotiationConfig() MutationNegotiationConfig {
	return MutationNegotiationConfig{
		EnhancedHeader:      EnhancedRequestHeader,
		EnhancedHeaderValue: EnhancedRequestHeaderValue,
		EnhancedMediaTypes:  []string{EnhancedMutationMediaType},
	}
}

// MutationResponse carries a typed mutation result plus transport-adjacent options.
// Presentation layers such as go-admin decide how to turn this into redirects,
// fragments, JSON envelopes, or flashes.
type MutationResponse[T any] struct {
	Data      T
	Operation CrudOperation
	Status    int
	Meta      map[string]any
}

// MutationResponseOption updates MutationResponse metadata without changing handler signatures.
type MutationResponseOption[T any] func(*MutationResponse[T])

// NewMutationResponse constructs an additive mutation response contract for CRUD operations
// and custom actions.
func NewMutationResponse[T any](data T, op CrudOperation, opts ...MutationResponseOption[T]) MutationResponse[T] {
	res := MutationResponse[T]{
		Data:      data,
		Operation: op,
		Status:    http.StatusOK,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&res)
		}
	}
	return res
}

// WithMutationStatus sets the preferred HTTP status for a mutation response.
func WithMutationStatus[T any](status int) MutationResponseOption[T] {
	return func(res *MutationResponse[T]) {
		if status > 0 {
			res.Status = status
		}
	}
}

// WithMutationMeta adds presentation-neutral metadata for the caller's responder.
func WithMutationMeta[T any](key string, value any) MutationResponseOption[T] {
	return func(res *MutationResponse[T]) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if res.Meta == nil {
			res.Meta = map[string]any{}
		}
		res.Meta[key] = value
	}
}

// MutationResponder can be implemented by presentation packages that choose the final
// response shape after business logic has completed.
type MutationResponder[T any] interface {
	RespondMutation(Context, MutationRequest, MutationResponse[T]) error
	RespondMutationError(Context, MutationRequest, error) error
}

// DetectMutationRequest returns response-negotiation facts for a mutation request.
func DetectMutationRequest(ctx any) MutationRequest {
	return DetectMutationRequestWithConfig(ctx, DefaultMutationNegotiationConfig())
}

// DetectMutationRequestWithConfig returns response-negotiation facts using the
// provided enhanced request markers.
func DetectMutationRequestWithConfig(ctx any, cfg MutationNegotiationConfig) MutationRequest {
	accept := requestHeader(ctx, "Accept")
	contentType := requestHeader(ctx, "Content-Type")

	req := MutationRequest{
		Enhanced:    IsEnhancedRequestWithConfig(ctx, cfg),
		Accept:      accept,
		ContentType: contentType,
		Mode:        MutationResponseModeJSON,
	}
	switch {
	case req.Enhanced:
		req.Mode = MutationResponseModeEnhanced
	case acceptsMediaType(accept, "text/html") && isFormContentType(contentType):
		req.Mode = MutationResponseModeHTML
	}
	return req
}

// IsEnhancedRequest reports whether a request explicitly opts into enhanced mutation responses.
func IsEnhancedRequest(ctx any) bool {
	return IsEnhancedRequestWithConfig(ctx, DefaultMutationNegotiationConfig())
}

// IsEnhancedRequestWithConfig reports whether a request explicitly opts into
// enhanced mutation responses using the provided negotiation markers.
func IsEnhancedRequestWithConfig(ctx any, cfg MutationNegotiationConfig) bool {
	cfg = normalizeMutationNegotiationConfig(cfg)
	if header := strings.TrimSpace(cfg.EnhancedHeader); header != "" && enhancedHeaderMatches(requestHeader(ctx, header), cfg.EnhancedHeaderValue) {
		return true
	}
	return acceptsAnyMediaType(requestHeader(ctx, "Accept"), cfg.EnhancedMediaTypes)
}

// AcceptsEnhancedMutation reports whether the Accept header includes the enhanced media type.
func AcceptsEnhancedMutation(ctx any) bool {
	return AcceptsEnhancedMutationWithConfig(ctx, DefaultMutationNegotiationConfig())
}

// AcceptsEnhancedMutationWithConfig reports whether the Accept header includes
// one of the configured enhanced media types.
func AcceptsEnhancedMutationWithConfig(ctx any, cfg MutationNegotiationConfig) bool {
	cfg = normalizeMutationNegotiationConfig(cfg)
	return acceptsAnyMediaType(requestHeader(ctx, "Accept"), cfg.EnhancedMediaTypes)
}

// AcceptsJSON reports whether the Accept header includes application/json.
func AcceptsJSON(ctx any) bool {
	return acceptsMediaType(requestHeader(ctx, "Accept"), "application/json")
}

// AcceptsHTML reports whether the Accept header includes text/html.
func AcceptsHTML(ctx any) bool {
	return acceptsMediaType(requestHeader(ctx, "Accept"), "text/html")
}

// IsFormContentType reports whether Content-Type is a browser form submission.
func IsFormContentType(ctx any) bool {
	return isFormContentType(requestHeader(ctx, "Content-Type"))
}

func requestHeader(ctx any, key string) string {
	if ctx == nil {
		return ""
	}
	provider, ok := ctx.(RequestHeaderProvider)
	if !ok || provider == nil {
		return ""
	}
	return strings.TrimSpace(provider.Header(key))
}

func isTruthyHeader(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func enhancedHeaderMatches(value string, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" || expected == EnhancedRequestHeaderValue {
		return isTruthyHeader(value)
	}
	return strings.EqualFold(strings.TrimSpace(value), expected)
}

func normalizeMutationNegotiationConfig(cfg MutationNegotiationConfig) MutationNegotiationConfig {
	if strings.TrimSpace(cfg.EnhancedHeader) == "" {
		cfg.EnhancedHeader = EnhancedRequestHeader
	}
	if strings.TrimSpace(cfg.EnhancedHeaderValue) == "" {
		cfg.EnhancedHeaderValue = EnhancedRequestHeaderValue
	}
	if len(cfg.EnhancedMediaTypes) == 0 {
		cfg.EnhancedMediaTypes = []string{EnhancedMutationMediaType}
	}
	return cfg
}

func acceptsAnyMediaType(header string, mediaTypes []string) bool {
	for _, mediaType := range mediaTypes {
		if acceptsMediaType(header, mediaType) {
			return true
		}
	}
	return false
}

func acceptsMediaType(header, mediaType string) bool {
	header = strings.TrimSpace(header)
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if header == "" || mediaType == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		item := strings.TrimSpace(part)
		if idx := strings.Index(item, ";"); idx >= 0 {
			item = strings.TrimSpace(item[:idx])
		}
		item = strings.ToLower(item)
		if item == mediaType {
			return true
		}
	}
	return false
}

func isFormContentType(contentType string) bool {
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = contentType[:idx]
	}
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "application/x-www-form-urlencoded", "multipart/form-data":
		return true
	default:
		return false
	}
}
