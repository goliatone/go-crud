package crud

import (
	"context"
	"errors"
)

// ActionInputDecoder maps transport input from ActionContext into a typed command input.
type ActionInputDecoder[TResource any, TInput any] func(ActionContext[TResource]) (TInput, error)

// ActionCommandExecutor executes a transport-neutral command.
type ActionCommandExecutor[TInput any, TResult any] interface {
	Execute(context.Context, TInput) (TResult, error)
}

// ActionCommandFunc adapts a function into ActionCommandExecutor.
type ActionCommandFunc[TInput any, TResult any] func(context.Context, TInput) (TResult, error)

// Execute calls fn(ctx, input).
func (fn ActionCommandFunc[TInput, TResult]) Execute(ctx context.Context, input TInput) (TResult, error) {
	return fn(ctx, input)
}

// ActionMutationResponder turns a successful command result into a transport response.
type ActionMutationResponder[TResource any, TResult any] func(ActionContext[TResource], MutationRequest, MutationResponse[TResult]) error

// ActionMutationErrorResponder turns a command or decode error into a transport response.
type ActionMutationErrorResponder[TResource any] func(ActionContext[TResource], MutationRequest, error) error

// CommandBackedActionConfig configures a custom action that delegates business logic
// to a transport-neutral command.
type CommandBackedActionConfig[TResource any, TInput any, TResult any] struct {
	Decode  ActionInputDecoder[TResource, TInput]
	Command ActionCommandExecutor[TInput, TResult]
	Respond ActionMutationResponder[TResource, TResult]
	OnError ActionMutationErrorResponder[TResource]
	Options []MutationResponseOption[TResult]
}

// CommandBackedActionHandler builds an ActionHandler that decodes request input, executes
// one command, and delegates response presentation after the command returns.
func CommandBackedActionHandler[TResource any, TInput any, TResult any](
	cfg CommandBackedActionConfig[TResource, TInput, TResult],
) ActionHandler[TResource] {
	return func(actx ActionContext[TResource]) error {
		req := DetectMutationRequest(actx)
		fail := func(err error) error {
			if cfg.OnError != nil {
				return cfg.OnError(actx, req, err)
			}
			return err
		}

		if cfg.Decode == nil {
			return fail(errors.New("crud: command-backed action missing decoder"))
		}
		if cfg.Command == nil {
			return fail(errors.New("crud: command-backed action missing command executor"))
		}
		if cfg.Respond == nil {
			return fail(errors.New("crud: command-backed action missing responder"))
		}

		input, err := cfg.Decode(actx)
		if err != nil {
			return fail(err)
		}
		result, err := cfg.Command.Execute(actx.UserContext(), input)
		if err != nil {
			return fail(err)
		}

		response := NewMutationResponse(result, actx.Operation, cfg.Options...)
		return cfg.Respond(actx, req, response)
	}
}
