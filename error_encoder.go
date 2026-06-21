package crud

import (
	stdErrors "errors"
	"net/http"
	"strings"
	"time"

	goerrors "github.com/goliatone/go-errors"
)

// ErrorEncoder serializes controller errors into HTTP responses.
type ErrorEncoder func(ctx Context, err error, op CrudOperation) error

// ErrorStatusResolver resolves the HTTP status code for a go-errors error.
type ErrorStatusResolver func(err *goerrors.Error, op CrudOperation) int

type problemJSONEncoderOption func(*problemJSONEncoderConfig)

type problemJSONEncoderConfig struct {
	includeStack   bool
	errorMappers   []goerrors.ErrorMapper
	statusResolver ErrorStatusResolver
	contentType    string
}

// ProblemJSONErrorEncoder returns an encoder that emits go-errors compatible
// RFC-7807/problem+json responses. The encoder inspects known error categories,
// maps them to HTTP status codes, and writes go-errors.ErrorResponse bodies.
func ProblemJSONErrorEncoder(opts ...problemJSONEncoderOption) ErrorEncoder {
	cfg := defaultProblemJSONEncoderConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx Context, err error, op CrudOperation) error {
		if err == nil {
			err = stdErrors.New("unknown error")
		}

		mapped := goerrors.MapToError(err, cfg.errorMappers)
		if mapped == nil {
			mapped = goerrors.New(err.Error(), goerrors.CategoryInternal)
		}

		status := cfg.statusResolver(mapped, op)
		if status <= 0 {
			status = http.StatusInternalServerError
		}

		mapped = mapped.WithCode(status)
		if strings.TrimSpace(mapped.TextCode) == "" {
			mapped = mapped.WithTextCode(goerrors.HTTPStatusToTextCode(status))
		}

		if mapped.Timestamp.IsZero() {
			mapped.Timestamp = time.Now().UTC()
		}

		includeStack := cfg.includeStack || goerrors.IsDevelopment
		if includeStack && len(mapped.StackTrace) == 0 {
			mapped = mapped.WithStackTrace()
		}

		attachErrorRequestMetadata(ctx, mapped, op)

		response := mapped.ToErrorResponse(includeStack, mapped.StackTrace)
		return ctx.Status(status).JSON(response, cfg.contentType)
	}
}

// WithProblemJSONIncludeStack configures whether stack traces should be serialized.
func WithProblemJSONIncludeStack(include bool) problemJSONEncoderOption {
	return func(cfg *problemJSONEncoderConfig) {
		cfg.includeStack = include
	}
}

// WithProblemJSONErrorMappers appends additional error mappers.
func WithProblemJSONErrorMappers(mappers ...goerrors.ErrorMapper) problemJSONEncoderOption {
	return func(cfg *problemJSONEncoderConfig) {
		if len(mappers) == 0 {
			return
		}
		cfg.errorMappers = append(cfg.errorMappers, mappers...)
	}
}

// WithProblemJSONStatusResolver overrides the status resolver used by the encoder.
func WithProblemJSONStatusResolver(resolver ErrorStatusResolver) problemJSONEncoderOption {
	return func(cfg *problemJSONEncoderConfig) {
		if resolver != nil {
			cfg.statusResolver = resolver
		}
	}
}

// WithProblemJSONContentType overrides the response content type (defaults to application/problem+json).
func WithProblemJSONContentType(contentType string) problemJSONEncoderOption {
	return func(cfg *problemJSONEncoderConfig) {
		if strings.TrimSpace(contentType) != "" {
			cfg.contentType = strings.TrimSpace(contentType)
		}
	}
}

// LegacyJSONErrorEncoder preserves the previous {success:false,error:string} payloads.
func LegacyJSONErrorEncoder() ErrorEncoder {
	return func(ctx Context, err error, _ CrudOperation) error {
		if err == nil {
			err = stdErrors.New("unknown error")
		}

		status := http.StatusInternalServerError
		var notFound *NotFoundError
		var validation *ValidationError
		switch {
		case stdErrors.As(err, &notFound):
			status = http.StatusNotFound
		case stdErrors.As(err, &validation):
			status = http.StatusBadRequest
		}

		return ctx.Status(status).JSON(map[string]any{
			"success": false,
			"error":   err.Error(),
		})
	}
}

