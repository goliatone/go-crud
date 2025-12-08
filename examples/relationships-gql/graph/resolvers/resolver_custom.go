package resolvers

import (
	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
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
