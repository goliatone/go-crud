package main

import (
	"log"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Post demonstrates virtual fields backed by Metadata.
type Post struct {
	bun.BaseModel `bun:"table:posts"`

	ID       uuid.UUID      `bun:"id,pk" json:"id"`
	Title    string         `bun:"title" json:"title"`
	Metadata map[string]any `bun:"metadata,type:jsonb" json:"metadata,omitempty"`

	Author   *string   `bun:"-" json:"author" crud:"virtual:Metadata"`
	Category *string   `bun:"-" json:"category" crud:"virtual:Metadata"`
	Tags     *[]string `bun:"-" json:"tags" crud:"virtual:Metadata"`
}

func main() {
	log.Println("This example is illustrative; wire bun DB, router, and crud controller as needed.")

	var db *bun.DB // initialize your bun DB here

	repo := repository.NewRepository(db, repository.ModelHandlers[*Post]{
		NewRecord: func() *Post { return &Post{} },
		GetID: func(p *Post) uuid.UUID {
			return p.ID
		},
		SetID: func(p *Post, id uuid.UUID) {
			p.ID = id
		},
		GetIdentifier: func() string {
			return "ID"
		},
	})

	cfg := crud.VirtualFieldHandlerConfig{
		// Preserve virtual keys in Metadata; set to pointer to false to strip.
		PreserveVirtualKeys: crud.BoolPtr(true),
	}

	_ = crud.NewController(repo,
		crud.WithVirtualFields[*Post](cfg),
	)
}