func defaultProblemJSONEncoderConfig() problemJSONEncoderConfig {
	return problemJSONEncoderConfig{
		includeStack:   goerrors.IsDevelopment,
		errorMappers:   append(defaultErrorMappers(), goerrors.DefaultErrorMappers()...),
		statusResolver: defaultErrorStatusResolver,
		contentType:    "application/problem+json",
	}
}

func defaultErrorMappers() []goerrors.ErrorMapper {
	return []goerrors.ErrorMapper{
		mapCRUDTypedErrors,
	}
}

func mapCRUDTypedErrors(err error) *goerrors.Error {
	if err == nil {
		return nil
	}

	var notFound *NotFoundError
	if stdErrors.As(err, &notFound) {
		message := strings.TrimSpace(notFound.Error())
		if message == "" {
			message = "resource not found"
		}
		result := goerrors.New(message, goerrors.CategoryNotFound).
			WithCode(http.StatusNotFound).
			WithTextCode("NOT_FOUND")
		if source := embeddedSourceError(notFound.error); source != nil {
			result.Source = source
		}
		return result
	}

	var validation *ValidationError
	if stdErrors.As(err, &validation) {
		message := strings.TrimSpace(validation.Error())
		if message == "" {
			message = "validation failed"
		}
		result := goerrors.New(message, goerrors.CategoryValidation).
			WithCode(http.StatusUnprocessableEntity).
			WithTextCode("VALIDATION_ERROR")
		if source := embeddedSourceError(validation.error); source != nil {
			result.Source = source
		}
		return result
	}

	return nil
}

func embeddedSourceError(err error) error {
	if err == nil {
		return nil
	}
	return err
}

var errorStatusByCategory = map[goerrors.Category]int{
	goerrors.CategoryValidation:       http.StatusUnprocessableEntity,
	goerrors.CategoryAuth:             http.StatusUnauthorized,
	goerrors.CategoryAuthz:            http.StatusForbidden,
	goerrors.CategoryNotFound:         http.StatusNotFound,
	goerrors.CategoryConflict:         http.StatusConflict,
	goerrors.CategoryRateLimit:        http.StatusTooManyRequests,
	goerrors.CategoryBadInput:         http.StatusBadRequest,
	goerrors.CategoryMethodNotAllowed: http.StatusMethodNotAllowed,
	goerrors.CategoryCommand:          http.StatusBadRequest,
	goerrors.CategoryExternal:         http.StatusBadGateway,
	goerrors.CategoryOperation:        http.StatusInternalServerError,
	goerrors.CategoryMiddleware:       http.StatusInternalServerError,
	goerrors.CategoryRouting:          http.StatusNotFound,
	goerrors.CategoryHandler:          http.StatusInternalServerError,
}

func defaultErrorStatusResolver(err *goerrors.Error, _ CrudOperation) int {
	if err == nil {
		return http.StatusInternalServerError
	}

	if err.Code > 0 {
		return err.Code
	}

	if status, ok := errorStatusByCategory[err.Category]; ok {
		return status
	}
	return http.StatusInternalServerError
}

func attachErrorRequestMetadata(ctx Context, err *goerrors.Error, op CrudOperation) {
	if ctx == nil || err == nil {
		return
	}

	requestID := RequestIDFromContext(ctx.UserContext())
	if requestID != "" {
		err = err.WithRequestID(requestID)
	}

	correlationID := CorrelationIDFromContext(ctx.UserContext())
	if correlationID != "" {
		err = err.WithMetadata(map[string]any{
			"correlation_id": correlationID,
		})
	}

	if op != "" {
		err = err.WithMetadata(map[string]any{
			"operation": string(op),
		})
	}
}
