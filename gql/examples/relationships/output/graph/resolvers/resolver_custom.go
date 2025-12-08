//go:build ignore
// +build ignore

package resolvers

import (
	"github.com/goliatone/go-crud"
	"graph/model"
)

// Custom resolver stubs. Safe to edit.
// Resolver satisfies gqlgen bindings and can hold your dependencies.
type Resolver struct {
	ScopeGuard             ScopeGuardFunc
	ContextFactory         ContextFactory
	AuthorService          crud.Service[model.Author]
	AuthorProfileService   crud.Service[model.AuthorProfile]
	BookService            crud.Service[model.Book]
	ChapterService         crud.Service[model.Chapter]
	HeadquartersService    crud.Service[model.Headquarters]
	PublishingHouseService crud.Service[model.PublishingHouse]
	TagService             crud.Service[model.Tag]
}
