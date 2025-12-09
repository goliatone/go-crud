package loader

import (
	"net/http"

	"github.com/uptrace/bun"

	"github.com/goliatone/go-crud/examples/relationships-gql/graph/dataloader"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/resolvers"
)

// Middleware injects a per-request dataloader into the HTTP context for gqlgen.
func Middleware(resolver *resolvers.Resolver, db bun.IDB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ldr := New(resolver, db)
			if ldr == nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := dataloader.Inject(r.Context(), ldr)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// New builds a dataloader wiring the resolver services and optional DB handle.
func New(resolver *resolvers.Resolver, db bun.IDB) *dataloader.Loader {
	if resolver == nil {
		return nil
	}

	services := dataloader.Services{
		Author:          resolver.AuthorSvc,
		AuthorProfile:   resolver.AuthorProfileSvc,
		Book:            resolver.BookSvc,
		Chapter:         resolver.ChapterSvc,
		Headquarters:    resolver.HeadquartersSvc,
		PublishingHouse: resolver.PublishingHouseSvc,
		Tag:             resolver.TagSvc,
	}

	opts := []dataloader.Option{}
	if db != nil {
		opts = append(opts, dataloader.WithDB(db))
	}
	if resolver.ContextFactory != nil {
		opts = append(opts, dataloader.WithContextFactory(resolver.ContextFactory))
	}

	return dataloader.New(services, opts...)
}
