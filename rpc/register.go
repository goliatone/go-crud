package rpc

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	commandrpc "github.com/goliatone/go-command/rpc"
	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
)

const defaultMethodPrefix = "crud"

// Registrar represents an RPC server compatible with go-command/rpc.Server.
type Registrar interface {
	RegisterEndpoint(def commandrpc.EndpointDefinition) error
	RegisterEndpoints(defs ...commandrpc.EndpointDefinition) error
}

// ResourceRegistrationOptions configures CRUD endpoint registration.
type ResourceRegistrationOptions struct {
	Resource       string
	MethodPrefix   string
	MethodResolver func(resource string, operation string) string
}

// RegisterResourceEndpoints registers CRUD endpoint handlers against an RPC registrar.
func RegisterResourceEndpoints[T any](
	server Registrar,
	controller *crud.Controller[T],
	opts ResourceRegistrationOptions,
) error {
	if server == nil {
		return fmt.Errorf("rpc registrar is required")
	}
	if controller == nil {
		return fmt.Errorf("controller is required")
	}

	resource := resolveResourceName[T](opts.Resource)
	if resource == "" {
		return fmt.Errorf("resource name is required")
	}

	methodFor := func(op string) string {
		if opts.MethodResolver != nil {
			if resolved := strings.TrimSpace(opts.MethodResolver(resource, op)); resolved != "" {
				return resolved
			}
		}
		prefix := strings.TrimSpace(opts.MethodPrefix)
		if prefix == "" {
			prefix = defaultMethodPrefix
		}
		return strings.Join([]string{prefix, resource, op}, ".")
	}

	defs := []commandrpc.EndpointDefinition{
		commandrpc.NewEndpoint[CreateData[T], T](commandrpc.EndpointSpec{
			Method: methodFor("create"),
			Kind:   commandrpc.MethodKindCommand,
		}, func(
			ctx context.Context,
			req RequestEnvelope[CreateData[T]],
		) (ResponseEnvelope[T], error) {
			rpcCtx := newRequestContext(ctx, req.Meta)
			record, err := controller.CreateRecord(rpcCtx, req.Data.Record)
			if err != nil {
				return ResponseEnvelope[T]{}, err
			}
			return ResponseEnvelope[T]{Data: record}, nil
		}),
		commandrpc.NewEndpoint[CreateBatchData[T], ListResult[T]](commandrpc.EndpointSpec{
			Method: methodFor("create_batch"),
			Kind:   commandrpc.MethodKindCommand,
		}, func(
			ctx context.Context,
			req RequestEnvelope[CreateBatchData[T]],
		) (ResponseEnvelope[ListResult[T]], error) {
			rpcCtx := newRequestContext(ctx, req.Meta)
			records, err := controller.CreateRecords(rpcCtx, req.Data.Records)
			if err != nil {
				return ResponseEnvelope[ListResult[T]]{}, err
			}
			return ResponseEnvelope[ListResult[T]]{
				Data: ListResult[T]{Items: records, Count: len(records)},
			}, nil
		}),
		commandrpc.NewEndpoint[ShowData, T](commandrpc.EndpointSpec{
			Method: methodFor("show"),
			Kind:   commandrpc.MethodKindQuery,
		}, func(
			ctx context.Context,
			req RequestEnvelope[ShowData],
		) (ResponseEnvelope[T], error) {
			rpcCtx := newRequestContext(ctx, req.Meta)
			id := strings.TrimSpace(req.Data.ID)
			if id == "" {
				id = strings.TrimSpace(req.Meta.Params["id"])
			}
			record, err := controller.ShowByID(rpcCtx, id, req.Data.Criteria)
			if err != nil {
				return ResponseEnvelope[T]{}, err
			}
			return ResponseEnvelope[T]{Data: record}, nil
		}),
		commandrpc.NewEndpoint[IndexData[crud.ListQueryOptions], ListResult[T]](commandrpc.EndpointSpec{
			Method: methodFor("index"),
			Kind:   commandrpc.MethodKindQuery,
		}, func(
			ctx context.Context,
			req RequestEnvelope[IndexData[crud.ListQueryOptions]],
		) (ResponseEnvelope[ListResult[T]], error) {
			rpcCtx := newRequestContext(ctx, req.Meta)
			criteria, err := buildIndexCriteria[T](req.Data.Options, req.Data.Criteria)
			if err != nil {
				return ResponseEnvelope[ListResult[T]]{}, err
			}
			records, count, err := controller.IndexWith(rpcCtx, criteria)
			if err != nil {
				return ResponseEnvelope[ListResult[T]]{}, err
			}
			return ResponseEnvelope[ListResult[T]]{
				Data: ListResult[T]{Items: records, Count: count},
			}, nil
		}),
		commandrpc.NewEndpoint[UpdateData[T], T](commandrpc.EndpointSpec{
			Method: methodFor("update"),
			Kind:   commandrpc.MethodKindCommand,
		}, func(
			ctx context.Context,
			req RequestEnvelope[UpdateData[T]],
		) (ResponseEnvelope[T], error) {
			rpcCtx := newRequestContext(ctx, req.Meta)
			id := strings.TrimSpace(req.Data.ID)
			if id == "" {
				id = strings.TrimSpace(req.Meta.Params["id"])
			}
			record, err := controller.UpdateRecord(rpcCtx, id, req.Data.Record)
			if err != nil {
				return ResponseEnvelope[T]{}, err
			}
			return ResponseEnvelope[T]{Data: record}, nil
		}),
		commandrpc.NewEndpoint[UpdateBatchData[T], ListResult[T]](commandrpc.EndpointSpec{
			Method: methodFor("update_batch"),
			Kind:   commandrpc.MethodKindCommand,
		}, func(
			ctx context.Context,
			req RequestEnvelope[UpdateBatchData[T]],
		) (ResponseEnvelope[ListResult[T]], error) {
			rpcCtx := newRequestContext(ctx, req.Meta)
			records, err := controller.UpdateRecords(rpcCtx, req.Data.Records)
			if err != nil {
				return ResponseEnvelope[ListResult[T]]{}, err
			}
			return ResponseEnvelope[ListResult[T]]{
				Data: ListResult[T]{Items: records, Count: len(records)},
			}, nil
		}),
		commandrpc.NewEndpoint[DeleteData, DeleteResult](commandrpc.EndpointSpec{
			Method: methodFor("delete"),
			Kind:   commandrpc.MethodKindCommand,
		}, func(
			ctx context.Context,
			req RequestEnvelope[DeleteData],
		) (ResponseEnvelope[DeleteResult], error) {
			rpcCtx := newRequestContext(ctx, req.Meta)
			id := strings.TrimSpace(req.Data.ID)
			if id == "" {
				id = strings.TrimSpace(req.Meta.Params["id"])
			}
			if err := controller.DeleteByID(rpcCtx, id); err != nil {
				return ResponseEnvelope[DeleteResult]{}, err
			}
			return ResponseEnvelope[DeleteResult]{Data: DeleteResult{Deleted: true}}, nil
		}),
		commandrpc.NewEndpoint[DeleteBatchData[T], DeleteBatchResult](commandrpc.EndpointSpec{
			Method: methodFor("delete_batch"),
			Kind:   commandrpc.MethodKindCommand,
		}, func(
			ctx context.Context,
			req RequestEnvelope[DeleteBatchData[T]],
		) (ResponseEnvelope[DeleteBatchResult], error) {
			rpcCtx := newRequestContext(ctx, req.Meta)
			records := req.Data.Records
			if len(records) == 0 && len(req.Data.IDs) > 0 {
				parsed, err := controller.RecordsFromIDs(req.Data.IDs)
				if err != nil {
					return ResponseEnvelope[DeleteBatchResult]{}, err
				}
				records = parsed
			}
			if err := controller.DeleteRecords(rpcCtx, records); err != nil {
				return ResponseEnvelope[DeleteBatchResult]{}, err
			}
			return ResponseEnvelope[DeleteBatchResult]{Data: DeleteBatchResult{Count: len(records)}}, nil
		}),
	}

	return server.RegisterEndpoints(defs...)
}

