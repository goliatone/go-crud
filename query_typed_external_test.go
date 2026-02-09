package crud_test

import (
	"testing"

	crud "github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
)

type externalArticle struct {
	ID    string `json:"id" bun:"id,pk"`
	Title string `json:"title" bun:"title"`
}

type externalReadService struct{}

func (externalReadService) Index(_ crud.Context, _ []repository.SelectCriteria) ([]externalArticle, int, error) {
	return nil, 0, nil
}

func (externalReadService) Show(_ crud.Context, _ string, _ []repository.SelectCriteria) (externalArticle, error) {
	return externalArticle{}, nil
}

func TestBuildListCriteriaFromOptions_ExternalCompile(t *testing.T) {
	criteria, _, err := crud.BuildListCriteriaFromOptions[externalArticle](crud.ListQueryOptions{
		Limit:   20,
		Offset:  0,
		SortBy:  "title",
		Search:  "guide",
		Filters: map[string]any{"title__ilike": "%guide%"},
	}, crud.WithSearchColumns("title"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var svc crud.ReadService[externalArticle] = externalReadService{}
	if _, _, err := svc.Index(nil, criteria); err != nil {
		t.Fatalf("unexpected service error: %v", err)
	}
}
