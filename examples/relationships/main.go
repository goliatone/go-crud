package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
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

// Author showcases belongs-to (PublishingHouse), has-one (Profile), and has-many (Books) relations.
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

type repositories struct {
	Publishers     repository.Repository[*PublishingHouse]
	Headquarters   repository.Repository[*Headquarters]
	Authors        repository.Repository[*Author]
	AuthorProfiles repository.Repository[*AuthorProfile]
	Books          repository.Repository[*Book]
	Chapters       repository.Repository[*Chapter]
	Tags           repository.Repository[*Tag]
}

func main() {
	db := setupDatabase()
	defer db.Close()

	repos := registerRepositories(db)
	if err := migrateSchema(db); err != nil {
		log.Fatalf("failed to migrate schema: %v", err)
	}
	if err := seedDatabase(context.Background(), db, repos); err != nil {
		log.Fatalf("failed to seed database: %v", err)
	}

	app := router.NewFiberAdapter(func(_ *fiber.App) *fiber.App {
		return fiber.New(fiber.Config{
			AppName:           "go-crud Relationship Demo",
			EnablePrintRoutes: true,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
		})
	})

	api := app.Router().Group("/api")
	apiAdapter := crud.NewGoRouterAdapter(api)

	crud.NewController(repos.Publishers).RegisterRoutes(apiAdapter)
	crud.NewController(repos.Headquarters).RegisterRoutes(apiAdapter)
	crud.NewController(repos.Authors).RegisterRoutes(apiAdapter)
	crud.NewController(repos.AuthorProfiles).RegisterRoutes(apiAdapter)
	crud.NewController(repos.Books).RegisterRoutes(apiAdapter)
	crud.NewController(repos.Chapters).RegisterRoutes(apiAdapter)
	crud.NewController(repos.Tags).RegisterRoutes(apiAdapter)

	router.ServeOpenAPI(app.Router(), &router.OpenAPIRenderer{
		Title:   "go-crud Relationship Demo",
		Version: "v0.1.0",
		Description: `## Relationship Showcase
This example focuses on eager-loading options for complex data models.
Use the query string parameter 'include' (e.g. ?include=authors.profile,books.tags)
to request nested relations over the REST endpoints.`,
		Contact: &router.OpenAPIFieldContact{
			Email: "oss@goliatone.com",
			Name:  "go-crud Examples",
			URL:   "https://github.com/goliatone/go-crud",
		},
	})

	app.Router().PrintRoutes()

	go func() {
		addr := ":9091"
		log.Printf("Starting relationship demo on %s", addr)
		log.Printf("OpenAPI UI available at http://localhost%s/meta/docs/", addr)
		if err := app.Serve(addr); err != nil {
			log.Panicf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down server...")
	if err := app.Shutdown(context.Background()); err != nil {
		log.Panicf("failed to shut down: %v", err)
	}
}

func setupDatabase() *bun.DB {
	sqldb, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		log.Fatalf("failed to open sqlite database: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	sqldb.SetMaxIdleConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	// Register pivot tables so bun can resolve many-to-many relations.
	db.RegisterModel((*BookTag)(nil))
	return db
}

func registerRepositories(db *bun.DB) repositories {
	return repositories{
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

func migrateSchema(db *bun.DB) error {
	ctx := context.Background()
	models := []any{
		(*PublishingHouse)(nil),
		(*Headquarters)(nil),
		(*Author)(nil),
		(*AuthorProfile)(nil),
		(*Book)(nil),
		(*Chapter)(nil),
		(*Tag)(nil),
		(*BookTag)(nil),
	}

	for _, model := range models {
		_, err := db.NewCreateTable().IfNotExists().Model(model).Exec(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func seedDatabase(ctx context.Context, db *bun.DB, repos repositories) error {
	now := time.Now().UTC()

	aurora, err := repos.Publishers.Create(ctx, &PublishingHouse{
		Name:          "Aurora Press",
		EstablishedAt: time.Date(1998, 3, 14, 0, 0, 0, 0, time.UTC),
		ImprintPrefix: "AUR",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return err
	}

	nimbus, err := repos.Publishers.Create(ctx, &PublishingHouse{
		Name:          "Nimbus Editorial",
		EstablishedAt: time.Date(2005, 7, 2, 0, 0, 0, 0, time.UTC),
		ImprintPrefix: "NIM",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return err
	}

	if _, err := repos.Headquarters.Create(ctx, &Headquarters{
		PublisherID:  aurora.ID,
		AddressLine1: "101 Skyline Ave",
		City:         "Seattle",
		Country:      "USA",
		OpenedAt:     now.AddDate(-10, 0, 0),
	}); err != nil {
		return err
	}

	if _, err := repos.Headquarters.Create(ctx, &Headquarters{
		PublisherID:  nimbus.ID,
		AddressLine1: "18 Harbour Road",
		City:         "Dublin",
		Country:      "Ireland",
		OpenedAt:     now.AddDate(-6, 0, 0),
	}); err != nil {
		return err
	}

	lina, err := repos.Authors.Create(ctx, &Author{
		PublisherID: aurora.ID,
		FullName:    "Lina Ortiz",
		PenName:     "L. Aurora",
		Email:       "lina.ortiz@example.com",
		Active:      true,
		HiredAt:     now.AddDate(-7, 0, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return err
	}

	miles, err := repos.Authors.Create(ctx, &Author{
		PublisherID: aurora.ID,
		FullName:    "Miles Dorsey",
		PenName:     "M. Gale",
		Email:       "miles.dorsey@example.com",
		Active:      true,
		HiredAt:     now.AddDate(-4, -3, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return err
	}

	if _, err := repos.Authors.Create(ctx, &Author{
		PublisherID: nimbus.ID,
		FullName:    "Esha Kapur",
		PenName:     "E. K. Shore",
		Email:       "esha.kapur@example.com",
		Active:      false,
		HiredAt:     now.AddDate(-3, 0, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return err
	}

	if _, err := repos.AuthorProfiles.Create(ctx, &AuthorProfile{
		AuthorID:      lina.ID,
		Biography:     "Award-winning sci-fi author exploring cosmic diplomacy and fractured futures.",
		WritingStyle:  "Cinematic prose with grounded scientific detail",
		FavoriteGenre: "Space Opera",
	}); err != nil {
		return err
	}

	if _, err := repos.AuthorProfiles.Create(ctx, &AuthorProfile{
		AuthorID:      miles.ID,
		Biography:     "Former weather scientist crafting thrillers about climate-engineered cities.",
		WritingStyle:  "Fast-paced investigative narratives",
		FavoriteGenre: "Speculative Thriller",
	}); err != nil {
		return err
	}

	contactShadows, err := repos.Books.Create(ctx, &Book{
		PublisherID: aurora.ID,
		AuthorID:    lina.ID,
		Title:       "Contact Shadows",
		ISBN:        "978-1-4028-9462-1",
		Status:      "in_print",
		ReleaseDate: time.Date(2021, 5, 4, 0, 0, 0, 0, time.UTC),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return err
	}

	exoArchive, err := repos.Books.Create(ctx, &Book{
		PublisherID: aurora.ID,
		AuthorID:    lina.ID,
		Title:       "The Exo-Archive Accord",
		ISBN:        "978-1-4028-9463-8",
		Status:      "editing",
		ReleaseDate: time.Date(2025, 2, 11, 0, 0, 0, 0, time.UTC),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return err
	}

	microburst, err := repos.Books.Create(ctx, &Book{
		PublisherID: aurora.ID,
		AuthorID:    miles.ID,
		Title:       "Microburst Protocol",
		ISBN:        "978-1-4028-9464-5",
		Status:      "in_print",
		ReleaseDate: time.Date(2022, 9, 22, 0, 0, 0, 0, time.UTC),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return err
	}

	chapters := []*Chapter{
		{
			BookID:       contactShadows.ID,
			Title:        "Signal Twelve",
			WordCount:    6400,
			ChapterIndex: 1,
		},
		{
			BookID:       contactShadows.ID,
			Title:        "Embassy in Orbit",
			WordCount:    7200,
			ChapterIndex: 2,
		},
		{
			BookID:       microburst.ID,
			Title:        "Doppler Alarm",
			WordCount:    5800,
			ChapterIndex: 1,
		},
	}
	if _, err := repos.Chapters.CreateMany(ctx, chapters); err != nil {
		return err
	}

	sciFi, err := repos.Tags.Create(ctx, &Tag{
		Name:        "Science Fiction",
		Category:    "genre",
		Description: "Stories grounded in speculative science and technology.",
		CreatedAt:   now,
	})
	if err != nil {
		return err
	}

	spaceOpera, err := repos.Tags.Create(ctx, &Tag{
		Name:        "Space Opera",
		Category:    "genre",
		Description: "Large-scale galactic adventures with political intrigue.",
		CreatedAt:   now,
	})
	if err != nil {
		return err
	}

	techThriller, err := repos.Tags.Create(ctx, &Tag{
		Name:        "Tech Thriller",
		Category:    "genre",
		Description: "High-stakes suspense fueled by emerging technology.",
		CreatedAt:   now,
	})
	if err != nil {
		return err
	}

	pivotRows := []BookTag{
		{BookID: contactShadows.ID, TagID: sciFi.ID, LinkStrength: 9, LinkedAt: now},
		{BookID: contactShadows.ID, TagID: spaceOpera.ID, LinkStrength: 8, LinkedAt: now},
		{BookID: exoArchive.ID, TagID: sciFi.ID, LinkStrength: 10, LinkedAt: now},
		{BookID: microburst.ID, TagID: techThriller.ID, LinkStrength: 8, LinkedAt: now},
	}
	if _, err := db.NewInsert().Model(&pivotRows).Exec(ctx); err != nil {
		return err
	}

	log.Println("Seeded relationship demo with sample data")
	return nil
}
