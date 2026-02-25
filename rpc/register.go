package rpc

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/goliatone/go-command"
	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
)

const defaultMethodPrefix = "crud"

// Registrar represents an RPC server compatible with go-command/rpc.Server.
type Registrar interface {
	Register(opts command.RPCConfig, handler any, meta command.CommandMeta) error
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

	register := func(method string, handler any) error {
		return server.Register(command.RPCConfig{Method: method}, handler, command.CommandMeta{})
	}

	if err := register(methodFor("create"), func(
		ctx context.Context,
		req RequestEnvelope[CreateData[T]],
	) (ResponseEnvelope[T], error) {
		rpcCtx := newRequestContext(ctx, req.Meta)
		record, err := controller.CreateRecord(rpcCtx, req.Data.Record)
		if err != nil {
			return ResponseEnvelope[T]{}, err
		}
		return ResponseEnvelope[T]{Data: record}, nil
	}); err != nil {
		return err
	}

	if err := register(methodFor("create_batch"), func(
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
	}); err != nil {
		return err
	}

	if err := register(methodFor("show"), func(
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
	}); err != nil {
		return err
	}

	if err := register(methodFor("index"), func(
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
	}); err != nil {
		return err
	}

	if err := register(methodFor("update"), func(
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
	}); err != nil {
		return err
	}

	if err := register(methodFor("update_batch"), func(
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
	}); err != nil {
		return err
	}

	if err := register(methodFor("delete"), func(
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
	}); err != nil {
		return err
	}

	if err := register(methodFor("delete_batch"), func(
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
	}); err != nil {
		return err
	}

	return nil
}

func resolveResourceName[T any](explicit string) string {
	if value := strings.TrimSpace(explicit); value != "" {
		return value
	}
	var zero T
	typ := reflect.TypeOf(zero)
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
