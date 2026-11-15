package crud

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"

	repository "github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type includePublisher struct {
	bun.BaseModel `bun:"table:include_publishers,alias:ip"`
	ID            uuid.UUID       `bun:"id,pk" json:"id"`
	Office        *includeOffice  `bun:"rel:has-one,join:id=publisher_id,join_on:city != 'Hidden'" json:"office"`
	Authors       []includeAuthor `bun:"rel:has-many,join:id=publisher_id" json:"authors"`
	Books         []includeBook   `bun:"rel:has-many,join:id=publisher_id" json:"books"`
}
type includeOffice struct {
	bun.BaseModel `bun:"table:include_offices,alias:io"`
	ID            uuid.UUID `bun:"id,pk" json:"id"`
	PublisherID   uuid.UUID `json:"publisher_id"`
	City          string    `json:"city"`
}
type includeAuthor struct {
	bun.BaseModel `bun:"table:include_authors,alias:ia"`
	ID            uuid.UUID `bun:"id,pk" json:"id"`
	PublisherID   uuid.UUID `json:"publisher_id"`
	// Both sibling branches reach includeBook. One must not hide the other.
	Publisher *includePublisher `bun:"rel:belongs-to,join:publisher_id=id" json:"publisher"`
	Books     []includeBook     `bun:"rel:has-many,join:id=author_id" json:"books"`
	Tags      []includeTag      `bun:"m2m:include_author_tags,join:Author=Tag" json:"labels"`
}
type includeBook struct {
	bun.BaseModel `bun:"table:include_books,alias:ib"`
	ID            uuid.UUID         `bun:"id,pk" json:"id"`
	AuthorID      uuid.UUID         `json:"author_id"`
	PublisherID   uuid.UUID         `json:"publisher_id"`
	Publisher     *includePublisher `bun:"rel:belongs-to,join:publisher_id=id" json:"publisher"`
	Author        *includeAuthor    `bun:"rel:belongs-to,join:author_id=id" json:"author"`
	Chapters      []includeChapter  `bun:"rel:has-many,join:id=book_id" json:"chapters"`
	Tags          []includeTag      `bun:"m2m:include_book_tags,join:Book=Tag" json:"tags"`
}
type includeChapter struct {
	bun.BaseModel `bun:"table:include_chapters,alias:ic"`
	ID            uuid.UUID `bun:"id,pk" json:"id"`
	BookID        uuid.UUID `json:"book_id"`
	Title         string    `json:"title"`
}
type includeTag struct {
	bun.BaseModel `bun:"table:include_tags,alias:it"`
	ID            uuid.UUID `bun:"id,pk" json:"id"`
	Name          string    `json:"name"`
}
type includeAuthorTag struct {
	bun.BaseModel `bun:"table:include_author_tags"`
	AuthorID      uuid.UUID      `bun:"author_id,pk"`
	TagID         uuid.UUID      `bun:"tag_id,pk"`
	Author        *includeAuthor `bun:"rel:belongs-to,join:author_id=id"`
	Tag           *includeTag    `bun:"rel:belongs-to,join:tag_id=id"`
}
type includeBookTag struct {
	bun.BaseModel `bun:"table:include_book_tags"`
	BookID        uuid.UUID    `bun:"book_id,pk"`
	TagID         uuid.UUID    `bun:"tag_id,pk"`
	Book          *includeBook `bun:"rel:belongs-to,join:book_id=id"`
	Tag           *includeTag  `bun:"rel:belongs-to,join:tag_id=id"`
}

