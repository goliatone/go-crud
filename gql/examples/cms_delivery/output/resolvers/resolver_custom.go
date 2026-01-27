//go:build gqlgen_snapshot
// +build gqlgen_snapshot

package resolvers

import (
	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/gql/examples/cms_delivery/output/model"
)

// Custom resolver stubs. Safe to edit.
// Resolver satisfies gqlgen bindings and can hold your dependencies.
type Resolver struct {
	ScopeGuard     ScopeGuardFunc
	ContextFactory ContextFactory
	ContentSvc     crud.Service[model.Content]
	PageSvc        crud.Service[model.Page]
	MenuSvc        crud.Service[model.Menu]
}
