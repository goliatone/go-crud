package crud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testActionInput struct {
	ID    string
	Actor string
}

type testActionResult struct {
	ID     string
	Status string
}

func TestCommandBackedActionHandlerExecutesCommandAndResponds(t *testing.T) {
	var decoded bool
	var commandInput testActionInput
	var response MutationResponse[testActionResult]
	var request MutationRequest

	handler := CommandBackedActionHandler(CommandBackedActionConfig[*TestUser, testActionInput, testActionResult]{
		Decode: func(actx ActionContext[*TestUser]) (testActionInput, error) {
			decoded = true
			return testActionInput{
				ID:    actx.Params("id"),
				Actor: actx.Actor.ActorID,
			}, nil
		},
		Command: ActionCommandFunc[testActionInput, testActionResult](
			func(_ context.Context, input testActionInput) (testActionResult, error) {
				commandInput = input
				return testActionResult{ID: input.ID, Status: "deactivated"}, nil
			},
		),
		Respond: func(_ ActionContext[*TestUser], req MutationRequest, res MutationResponse[testActionResult]) error {
			request = req
			response = res
			return nil
		},
		Options: []MutationResponseOption[testActionResult]{
			WithMutationStatus[testActionResult](http.StatusAccepted),
			WithMutationMeta[testActionResult]("redirect", "/admin/users/user-1"),
		},
	})

	ctx := newActionTestContext(map[string]string{
		"id": "user-1",
	}, map[string]string{
		EnhancedRequestHeader: EnhancedRequestHeaderValue,
		"Accept":              EnhancedMutationMediaType,
	})

	err := handler(ActionContext[*TestUser]{
		Context:   ctx,
		Actor:     ActorContext{ActorID: "actor-1"},
		Operation: CrudOperation("action:deactivate"),
	})

	require.NoError(t, err)
	assert.True(t, decoded)
	assert.Equal(t, testActionInput{ID: "user-1", Actor: "actor-1"}, commandInput)
	assert.Equal(t, MutationResponseModeEnhanced, request.Mode)
	assert.Equal(t, testActionResult{ID: "user-1", Status: "deactivated"}, response.Data)
	assert.Equal(t, CrudOperation("action:deactivate"), response.Operation)
	assert.Equal(t, http.StatusAccepted, response.Status)
	assert.Equal(t, "/admin/users/user-1", response.Meta["redirect"])
}

func TestCommandBackedActionHandlerRoutesDecodeErrorsToResponder(t *testing.T) {
	decodeErr := errors.New("invalid action input")
	var gotErr error
	var gotReq MutationRequest

	handler := CommandBackedActionHandler(CommandBackedActionConfig[*TestUser, testActionInput, testActionResult]{
		Decode: func(ActionContext[*TestUser]) (testActionInput, error) {
			return testActionInput{}, decodeErr
		},
		Command: ActionCommandFunc[testActionInput, testActionResult](
			func(context.Context, testActionInput) (testActionResult, error) {
				t.Fatal("command should not execute after decode failure")
				return testActionResult{}, nil
			},
		),
		Respond: func(ActionContext[*TestUser], MutationRequest, MutationResponse[testActionResult]) error {
			t.Fatal("success responder should not run after decode failure")
			return nil
		},
		OnError: func(_ ActionContext[*TestUser], req MutationRequest, err error) error {
			gotReq = req
			gotErr = err
			return nil
		},
	})

	ctx := newActionTestContext(nil, map[string]string{
		"Accept":       "text/html",
		"Content-Type": "application/x-www-form-urlencoded",
	})

	err := handler(ActionContext[*TestUser]{Context: ctx})

	require.NoError(t, err)
	assert.ErrorIs(t, gotErr, decodeErr)
	assert.Equal(t, MutationResponseModeHTML, gotReq.Mode)
}

func TestCommandBackedActionHandlerReturnsConfigurationErrorWithoutErrorResponder(t *testing.T) {
	handler := CommandBackedActionHandler(CommandBackedActionConfig[*TestUser, testActionInput, testActionResult]{})

	err := handler(ActionContext[*TestUser]{Context: newActionTestContext(nil, nil)})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing decoder")
}

func TestControllerCommandBackedActionHandlerPreservesActionContext(t *testing.T) {
	var guardOps []CrudOperation
	var commandInput testActionInput

	action := Action[*TestUser]{
		Name:   "Deactivate",
		Method: http.MethodPost,
		Target: ActionTargetResource,
		Handler: CommandBackedActionHandler(CommandBackedActionConfig[*TestUser, testActionInput, testActionResult]{
			Decode: func(actx ActionContext[*TestUser]) (testActionInput, error) {
				return testActionInput{
					ID:    actx.Params("id"),
					Actor: actx.Actor.ActorID,
				}, nil
			},
			Command: ActionCommandFunc[testActionInput, testActionResult](
				func(_ context.Context, input testActionInput) (testActionResult, error) {
					commandInput = input
					return testActionResult{ID: input.ID, Status: "deactivated"}, nil
				},
			),
			Respond: func(actx ActionContext[*TestUser], req MutationRequest, res MutationResponse[testActionResult]) error {
				return actx.Status(http.StatusAccepted).JSON(map[string]any{
					"action": actx.Action.Slug,
					"mode":   req.Mode,
					"result": res.Data,
				})
			},
		}),
	}

	guard := func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
		guardOps = append(guardOps, op)
		return ActorContext{ActorID: "actor-action"}, ScopeFilter{}, nil
	}

	app, db := setupApp(t,
		WithActions(action),
		WithScopeGuard[*TestUser](guard),
	)
	defer db.Close()

	id := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/test-user/%s/actions/deactivate", id), nil)
	req.Header.Set(EnhancedRequestHeader, EnhancedRequestHeaderValue)
	req.Header.Set("Accept", EnhancedMutationMediaType)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var payload struct {
		Action string               `json:"action"`
		Mode   MutationResponseMode `json:"mode"`
		Result testActionResult     `json:"result"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.Equal(t, []CrudOperation{CrudOperation("action:deactivate")}, guardOps)
	assert.Equal(t, testActionInput{ID: id, Actor: "actor-action"}, commandInput)
	assert.Equal(t, "deactivate", payload.Action)
	assert.Equal(t, MutationResponseModeEnhanced, payload.Mode)
	assert.Equal(t, testActionResult{ID: id, Status: "deactivated"}, payload.Result)
}

type actionTestContext struct {
	*stubContext
	params  map[string]string
	headers map[string]string
}

func newActionTestContext(params, headers map[string]string) *actionTestContext {
	if params == nil {
		params = map[string]string{}
	}
	if headers == nil {
		headers = map[string]string{}
	}
	return &actionTestContext{
		stubContext: newStubContext(),
		params:      params,
		headers:     headers,
	}
}

func (c *actionTestContext) Params(key string, defaultValue ...string) string {
	if value := c.params[key]; value != "" {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (c *actionTestContext) Header(key string) string {
	return c.headers[key]
}
