package data

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

// PublishingHouse demonstrates has-one (Headquarters) and has-many (Authors, Books) relations.
type PublishingHouse struct {
	bun.BaseModel `bun:"table:publishing_houses,alias:ph"`
	ID            uuid.UUID     `bun:"id,pk,type:uuid" json:"id"`
	Name          string        `bun:"name,notnull" json:"name"`
	EstablishedAt time.Time     `bun:"established_at" json:"established_at"`
	ImprintPrefix string        `bun:"imprint_prefix" json:"imprint_prefix"`
	Headquarters  *Headquarters `bun:"rel:has-one,join:id=publisher_id" json:"headquarters,omitempty"`
	Authors       []Author      `bun:"rel:has-many,join:id=publisher_id" json:"authors,omitempty"`
	Books         []Book        `bun:"rel:has-many,join:id=publisher_id" json:"books,omitempty"`
	CreatedAt     time.Time     `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time     `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

// Headquarters belongs to a publishing house (belongs-to relation).
type Headquarters struct {
	bun.BaseModel `bun:"table:headquarters,alias:hq"`
	ID            uuid.UUID        `bun:"id,pk,type:uuid" json:"id"`
	PublisherID   uuid.UUID        `bun:"publisher_id,type:uuid,notnull" json:"publisher_id"`
	AddressLine1  string           `bun:"address_line1,notnull" json:"address_line1"`
	AddressLine2  string           `bun:"address_line2" json:"address_line2,omitempty"`
	City          string           `bun:"city,notnull" json:"city"`
	Country       string           `bun:"country,notnull" json:"country"`
	OpenedAt      time.Time        `bun:"opened_at" json:"opened_at"`
	Publisher     *PublishingHouse `bun:"rel:belongs-to,join:publisher_id=id" json:"publisher,omitempty"`
}

// Author showcases belongs-to (PublishingHouse), has-one (Profile), has-many (Books), and many-to-many (Tags) relations.
type Author struct {
	bun.BaseModel `bun:"table:authors,alias:a"`
	ID            uuid.UUID        `bun:"id,pk,type:uuid" json:"id"`
	PublisherID   uuid.UUID        `bun:"publisher_id,type:uuid,notnull" json:"publisher_id"`
	FullName      string           `bun:"full_name,notnull" json:"full_name"`
	PenName       string           `bun:"pen_name" json:"pen_name,omitempty"`
	Email         string           `bun:"email,notnull,unique" json:"email"`
	Active        bool             `bun:"active,notnull" json:"active"`
	HiredAt       time.Time        `bun:"hired_at" json:"hired_at"`
	Publisher     *PublishingHouse `bun:"rel:belongs-to,join:publisher_id=id" json:"publisher,omitempty"`
	Profile       *AuthorProfile   `bun:"rel:has-one,join:id=author_id" json:"profile,omitempty"`
	Books         []Book           `bun:"rel:has-many,join:id=author_id" json:"books,omitempty"`
	Tags          []Tag            `bun:"m2m:author_tags,join:Author=Tag" json:"tags,omitempty"`
	CreatedAt     time.Time        `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time        `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

// AuthorProfile is the has-one counterpart for Author.
type AuthorProfile struct {
	bun.BaseModel `bun:"table:author_profiles,alias:ap"`
	ID            uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	AuthorID      uuid.UUID `bun:"author_id,type:uuid,notnull,unique" json:"author_id"`
	Biography     string    `bun:"biography" json:"biography"`
	WritingStyle  string    `bun:"writing_style" json:"writing_style"`
	FavoriteGenre string    `bun:"favorite_genre" json:"favorite_genre"`
	Author        *Author   `bun:"rel:belongs-to,join:author_id=id" json:"author,omitempty"`
}

// Book demonstrates multiple belongs-to relations and a many-to-many relationship with Tag.
type Book struct {
	bun.BaseModel `bun:"table:books,alias:b"`
	ID            uuid.UUID        `bun:"id,pk,type:uuid" json:"id"`
	PublisherID   uuid.UUID        `bun:"publisher_id,type:uuid,notnull" json:"publisher_id"`
	AuthorID      uuid.UUID        `bun:"author_id,type:uuid,notnull" json:"author_id"`
	Title         string           `bun:"title,notnull" json:"title"`
	ISBN          string           `bun:"isbn,unique,notnull" json:"isbn"`
	Status        string           `bun:"status,notnull" json:"status"`
	ReleaseDate   time.Time        `bun:"release_date" json:"release_date"`
	Publisher     *PublishingHouse `bun:"rel:belongs-to,join:publisher_id=id" json:"publisher,omitempty"`
	Author        *Author          `bun:"rel:belongs-to,join:author_id=id" json:"author,omitempty"`
	Chapters      []Chapter        `bun:"rel:has-many,join:id=book_id" json:"chapters,omitempty"`
	Tags          []Tag            `bun:"m2m:book_tags,join:Book=Tag" json:"tags,omitempty"`
	LastReprintAt *time.Time       `bun:"last_reprint_at" json:"last_reprint_at,omitempty"`
	CreatedAt     time.Time        `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time        `bun:"updated_at,nullzero,default:current_timestamp" json:"updated_at"`
}

// Chapter belongs to a Book.
type Chapter struct {
	bun.BaseModel `bun:"table:chapters,alias:c"`
	ID            uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	BookID        uuid.UUID `bun:"book_id,type:uuid,notnull" json:"book_id"`
	Title         string    `bun:"title,notnull" json:"title"`
	WordCount     int       `bun:"word_count,notnull" json:"word_count"`
	ChapterIndex  int       `bun:"chapter_index,notnull" json:"chapter_index"`
	Book          *Book     `bun:"rel:belongs-to,join:book_id=id" json:"book,omitempty"`
}

// Tag participates in a many-to-many relationship with Book via book_tags.
type Tag struct {
	bun.BaseModel `bun:"table:tags,alias:t"`
	ID            uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	Name          string    `bun:"name,notnull" json:"name"`
	Category      string    `bun:"category,notnull" json:"category"`
	Description   string    `bun:"description" json:"description,omitempty"`
	Books         []Book    `bun:"m2m:book_tags,join:Tag=Book" json:"books,omitempty"`
	Authors       []Author  `bun:"m2m:author_tags,join:Tag=Author" json:"authors,omitempty"`
	CreatedAt     time.Time `bun:"created_at,nullzero,default:current_timestamp" json:"created_at"`
}

// BookTag is the join table for the many-to-many relationship.
type BookTag struct {
	bun.BaseModel `bun:"table:book_tags,alias:bt"`
	BookID        uuid.UUID `bun:"book_id,type:uuid,pk" json:"book_id"`
	TagID         uuid.UUID `bun:"tag_id,type:uuid,pk" json:"tag_id"`
	LinkStrength  int       `bun:"link_strength,notnull" json:"link_strength"`
	LinkedAt      time.Time `bun:"linked_at,nullzero,default:current_timestamp" json:"linked_at"`
	Book          *Book     `bun:"rel:belongs-to,join:book_id=id" json:"book,omitempty"`
	Tag           *Tag      `bun:"rel:belongs-to,join:tag_id=id" json:"tag,omitempty"`
}

// AuthorTag is the join table for Author to Tag many-to-many relationship.
type AuthorTag struct {
	bun.BaseModel `bun:"table:author_tags,alias:at"`
	AuthorID      uuid.UUID `bun:"author_id,type:uuid,pk" json:"author_id"`
	TagID         uuid.UUID `bun:"tag_id,type:uuid,pk" json:"tag_id"`
	AssociatedAt  time.Time `bun:"associated_at,nullzero,default:current_timestamp" json:"associated_at"`
	Author        *Author   `bun:"rel:belongs-to,join:author_id=id" json:"author,omitempty"`
	Tag           *Tag      `bun:"rel:belongs-to,join:tag_id=id" json:"tag,omitempty"`
}

type Repositories struct {
	Publishers     repository.Repository[*PublishingHouse]
	Headquarters   repository.Repository[*Headquarters]
	Authors        repository.Repository[*Author]
	AuthorProfiles repository.Repository[*AuthorProfile]
	Books          repository.Repository[*Book]
	Chapters       repository.Repository[*Chapter]
	Tags           repository.Repository[*Tag]
}

//go:embed fixtures/*.yaml
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

	// Run any registered migrations (none for this example, but keeps parity with other apps).
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
