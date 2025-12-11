//go:build gqlgen_snapshot
// +build gqlgen_snapshot

package resolvers

import (
	"github.com/goliatone/go-crud"
	"graph/model"
)

// Custom resolver stubs. Safe to edit.
// Resolver satisfies gqlgen bindings and can hold your dependencies.
type Resolver struct {
	ScopeGuard     ScopeGuardFunc
	ContextFactory ContextFactory
	PostService    crud.Service[model.Post]
	UserService    crud.Service[model.User]
}
