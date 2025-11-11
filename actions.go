package crud

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ettle/strcase"
	"github.com/goliatone/go-router"
)

// ActionTarget indicates whether the action targets the collection or a specific record.
type ActionTarget string

const (
	ActionTargetCollection ActionTarget = "collection"
	ActionTargetResource   ActionTarget = "resource"
)

// ActionHandler executes the custom action logic. Use the embedded Context to write responses.
type ActionHandler[T any] func(ActionContext[T]) error

// Action describes a custom endpoint registered alongside the CRUD routes.
type Action[T any] struct {
	Name        string
	Method      string
	Target      ActionTarget
	Path        string
	Summary     string
	Description string
	Tags        []string
	Parameters  []router.Parameter
	RequestBody *router.RequestBody
	Responses   []router.Response
	Security    []string
	Handler     ActionHandler[T]
}

// ActionDescriptor is a normalized view of the action exposed via metadata.
type ActionDescriptor struct {
	Name        string       `json:"name"`
	Slug        string       `json:"slug"`
	Method      string       `json:"method"`
	Target      ActionTarget `json:"target"`
	Path        string       `json:"path"`
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
}

// ActionContext extends the base Context with actor/scope metadata for convenience.
type ActionContext[T any] struct {
	Context
	Actor         ActorContext
	Scope         ScopeFilter
	RequestID     string
	CorrelationID string
	Action        ActionDescriptor
	Operation     CrudOperation
}

type resolvedAction[T any] struct {
	action     Action[T]
	slug       string
	method     string
	target     ActionTarget
	path       string
	routeName  string
	operation  CrudOperation
	descriptor ActionDescriptor
	routeDef   router.RouteDefinition
	handler    ActionHandler[T]
}

func resolveActions[T any](actions []Action[T], resource, resources string) []resolvedAction[T] {
	resolved := make([]resolvedAction[T], 0, len(actions))
	for _, action := range actions {
		if action.Handler == nil {
			continue
		}
		name := strings.TrimSpace(action.Name)
		if name == "" {
			continue
		}
		slug := strcase.ToKebab(name)
		method := strings.ToUpper(strings.TrimSpace(action.Method))
		if method == "" {
			method = http.MethodPost
		}
		target := action.Target
		if target != ActionTargetCollection && target != ActionTargetResource {
			target = ActionTargetResource
		}

		path := strings.TrimSpace(action.Path)
		if path == "" {
			if target == ActionTargetResource {
				path = fmt.Sprintf("/%s/:id/actions/%s", resource, slug)
			} else {
				path = fmt.Sprintf("/%s/actions/%s", resources, slug)
			}
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		routeName := fmt.Sprintf("%s:action:%s", resource, slug)
		op := CrudOperation(fmt.Sprintf("action:%s", slug))

		descriptor := ActionDescriptor{
			Name:        name,
			Slug:        slug,
			Method:      method,
			Target:      target,
			Path:        path,
			Summary:     action.Summary,
			Description: action.Description,
			Tags:        cloneStringSlice(action.Tags),
		}

		resolved = append(resolved, resolvedAction[T]{
			action:     action,
			slug:       slug,
			method:     method,
			target:     target,
			path:       path,
			routeName:  routeName,
			operation:  op,
			descriptor: descriptor,
			routeDef: router.RouteDefinition{
				Method:      router.HTTPMethod(method),
				Path:        path,
				Name:        routeName,
				Summary:     action.Summary,
				Description: action.Description,
				Tags:        cloneStringSlice(action.Tags),
				Parameters:  cloneParameters(action.Parameters),
				RequestBody: cloneRequestBody(action.RequestBody),
				Responses:   cloneResponses(action.Responses),
				Security:    cloneStringSlice(action.Security),
			},
			handler: action.Handler,
		})
	}
	return resolved
}

func cloneParameters(params []router.Parameter) []router.Parameter {
	if len(params) == 0 {
		return nil
	}
	cloned := make([]router.Parameter, len(params))
	copy(cloned, params)
	return cloned
}

func cloneResponses(responses []router.Response) []router.Response {
	if len(responses) == 0 {
		return nil
	}
	cloned := make([]router.Response, len(responses))
	copy(cloned, responses)
	return cloned
}

func cloneRequestBody(body *router.RequestBody) *router.RequestBody {
	if body == nil {
		return nil
	}
	cloned := *body
	if len(body.Content) > 0 {
		cloned.Content = make(map[string]any, len(body.Content))
		for k, v := range body.Content {
			cloned.Content[k] = v
		}
	}
	return &cloned
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
