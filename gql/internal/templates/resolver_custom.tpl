package resolvers

import (
	"github.com/goliatone/go-crud"
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
