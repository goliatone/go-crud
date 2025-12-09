package resolvers

import (
	"context"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
	repository "github.com/goliatone/go-repository-bun"
)

// Custom resolver stubs. Safe to edit.
// Resolver satisfies gqlgen bindings and can hold your dependencies.
type Resolver struct {
	ScopeGuard         ScopeGuardFunc
	ContextFactory     ContextFactory
	AuthorSvc          crud.Service[model.Author]
	AuthorProfileSvc   crud.Service[model.AuthorProfile]
	BookSvc            crud.Service[model.Book]
	ChapterSvc         crud.Service[model.Chapter]
	HeadquartersSvc    crud.Service[model.Headquarters]
	PublishingHouseSvc crud.Service[model.PublishingHouse]
	TagSvc             crud.Service[model.Tag]
}

// NewResolver wires CRUD services backed by the shared repositories.
func NewResolver(repos relationships.Repositories) *Resolver {
	svc := newServices(repos)
	return &Resolver{
		ContextFactory:     NewCRUDContext,
		AuthorSvc:          svc.author,
		AuthorProfileSvc:   svc.authorProfile,
		BookSvc:            svc.book,
		ChapterSvc:         svc.chapter,
		HeadquartersSvc:    svc.headquarters,
		PublishingHouseSvc: svc.publishingHouse,
		TagSvc:             svc.tag,
	}
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
