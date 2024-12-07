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

type Filters struct {
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
	Count   int      `json:"count"`
	Order   []Order  `json:"order,omitempty"`
	Fields  []string `json:"fields,omitempty"`
	Include []string `json:"include,omitempty"`
}

type Order struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

// ResponseHandler defines how controller responses are handled
type ResponseHandler[T any] interface {
	OnError(ctx Context, err error, op CrudOperation) error
	OnData(ctx Context, data T, op CrudOperation) error
	OnEmpty(ctx Context, op CrudOperation) error
	OnList(ctx Context, data []T, op CrudOperation, filters *Filters) error
}

type DefaultResponseHandler[T any] struct{}

func NewDefaultResponseHandler[T any]() ResponseHandler[T] {
	return DefaultResponseHandler[T]{}
}

func (h DefaultResponseHandler[T]) OnError(c Context, err error, op CrudOperation) error {
	switch err.(type) {
	case *NotFoundError:
		return c.Status(http.StatusNotFound).JSON(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	case *ValidationError:
		return c.Status(http.StatusBadRequest).JSON(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	default:
		return c.Status(http.StatusInternalServerError).JSON(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}
}

func (h DefaultResponseHandler[T]) OnData(c Context, data T, op CrudOperation) error {
	if op == OpCreate {
		return c.Status(http.StatusCreated).JSON(data)
	}

	return c.Status(http.StatusOK).JSON(map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

func (h DefaultResponseHandler[T]) OnEmpty(c Context, op CrudOperation) error {
	return c.SendStatus(http.StatusNoContent)
}

func (h DefaultResponseHandler[T]) OnList(c Context, data []T, op CrudOperation, filters *Filters) error {
	return c.Status(http.StatusOK).JSON(map[string]interface{}{
		"success": true,
		"data":    data,
		"$meta":   filters,
	})
}
