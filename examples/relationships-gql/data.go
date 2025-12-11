package relationships

import (
	"context"
	"database/sql"
	"embed"
	"time"

	persistence "github.com/goliatone/go-persistence-bun"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type Repositories struct {
	Publishers     repository.Repository[*PublishingHouse]
	Headquarters   repository.Repository[*Headquarters]
	Authors        repository.Repository[*Author]
	AuthorProfiles repository.Repository[*AuthorProfile]
	Books          repository.Repository[*Book]
	Chapters       repository.Repository[*Chapter]
	Tags           repository.Repository[*Tag]
}

//go:embed internal/data/fixtures/*.yaml
var fixturesFS embed.FS

type persistenceConfig struct {
	debug bool
}

func (p persistenceConfig) GetDebug() bool                { return p.debug }
func (p persistenceConfig) GetDriver() string             { return "sqlite3" }
func (p persistenceConfig) GetServer() string             { return "file::memory:?cache=shared" }
func (p persistenceConfig) GetPingTimeout() time.Duration { return 5 * time.Second }
func (p persistenceConfig) GetOtelIdentifier() string     { return "relationships-gql" }

func registerModels() {
	persistence.RegisterModel(
		(*PublishingHouse)(nil),
		(*Headquarters)(nil),
		(*Author)(nil),
		(*AuthorProfile)(nil),
		(*Book)(nil),
		(*Chapter)(nil),
		(*Tag)(nil),
	)
	persistence.RegisterMany2ManyModel(
		(*BookTag)(nil),
		(*AuthorTag)(nil),
	)
}

// SetupDatabase boots an in-memory SQLite database using go-persistence-bun
// and returns the configured client.
func SetupDatabase(ctx context.Context) (*persistence.Client, error) {
	registerModels()

	sqlDB, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	client, err := persistence.New(persistenceConfig{}, sqlDB, sqlitedialect.New())
	if err != nil {
		return nil, err
	}

	if err := client.Migrate(ctx); err != nil {
		return nil, err
	}

	client.RegisterFixtures(fixturesFS).AddOptions(persistence.WithTrucateTables())

	return client, nil
}

func RegisterRepositories(db *bun.DB) Repositories {
	return Repositories{
		Publishers: repository.NewRepository(db, repository.ModelHandlers[*PublishingHouse]{
			NewRecord: func() *PublishingHouse { return &PublishingHouse{} },
			GetID: func(ph *PublishingHouse) uuid.UUID {
				if ph == nil {
					return uuid.Nil
				}
				return ph.ID
			},
			SetID: func(ph *PublishingHouse, id uuid.UUID) {
				if ph != nil {
					ph.ID = id
				}
			},
			GetIdentifier: func() string { return "name" },
			GetIdentifierValue: func(ph *PublishingHouse) string {
				if ph == nil {
					return ""
				}
				return ph.Name
			},
		}),
		Headquarters: repository.NewRepository(db, repository.ModelHandlers[*Headquarters]{
			NewRecord: func() *Headquarters { return &Headquarters{} },
			GetID: func(hq *Headquarters) uuid.UUID {
				if hq == nil {
					return uuid.Nil
				}
				return hq.ID
			},
			SetID: func(hq *Headquarters, id uuid.UUID) {
				if hq != nil {
					hq.ID = id
				}
			},
		}),
		Authors: repository.NewRepository(db, repository.ModelHandlers[*Author]{
			NewRecord: func() *Author { return &Author{} },
			GetID: func(a *Author) uuid.UUID {
				if a == nil {
					return uuid.Nil
				}
				return a.ID
			},
			SetID: func(a *Author, id uuid.UUID) {
				if a != nil {
					a.ID = id
				}
			},
			GetIdentifier: func() string { return "email" },
			GetIdentifierValue: func(a *Author) string {
				if a == nil {
					return ""
				}
				return a.Email
			},
		}),
		AuthorProfiles: repository.NewRepository(db, repository.ModelHandlers[*AuthorProfile]{
			NewRecord: func() *AuthorProfile { return &AuthorProfile{} },
			GetID: func(ap *AuthorProfile) uuid.UUID {
				if ap == nil {
					return uuid.Nil
				}
				return ap.ID
			},
			SetID: func(ap *AuthorProfile, id uuid.UUID) {
				if ap != nil {
					ap.ID = id
				}
			},
			GetIdentifier: func() string { return "author_id" },
			GetIdentifierValue: func(ap *AuthorProfile) string {
				if ap == nil {
					return ""
				}
				return ap.AuthorID.String()
			},
		}),
		Books: repository.NewRepository(db, repository.ModelHandlers[*Book]{
			NewRecord: func() *Book { return &Book{} },
			GetID: func(b *Book) uuid.UUID {
				if b == nil {
					return uuid.Nil
				}
				return b.ID
			},
			SetID: func(b *Book, id uuid.UUID) {
				if b != nil {
					b.ID = id
				}
			},
			GetIdentifier: func() string { return "isbn" },
			GetIdentifierValue: func(b *Book) string {
				if b == nil {
					return ""
				}
				return b.ISBN
			},
		}),
		Chapters: repository.NewRepository(db, repository.ModelHandlers[*Chapter]{
			NewRecord: func() *Chapter { return &Chapter{} },
			GetID: func(c *Chapter) uuid.UUID {
				if c == nil {
					return uuid.Nil
				}
				return c.ID
			},
			SetID: func(c *Chapter, id uuid.UUID) {
				if c != nil {
					c.ID = id
				}
			},
		}),
		Tags: repository.NewRepository(db, repository.ModelHandlers[*Tag]{
			NewRecord: func() *Tag { return &Tag{} },
			GetID: func(t *Tag) uuid.UUID {
				if t == nil {
					return uuid.Nil
				}
				return t.ID
			},
			SetID: func(t *Tag, id uuid.UUID) {
				if t != nil {
					t.ID = id
				}
			},
			GetIdentifier: func() string { return "name" },
			GetIdentifierValue: func(t *Tag) string {
				if t == nil {
					return ""
				}
				return t.Name
			},
		}),
	}
}

func MigrateSchema(ctx context.Context, db *bun.DB) error {
	models := []any{
		(*PublishingHouse)(nil),
		(*Headquarters)(nil),
		(*Author)(nil),
		(*AuthorProfile)(nil),
		(*Book)(nil),
		(*Chapter)(nil),
		(*Tag)(nil),
		(*BookTag)(nil),
		(*AuthorTag)(nil),
	}

	for _, model := range models {
		if _, err := db.NewCreateTable().IfNotExists().Model(model).Exec(ctx); err != nil {
			return err
		}
	}

	return nil
}

func SeedDatabase(ctx context.Context, client *persistence.Client) error {
	return client.Seed(ctx)
}
