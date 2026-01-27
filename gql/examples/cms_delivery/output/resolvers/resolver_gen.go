//go:build gqlgen_snapshot
// +build gqlgen_snapshot

package resolvers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	crud "github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"

	"github.com/goliatone/go-crud/gql/examples/cms_delivery/output/model"
)

type ScopeGuardFunc func(ctx context.Context, entity, action string) error
type ContextFactory func(ctx context.Context) crud.Context

type queryResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }

func (r *Resolver) Query() *queryResolver {
	return &queryResolver{r}
}

func (r *Resolver) Mutation() *mutationResolver {
	return &mutationResolver{r}
}

func (r *Resolver) crudContext(ctx context.Context) crud.Context {
	if r.ContextFactory != nil {
		return r.ContextFactory(ctx)
	}
	return nil
}

func (r *Resolver) guard(ctx context.Context, entity, action string) error {
	if r.ScopeGuard != nil {
		return r.ScopeGuard(ctx, entity, action)
	}
	return nil
}

func (r *queryResolver) GetContent(ctx context.Context, id model.UUID) (*model.Content, error) {
	if err := r.guard(ctx, "Content", "read"); err != nil {
		return nil, err
	}
	if r.ContentSvc == nil {
		return nil, errors.New("content service not configured")
	}
	record, err := r.ContentSvc.Show(r.crudContext(ctx), string(id), nil)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *queryResolver) ListContent(ctx context.Context, _ *model.PaginationInput, _ []*model.OrderByInput, _ []*model.FilterInput) (*model.ContentConnection, error) {
	if err := r.guard(ctx, "Content", "list"); err != nil {
		return nil, err
	}
	if r.ContentSvc == nil {
		return nil, errors.New("content service not configured")
	}
	records, total, err := r.ContentSvc.Index(r.crudContext(ctx), []repository.SelectCriteria{})
	if err != nil {
		return nil, err
	}
	return buildContentConnection(records, total), nil
}

func (r *queryResolver) GetPage(ctx context.Context, id model.UUID) (*model.Page, error) {
	if err := r.guard(ctx, "Page", "read"); err != nil {
		return nil, err
	}
	if r.PageSvc == nil {
		return nil, errors.New("page service not configured")
	}
	record, err := r.PageSvc.Show(r.crudContext(ctx), string(id), nil)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *queryResolver) ListPage(ctx context.Context, _ *model.PaginationInput, _ []*model.OrderByInput, _ []*model.FilterInput) (*model.PageConnection, error) {
	if err := r.guard(ctx, "Page", "list"); err != nil {
		return nil, err
	}
	if r.PageSvc == nil {
		return nil, errors.New("page service not configured")
	}
	records, total, err := r.PageSvc.Index(r.crudContext(ctx), []repository.SelectCriteria{})
	if err != nil {
		return nil, err
	}
	return buildPageConnection(records, total), nil
}

func (r *queryResolver) GetMenu(ctx context.Context, id model.UUID) (*model.Menu, error) {
	if err := r.guard(ctx, "Menu", "read"); err != nil {
		return nil, err
	}
	if r.MenuSvc == nil {
		return nil, errors.New("menu service not configured")
	}
	record, err := r.MenuSvc.Show(r.crudContext(ctx), string(id), nil)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *queryResolver) ListMenu(ctx context.Context, _ *model.PaginationInput, _ []*model.OrderByInput, _ []*model.FilterInput) (*model.MenuConnection, error) {
	if err := r.guard(ctx, "Menu", "list"); err != nil {
		return nil, err
	}
	if r.MenuSvc == nil {
		return nil, errors.New("menu service not configured")
	}
	records, total, err := r.MenuSvc.Index(r.crudContext(ctx), []repository.SelectCriteria{})
	if err != nil {
		return nil, err
	}
	return buildMenuConnection(records, total), nil
}

func (r *mutationResolver) CreateContent(ctx context.Context, _ model.CreateContentInput) (*model.Content, error) {
	return nil, errors.New("delivery API is read-only")
}

func (r *mutationResolver) UpdateContent(ctx context.Context, _ model.UUID, _ model.UpdateContentInput) (*model.Content, error) {
	return nil, errors.New("delivery API is read-only")
}

func (r *mutationResolver) DeleteContent(ctx context.Context, _ model.UUID) (bool, error) {
	return false, errors.New("delivery API is read-only")
}

func (r *mutationResolver) CreateMenu(ctx context.Context, _ model.CreateMenuInput) (*model.Menu, error) {
	return nil, errors.New("delivery API is read-only")
}

func (r *mutationResolver) UpdateMenu(ctx context.Context, _ model.UUID, _ model.UpdateMenuInput) (*model.Menu, error) {
	return nil, errors.New("delivery API is read-only")
}

func (r *mutationResolver) DeleteMenu(ctx context.Context, _ model.UUID) (bool, error) {
	return false, errors.New("delivery API is read-only")
}

func (r *mutationResolver) CreatePage(ctx context.Context, _ model.CreatePageInput) (*model.Page, error) {
	return nil, errors.New("delivery API is read-only")
}

func (r *mutationResolver) UpdatePage(ctx context.Context, _ model.UUID, _ model.UpdatePageInput) (*model.Page, error) {
	return nil, errors.New("delivery API is read-only")
}

func (r *mutationResolver) DeletePage(ctx context.Context, _ model.UUID) (bool, error) {
	return false, errors.New("delivery API is read-only")
}

func buildContentConnection(records []model.Content, total int) *model.ContentConnection {
	edges := make([]*model.ContentEdge, 0, len(records))
	for i, record := range records {
		node := record
		edges = append(edges, &model.ContentEdge{
			Cursor: encodeCursor(i),
			Node:   &node,
		})
	}
	return &model.ContentConnection{
		Edges:    edges,
		PageInfo: buildPageInfo(total),
	}
}

func buildPageConnection(records []model.Page, total int) *model.PageConnection {
	edges := make([]*model.PageEdge, 0, len(records))
	for i, record := range records {
		node := record
		edges = append(edges, &model.PageEdge{
			Cursor: encodeCursor(i),
			Node:   &node,
		})
	}
	return &model.PageConnection{
		Edges:    edges,
		PageInfo: buildPageInfo(total),
	}
}

func buildMenuConnection(records []model.Menu, total int) *model.MenuConnection {
	edges := make([]*model.MenuEdge, 0, len(records))
	for i, record := range records {
		node := record
		edges = append(edges, &model.MenuEdge{
			Cursor: encodeCursor(i),
			Node:   &node,
		})
	}
	return &model.MenuConnection{
		Edges:    edges,
		PageInfo: buildPageInfo(total),
	}
}

func buildPageInfo(total int) *model.PageInfo {
	return &model.PageInfo{
		Total:           total,
		HasNextPage:     false,
		HasPreviousPage: false,
		StartCursor:     "",
		EndCursor:       "",
	}
}

func encodeCursor(index int) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("cursor:%d", index)))
}
