package registrar

import (
	"context"
	"log"

	"github.com/goliatone/go-crud"
	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	gqlregistrar "github.com/goliatone/go-crud/gql/registrar"
)

// init registers controllers for all models so graphqlgen can pull metadata
// via --schema-package without relying on a checked-in metadata.json.
func init() {
	ctx := context.Background()

	client, err := relationships.SetupDatabase(ctx)
	if err != nil {
		log.Fatalf("registrar: setup database: %v", err)
	}
	db := client.DB()
	if db == nil {
		log.Fatal("registrar: database is nil")
	}

	repos := relationships.RegisterRepositories(db)
	gqlregistrar.RegisterControllers(
		crud.NewController(repos.Publishers),
		crud.NewController(repos.Headquarters),
		crud.NewController(repos.Authors),
		crud.NewController(repos.AuthorProfiles),
		crud.NewController(repos.Books),
		crud.NewController(repos.Chapters),
		crud.NewController(repos.Tags),
	)
}
