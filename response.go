package crud

import (
	"github.com/gofiber/fiber/v2"
)

// ResponseHandler defines how controller responses are handled
type ResponseHandler[T any] interface {
	// OnError handles any error responses
	OnError(ctx *fiber.Ctx, err error, op CrudOperation) error
	// OnData handles successful responses with data
	OnData(ctx *fiber.Ctx, data T, op CrudOperation) error
	// OnEmpty handles successful responses without data (e.g., DELETE)
	OnEmpty(ctx *fiber.Ctx, op CrudOperation) error
	// OnList handles successful list responses
	OnList(ctx *fiber.Ctx, data []T, op CrudOperation, count int) error
}

// DefaultResponseHandler provides the default response handling implementation
type DefaultResponseHandler[T any] struct{}

func NewDefaultResponseHandler[T any]() ResponseHandler[T] {
	return &DefaultResponseHandler[T]{}
}

func (h *DefaultResponseHandler[T]) OnError(ctx *fiber.Ctx, err error, op CrudOperation) error {
	switch err.(type) {
	case *NotFoundError:
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	case *ValidationError:
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	default:
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}
}

func (h *DefaultResponseHandler[T]) OnData(ctx *fiber.Ctx, data T, op CrudOperation) error {
	if op == OpCreate {
		return ctx.Status(fiber.StatusCreated).JSON(data)
	}

	return ctx.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

func (h *DefaultResponseHandler[T]) OnEmpty(ctx *fiber.Ctx, op CrudOperation) error {
	return ctx.SendStatus(fiber.StatusNoContent)
}

func (h *DefaultResponseHandler[T]) OnList(ctx *fiber.Ctx, data []T, op CrudOperation, total int) error {
	return ctx.JSON(fiber.Map{
		"success": true,
		"data":    data,
		"$meta": map[string]any{
			"count": total,
		},
	})
}

// Custom error types for better error handling
type NotFoundError struct{ error }
type ValidationError struct{ error }