func includeDatabase(t *testing.T) *bun.DB {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	db := bun.NewDB(raw, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.RegisterModel((*includeAuthorTag)(nil), (*includeBookTag)(nil))
	for _, model := range []any{(*includePublisher)(nil), (*includeOffice)(nil), (*includeAuthor)(nil), (*includeBook)(nil), (*includeChapter)(nil), (*includeTag)(nil), (*includeAuthorTag)(nil), (*includeBookTag)(nil)} {
		_, err := db.NewCreateTable().Model(model).Exec(t.Context())
		require.NoError(t, err)
	}
	publisher, author, book, tag, otherTag := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, record := range []any{
		&includePublisher{ID: publisher}, &includeOffice{ID: uuid.New(), PublisherID: publisher, City: "Seattle"},
		&includeAuthor{ID: author, PublisherID: publisher}, &includeBook{ID: book, AuthorID: author, PublisherID: publisher},
		&includeChapter{ID: uuid.New(), BookID: book, Title: "First"}, &includeChapter{ID: uuid.New(), BookID: book, Title: "Second"},
		&includeTag{ID: tag, Name: "science"}, &includeTag{ID: otherTag, Name: "history"},
		&includeAuthorTag{AuthorID: author, TagID: tag}, &includeAuthorTag{AuthorID: author, TagID: otherTag},
		&includeBookTag{BookID: book, TagID: tag}, &includeBookTag{BookID: book, TagID: otherTag},
	} {
		_, err := db.NewInsert().Model(record).Exec(t.Context())
		require.NoError(t, err)
	}
	return db
}

func TestRelationshipIncludesAdapterMatrix(t *testing.T) {
	for _, backend := range []string{"native-v2", "native-v3", "generic-v2", "generic-v3"} {
		t.Run(backend, func(t *testing.T) {
			db := includeDatabase(t)
			f := newAdapterFixture(backend)
			authors := NewController(repository.NewRepository(db, repository.ModelHandlers[*includeAuthor]{NewRecord: func() *includeAuthor { return &includeAuthor{} }}))
			books := NewController(repository.NewRepository(db, repository.ModelHandlers[*includeBook]{NewRecord: func() *includeBook { return &includeBook{} }}))
			f.routes.Get("/authors", authors.Index)
			f.routes.Get("/books", books.Index)
			f.init()
			request := func(t *testing.T, path string, target any) {
				t.Helper()
				resp, err := f.request(httptest.NewRequestWithContext(t.Context(), "GET", "/api"+path, nil))
				require.NoError(t, err)
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, 200, resp.StatusCode, string(body))
				require.NoError(t, json.Unmarshal(body, target))
			}
			t.Run("m2m-alias", func(t *testing.T) {
				var data APIListResponse[includeAuthor]
				request(t, "/authors?include=labels", &data)
				require.Len(t, data.Data, 1)
				require.Len(t, data.Data[0].Tags, 2)
			})
			t.Run("nested-m2m", func(t *testing.T) {
				var data APIListResponse[includeAuthor]
				request(t, "/authors?include=books.tags", &data)
				require.Len(t, data.Data, 1)
				require.Len(t, data.Data[0].Books, 1)
				require.Len(t, data.Data[0].Books[0].Tags, 2)
			})
			t.Run("sibling-paths", func(t *testing.T) {
				var data APIListResponse[includeAuthor]
				request(t, "/authors?include=publisher.books.chapters,books.chapters", &data)
				require.Len(t, data.Data, 1)
				a := data.Data[0]
				require.NotNil(t, a.Publisher)
				require.Len(t, a.Publisher.Books, 1)
				require.Len(t, a.Publisher.Books[0].Chapters, 2)
				require.Len(t, a.Books, 1)
				require.Len(t, a.Books[0].Chapters, 2)
			})
			t.Run("inline-child", func(t *testing.T) {
				var data APIListResponse[includeAuthor]
				request(t, "/authors?include=publisher.office", &data)
				require.Len(t, data.Data, 1)
				require.NotNil(t, data.Data[0].Publisher)
				require.NotNil(t, data.Data[0].Publisher.Office)
				require.Equal(t, "Seattle", data.Data[0].Publisher.Office.City)
			})
			t.Run("m2m-filter", func(t *testing.T) {
				var data APIListResponse[includeAuthor]
				request(t, "/authors?include=labels.name__eq=science,books.tags.name__eq=history", &data)
				require.Len(t, data.Data, 1)
				a := data.Data[0]
				require.Len(t, a.Tags, 1)
				require.Equal(t, "science", a.Tags[0].Name)
				require.Len(t, a.Books, 1)
				require.Len(t, a.Books[0].Tags, 1)
				require.Equal(t, "history", a.Books[0].Tags[0].Name)
			})
			t.Run("collection-inline-filter-scope", func(t *testing.T) {
				var matching APIListResponse[includeAuthor]
				request(t, "/authors?include=books.publisher.office.city__eq=Seattle", &matching)
				require.Len(t, matching.Data, 1)
				require.Len(t, matching.Data[0].Books, 1)
				require.NotNil(t, matching.Data[0].Books[0].Publisher.Office)
				var missing APIListResponse[includeAuthor]
				request(t, "/authors?include=books.publisher.office.city__eq=Nowhere", &missing)
				require.Len(t, missing.Data, 1)
				require.Empty(t, missing.Data[0].Books)
			})
			t.Run("nested-filter", func(t *testing.T) {
				var data APIListResponse[includeBook]
				request(t, "/books?include=chapters.title__eq=First,publisher.office.city__eq=Seattle", &data)
				require.Len(t, data.Data, 1)
				b := data.Data[0]
				require.Len(t, b.Chapters, 1)
				require.Equal(t, "First", b.Chapters[0].Title)
				require.NotNil(t, b.Publisher)
				require.NotNil(t, b.Publisher.Office)
			})
		})
	}
}

func TestRelationshipMetadataCyclesAndExclusions(t *testing.T) {
	meta := buildRelationMetadata(reflect.TypeFor[includeAuthor](), nil, make(map[reflect.Type]bool))
	_, err := buildIncludeTree("publisher.books.chapters,books.chapters,labels", meta)
	require.NoError(t, err)
	// A cycle may be selected as a terminal relation, but must not expand forever.
	_, err = buildIncludeTree("books.author.books.author", meta)
	require.Error(t, err)
	type hidden struct {
		Tags []includeTag `bun:"m2m:include_author_tags" crud:"-"`
	}
	hiddenMeta := buildRelationMetadata(reflect.TypeFor[hidden](), nil, make(map[reflect.Type]bool))
	_, err = buildIncludeTree("tags", hiddenMeta)
	require.Error(t, err)
}

