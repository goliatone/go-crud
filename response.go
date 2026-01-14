package crud

import (
	"net/http"
)

type NotFoundError struct{ error }
type ValidationError struct{ error }

type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

type APIListResponse[T any] struct {
	Success bool     `json:"success"`
	Data    []T      `json:"data"`
	Meta    *Filters `json:"$meta"`
}

type RelationFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type RelationInfo struct {
	Name    string           `json:"name"`
	Filters []RelationFilter `json:"filters,omitempty"`
}

type Filters struct {
	Operation string         `json:"operation,omitempty"`
	Limit     int            `json:"limit,omitempty"`
	Offset    int            `json:"offset,omitempty"`
	Page      int            `json:"page,omitempty"`
	Adjusted  bool           `json:"adjusted,omitempty"`
	Count     int            `json:"count,omitempty"`
	Order     []Order        `json:"order,omitempty"`
	Fields    []string       `json:"fields,omitempty"`
	Include   []string       `json:"include,omitempty"`
	Relations []RelationInfo `json:"relations,omitempty"`
}

type Order struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

// ResponseHandler defines how controller responses are handled
type ResponseHandler[T any] interface {
	OnError(ctx Context, err error, op CrudOperation) error
	OnData(ctx Context, data T, op CrudOperation, filters ...*Filters) error
	OnEmpty(ctx Context, op CrudOperation) error
	OnList(ctx Context, data []T, op CrudOperation, filters *Filters) error
}

type errorEncoderAware interface {
	setErrorEncoder(ErrorEncoder)
}

type DefaultResponseHandler[T any] struct {
	encoder ErrorEncoder
}

func NewDefaultResponseHandler[T any]() ResponseHandler[T] {
	return &DefaultResponseHandler[T]{
		encoder: ProblemJSONErrorEncoder(),
	}
}

func (h *DefaultResponseHandler[T]) setErrorEncoder(encoder ErrorEncoder) {
	if h == nil {
		return
	}
	h.encoder = encoder
}

func (h *DefaultResponseHandler[T]) OnError(c Context, err error, op CrudOperation) error {
	if h == nil {
		return ProblemJSONErrorEncoder()(c, err, op)
	}
	if h.encoder == nil {
		h.encoder = ProblemJSONErrorEncoder()
	}
	return h.encoder(c, err, op)
}

func (h *DefaultResponseHandler[T]) OnData(c Context, data T, op CrudOperation, filters ...*Filters) error {
	if op == OpCreate {
		return c.Status(http.StatusCreated).JSON(data)
	}

	filter := &Filters{}
	if len(filters) > 0 {
		filter = filters[0]
	}

	return c.Status(http.StatusOK).JSON(map[string]any{
		"$meta":   filter,
		"success": true,
		"data":    data,
	})
}

func (h *DefaultResponseHandler[T]) OnEmpty(c Context, op CrudOperation) error {
	return c.SendStatus(http.StatusNoContent)
}

func (h *DefaultResponseHandler[T]) OnList(c Context, data []T, op CrudOperation, filters *Filters) error {
	return c.Status(http.StatusOK).JSON(map[string]any{
		"$meta":   filters,
		"data":    data,
		"success": true,
	})
}

type errorEncoderResponseHandler[T any] struct {
	base    ResponseHandler[T]
	encoder ErrorEncoder
}

func (h *errorEncoderResponseHandler[T]) setErrorEncoder(encoder ErrorEncoder) {
	h.encoder = encoder
}

func (h *errorEncoderResponseHandler[T]) encoderFunc() ErrorEncoder {
	if h.encoder != nil {
		return h.encoder
	}
	return ProblemJSONErrorEncoder()
}

func (h *errorEncoderResponseHandler[T]) OnError(c Context, err error, op CrudOperation) error {
	return h.encoderFunc()(c, err, op)
}

func (h *errorEncoderResponseHandler[T]) OnData(c Context, data T, op CrudOperation, filters ...*Filters) error {
	if h.base != nil {
		return h.base.OnData(c, data, op, filters...)
	}
	return NewDefaultResponseHandler[T]().OnData(c, data, op, filters...)
}

func (h *errorEncoderResponseHandler[T]) OnEmpty(c Context, op CrudOperation) error {
	if h.base != nil {
		return h.base.OnEmpty(c, op)
	}
	return NewDefaultResponseHandler[T]().OnEmpty(c, op)
}

func (h *errorEncoderResponseHandler[T]) OnList(c Context, data []T, op CrudOperation, filters *Filters) error {
	if h.base != nil {
		return h.base.OnList(c, data, op, filters)
	}
	return NewDefaultResponseHandler[T]().OnList(c, data, op, filters)
}
