package resolvers

import (
	"context"

	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
	"{{ ModelPackage }}"
{% if EmitDataloader %}
	"{{ DataloaderPackage }}"
{% endif %}
)

// Custom resolver stubs. Safe to edit.
// Resolver satisfies gqlgen bindings and can hold your dependencies.
type Resolver struct {
	ScopeGuard     ScopeGuardFunc
	ContextFactory ContextFactory
{% if EmitDataloader %}	Loaders *dataloader.Loader
{% endif %}{% for entity in ResolverEntities %}	{{ entity.Name }}Svc crud.Service[model.{{ entity.Name }}]
{% endfor %}
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