func TestRelationshipIncludesPaginationAndTransaction(t *testing.T) {
	db := includeDatabase(t)
	repo := repository.NewRepository(db, repository.ModelHandlers[*includeAuthor]{NewRecord: func() *includeAuthor { return &includeAuthor{} }})
	var original includeAuthor
	require.NoError(t, db.NewSelect().Model(&original).Limit(1).Scan(t.Context()))
	second := includeAuthor{ID: uuid.New(), PublisherID: original.PublisherID}
	_, err := db.NewInsert().Model(&second).Exec(t.Context())
	require.NoError(t, err)
	criteria, _, _, err := buildIncludeCriteriaForType[*includeAuthor]("publisher.office", queryBuilderConfig{})
	require.NoError(t, err)
	criteria = append(criteria, paginationCriteria(1, 0))
	rows, total, err := repo.List(t.Context(), criteria...)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Publisher.Office)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.NewUpdate().Model((*includeOffice)(nil)).Set("city = ?", "Inside transaction").Where("publisher_id = ?", original.PublisherID).Exec(t.Context())
	require.NoError(t, err)
	rows, total, err = repo.ListTx(t.Context(), tx, criteria...)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, rows, 1)
	require.Equal(t, "Inside transaction", rows[0].Publisher.Office.City)
	require.NoError(t, tx.Rollback())
	rows, _, err = repo.List(t.Context(), criteria...)
	require.NoError(t, err)
	require.Equal(t, "Seattle", rows[0].Publisher.Office.City)
	// Declared join_on predicates retain their existing WHERE behavior.
	_, err = db.NewUpdate().Model((*includeOffice)(nil)).Set("city = ?", "Hidden").Where("publisher_id = ?", original.PublisherID).Exec(t.Context())
	require.NoError(t, err)
	rows, total, err = repo.List(t.Context(), criteria...)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Zero(t, total)
}

type includeRestrictedProvider struct{}

func (includeRestrictedProvider) BuildRelationDescriptor(typ reflect.Type) (*router.RelationDescriptor, error) {
	descriptor, err := newDefaultRelationProvider().BuildRelationDescriptor(typ)
	if err != nil {
		return nil, err
	}
	delete(descriptor.Tree.Children, "labels")
	delete(descriptor.Tree.Children, "tags")
	filtered := descriptor.Includes[:0]
	for _, path := range descriptor.Includes {
		if path != "labels" {
			filtered = append(filtered, path)
		}
	}
	descriptor.Includes = filtered
	return descriptor, nil
}

func TestRelationshipIncludeMetadataAuthority(t *testing.T) {
	descriptor, err := newDefaultRelationProvider().BuildRelationDescriptor(reflect.TypeFor[includeAuthor]())
	require.NoError(t, err)
	require.Contains(t, descriptor.Includes, "labels")
	require.Contains(t, descriptor.Includes, "books.tags")
	require.Same(t, descriptor.Tree.Children["labels"], descriptor.Tree.Children["tags"])
	registerRelationProvider(reflect.TypeFor[includeAuthor](), includeRestrictedProvider{})
	t.Cleanup(func() { registerRelationProvider(reflect.TypeFor[includeAuthor](), newDefaultRelationProvider()) })
	_, _, _, err = buildIncludeCriteriaForType[*includeAuthor]("labels", queryBuilderConfig{})
	require.Error(t, err, "custom metadata restrictions must remain authoritative")
	type filteredAuthor includeAuthor
	typ := reflect.TypeFor[filteredAuthor]()
	router.RegisterRelationFilter(func(resourceType reflect.Type, descriptor *router.RelationDescriptor) *router.RelationDescriptor {
		if resourceType == typ {
			delete(descriptor.Tree.Children, "labels")
			delete(descriptor.Tree.Children, "tags")
		}
		return descriptor
	})
	_, _, _, err = buildIncludeCriteriaForType[*filteredAuthor]("labels", queryBuilderConfig{})
	require.Error(t, err, "filters must run after default m2m completion")
}

func TestRelationshipIncludeSchemaMetadata(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		name := "default"
		if restricted {
			name = "custom"
		}
		t.Run(name, func(t *testing.T) {
			db := includeDatabase(t)
			repo := repository.NewRepository(db, repository.ModelHandlers[*includeAuthor]{NewRecord: func() *includeAuthor { return &includeAuthor{} }})
			var opts []Option[*includeAuthor]
			if restricted {
				opts = append(opts, WithRelationMetadataProvider[*includeAuthor](includeRestrictedProvider{}))
			}
			controller := NewController(repo, opts...)
			f := newAdapterFixture("native-v3")
			controller.RegisterRoutes(f.routes)
			meta, doc := controller.compileSchemaDocument()
			schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
			extension := schemas[meta.Name].(map[string]any)["x-formgen-relations"].(map[string]any)
			if restricted {
				require.NotContains(t, extension["includes"], "labels")
			} else {
				require.Contains(t, extension["includes"], "labels")
				require.Contains(t, extension["includes"], "books.tags")
			}
		})
	}
}
