package registrar

import (
	"context"
	"log"
	"reflect"
	"strings"

	"github.com/ettle/strcase"
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
		crud.NewController(repos.Publishers, crud.WithFieldMapProvider[*relationships.PublishingHouse](fieldMap[*relationships.PublishingHouse]())),
		crud.NewController(repos.Headquarters, crud.WithFieldMapProvider[*relationships.Headquarters](fieldMap[*relationships.Headquarters]())),
		crud.NewController(repos.Authors, crud.WithFieldMapProvider[*relationships.Author](fieldMap[*relationships.Author]())),
		crud.NewController(repos.AuthorProfiles, crud.WithFieldMapProvider[*relationships.AuthorProfile](fieldMap[*relationships.AuthorProfile]())),
		crud.NewController(repos.Books, crud.WithFieldMapProvider[*relationships.Book](fieldMap[*relationships.Book]())),
		crud.NewController(repos.Chapters, crud.WithFieldMapProvider[*relationships.Chapter](fieldMap[*relationships.Chapter]())),
		crud.NewController(repos.Tags, crud.WithFieldMapProvider[*relationships.Tag](fieldMap[*relationships.Tag]())),
	)
}

// fieldMap builds a crud.FieldMapProvider for the given model type using bun/json tags.
func fieldMap[T any]() crud.FieldMapProvider {
	var zero T
	base := indirect(reflect.TypeOf(zero))

	return func(t reflect.Type) map[string]string {
		if indirect(t) != base {
			return nil
		}
		fields := make(map[string]string)
		for i := 0; i < base.NumField(); i++ {
			field := base.Field(i)
			if !field.IsExported() || field.Tag.Get("crud") == "-" {
				continue
			}

			column := strings.Split(field.Tag.Get("bun"), ",")[0]
			if column == "" {
				column = strcase.ToSnake(field.Name)
			}

			jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
			if jsonName == "" {
				jsonName = strcase.ToSnake(field.Name)
			}
			if jsonName == "-" || column == "" {
				continue
			}
			fields[strings.ToLower(jsonName)] = column
		}
		return fields
	}
}

func indirect(t reflect.Type) reflect.Type {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
