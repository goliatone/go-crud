package relationships

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Regenerate the GraphQL schema/config/resolvers for the relationships example
// using the live models/relations registered in registrar (no metadata.json).
//go:generate go run ../../gql/cmd/graphqlgen --schema-package ./registrar --out ./graph --config ./gqlgen.yml --emit-subscriptions --emit-dataloader

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
