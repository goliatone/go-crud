//go:build gqlgen_snapshot
// +build gqlgen_snapshot

package resolvers

import (
	"context"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/gql/examples/cms_management/output/model"
	repository "github.com/goliatone/go-repository-bun"
)

// Custom resolver stubs. Safe to edit.
// Resolver satisfies gqlgen bindings and can hold your dependencies.
type Resolver struct {
	ScopeGuard     ScopeGuardFunc
	ContextFactory ContextFactory
	ContentSvc     crud.Service[model.Content]
	PageSvc        crud.Service[model.Page]
	ContentTypeSvc crud.Service[model.ContentType]
}

// Hook stubs (safe to edit); wire your auth/scope/preload/wrapping/error logic here.
func (r *Resolver) AuthGuard(ctx context.Context, entity, action string) error {
	return nil
}

func (r *Resolver) ScopeHook(ctx context.Context, entity, action string) error {
	return nil
}

func (r *Resolver) PreloadHook(ctx context.Context, entity, action string, criteria []repository.SelectCriteria) []repository.SelectCriteria {
	return criteria
}

func (r *Resolver) WrapService(ctx context.Context, entity, action string, svc crud.Service[any]) crud.Service[any] {
	return svc
}

func (r *Resolver) HandleError(ctx context.Context, entity, action string, err error) error {
	return err
}