func resolveResourceName[T any](explicit string) string {
	if value := strings.TrimSpace(explicit); value != "" {
		return value
	}

	typ := reflect.TypeFor[T]()
	if typ == nil {
		return ""
	}
	if typ.Kind() == reflect.Ptr {
		if typ.Elem().Kind() != reflect.Struct {
			return ""
		}
	} else if typ.Kind() != reflect.Struct {
		return ""
	}

	resource, _ := crud.GetResourceName(typ)
	return strings.TrimSpace(resource)
}

func buildIndexCriteria[T any](opts crud.ListQueryOptions, criteria []repository.SelectCriteria) ([]repository.SelectCriteria, error) {
	out := append([]repository.SelectCriteria(nil), criteria...)
	if hasListQueryOptions(opts) {
		built, _, err := crud.BuildListCriteriaFromOptions[T](opts)
		if err != nil {
			return nil, err
		}
		out = append(out, built...)
		return out, nil
	}

	if len(out) == 0 {
		defaulted, _, err := crud.BuildListCriteriaFromOptions[T](crud.ListQueryOptions{})
		if err != nil {
			return nil, err
		}
		out = append(out, defaulted...)
	}

	return out, nil
}

func hasListQueryOptions(opts crud.ListQueryOptions) bool {
	return opts.Page != 0 ||
		opts.PerPage != 0 ||
		opts.Limit != 0 ||
		opts.Offset != 0 ||
		opts.SortBy != "" ||
		opts.SortDesc ||
		opts.Order != "" ||
		opts.Search != "" ||
		len(opts.Filters) > 0 ||
		len(opts.Predicates) > 0 ||
		len(opts.Select) > 0 ||
		len(opts.Include) > 0
}
