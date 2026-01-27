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

	"github.com/goliatone/go-crud/gql/examples/cms_management/output/model"
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
	if err := r.AuthGuard(ctx, "Content", "show"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "Content", "show"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "Content", "show"); err != nil {
		return nil, err
	}
	if r.ContentSvc == nil {
		return nil, errors.New("content service not configured")
	}
	record, err := r.ContentSvc.Show(r.crudContext(ctx), string(id), nil)
	if err != nil {
		if herr := r.HandleError(ctx, "Content", "show", err); herr != nil {
			return nil, herr
		}
	}
	return &record, nil
}

func (r *queryResolver) ListContent(ctx context.Context, pagination *model.PaginationInput, orderBy []*model.OrderByInput, filter []*model.FilterInput) (*model.ContentConnection, error) {
	if err := r.AuthGuard(ctx, "Content", "index"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "Content", "index"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "Content", "index"); err != nil {
		return nil, err
	}
	if r.ContentSvc == nil {
		return nil, errors.New("content service not configured")
	}
	criteria := r.PreloadHook(ctx, "Content", "index", []repository.SelectCriteria{})
	_ = pagination
	_ = orderBy
	_ = filter
	records, total, err := r.ContentSvc.Index(r.crudContext(ctx), criteria)
	if err != nil {
		if herr := r.HandleError(ctx, "Content", "index", err); herr != nil {
			return nil, herr
		}
	}
	return buildContentConnection(records, total), nil
}

func (r *queryResolver) GetPage(ctx context.Context, id model.UUID) (*model.Page, error) {
	if err := r.AuthGuard(ctx, "Page", "show"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "Page", "show"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "Page", "show"); err != nil {
		return nil, err
	}
	if r.PageSvc == nil {
		return nil, errors.New("page service not configured")
	}
	record, err := r.PageSvc.Show(r.crudContext(ctx), string(id), nil)
	if err != nil {
		if herr := r.HandleError(ctx, "Page", "show", err); herr != nil {
			return nil, herr
		}
	}
	return &record, nil
}

func (r *queryResolver) ListPage(ctx context.Context, pagination *model.PaginationInput, orderBy []*model.OrderByInput, filter []*model.FilterInput) (*model.PageConnection, error) {
	if err := r.AuthGuard(ctx, "Page", "index"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "Page", "index"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "Page", "index"); err != nil {
		return nil, err
	}
	if r.PageSvc == nil {
		return nil, errors.New("page service not configured")
	}
	criteria := r.PreloadHook(ctx, "Page", "index", []repository.SelectCriteria{})
	_ = pagination
	_ = orderBy
	_ = filter
	records, total, err := r.PageSvc.Index(r.crudContext(ctx), criteria)
	if err != nil {
		if herr := r.HandleError(ctx, "Page", "index", err); herr != nil {
			return nil, herr
		}
	}
	return buildPageConnection(records, total), nil
}

func (r *queryResolver) GetContentType(ctx context.Context, id model.UUID) (*model.ContentType, error) {
	if err := r.AuthGuard(ctx, "ContentType", "show"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "ContentType", "show"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "ContentType", "show"); err != nil {
		return nil, err
	}
	if r.ContentTypeSvc == nil {
		return nil, errors.New("content type service not configured")
	}
	record, err := r.ContentTypeSvc.Show(r.crudContext(ctx), string(id), nil)
	if err != nil {
		if herr := r.HandleError(ctx, "ContentType", "show", err); herr != nil {
			return nil, herr
		}
	}
	return &record, nil
}

func (r *queryResolver) ListContentType(ctx context.Context, pagination *model.PaginationInput, orderBy []*model.OrderByInput, filter []*model.FilterInput) (*model.ContentTypeConnection, error) {
	if err := r.AuthGuard(ctx, "ContentType", "index"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "ContentType", "index"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "ContentType", "index"); err != nil {
		return nil, err
	}
	if r.ContentTypeSvc == nil {
		return nil, errors.New("content type service not configured")
	}
	criteria := r.PreloadHook(ctx, "ContentType", "index", []repository.SelectCriteria{})
	_ = pagination
	_ = orderBy
	_ = filter
	records, total, err := r.ContentTypeSvc.Index(r.crudContext(ctx), criteria)
	if err != nil {
		if herr := r.HandleError(ctx, "ContentType", "index", err); herr != nil {
			return nil, herr
		}
	}
	return buildContentTypeConnection(records, total), nil
}

func (r *mutationResolver) CreateContent(ctx context.Context, input model.CreateContentInput) (*model.Content, error) {
	if err := r.AuthGuard(ctx, "Content", "create"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "Content", "create"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "Content", "create"); err != nil {
		return nil, err
	}
	if r.ContentSvc == nil {
		return nil, errors.New("content service not configured")
	}
	record, err := r.ContentSvc.Create(r.crudContext(ctx), model.Content{
		Type:   input.Type,
		Slug:   input.Slug,
		Locale: input.Locale,
		Status: input.Status,
		Data:   input.Data,
	})
	if err != nil {
		if herr := r.HandleError(ctx, "Content", "create", err); herr != nil {
			return nil, herr
		}
	}
	return &record, nil
}

func (r *mutationResolver) UpdateContent(ctx context.Context, id model.UUID, input model.UpdateContentInput) (*model.Content, error) {
	if err := r.AuthGuard(ctx, "Content", "update"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "Content", "update"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "Content", "update"); err != nil {
		return nil, err
	}
	if r.ContentSvc == nil {
		return nil, errors.New("content service not configured")
	}
	record := model.Content{Id: string(id)}
	if input.Type != nil {
		record.Type = *input.Type
	}
	if input.Slug != nil {
		record.Slug = *input.Slug
	}
	if input.Locale != nil {
		record.Locale = *input.Locale
	}
	if input.Status != nil {
		record.Status = *input.Status
	}
	if input.Data != nil {
		record.Data = input.Data
	}
	updated, err := r.ContentSvc.Update(r.crudContext(ctx), record)
	if err != nil {
		if herr := r.HandleError(ctx, "Content", "update", err); herr != nil {
			return nil, herr
		}
	}
	return &updated, nil
}

func (r *mutationResolver) DeleteContent(ctx context.Context, id model.UUID) (bool, error) {
	if err := r.AuthGuard(ctx, "Content", "delete"); err != nil {
		return false, err
	}
	if err := r.ScopeHook(ctx, "Content", "delete"); err != nil {
		return false, err
	}
	if err := r.guard(ctx, "Content", "delete"); err != nil {
		return false, err
	}
	if r.ContentSvc == nil {
		return false, errors.New("content service not configured")
	}
	if err := r.ContentSvc.Delete(r.crudContext(ctx), model.Content{Id: string(id)}); err != nil {
		if herr := r.HandleError(ctx, "Content", "delete", err); herr != nil {
			return false, herr
		}
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreatePage(ctx context.Context, input model.CreatePageInput) (*model.Page, error) {
	if err := r.AuthGuard(ctx, "Page", "create"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "Page", "create"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "Page", "create"); err != nil {
		return nil, err
	}
	if r.PageSvc == nil {
		return nil, errors.New("page service not configured")
	}
	record, err := r.PageSvc.Create(r.crudContext(ctx), model.Page{
		Title:  input.Title,
		Slug:   input.Slug,
		Locale: input.Locale,
		Status: input.Status,
		Data:   input.Data,
	})
	if err != nil {
		if herr := r.HandleError(ctx, "Page", "create", err); herr != nil {
			return nil, herr
		}
	}
	return &record, nil
}

func (r *mutationResolver) UpdatePage(ctx context.Context, id model.UUID, input model.UpdatePageInput) (*model.Page, error) {
	if err := r.AuthGuard(ctx, "Page", "update"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "Page", "update"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "Page", "update"); err != nil {
		return nil, err
	}
	if r.PageSvc == nil {
		return nil, errors.New("page service not configured")
	}
	record := model.Page{Id: string(id)}
	if input.Title != nil {
		record.Title = input.Title
	}
	if input.Slug != nil {
		record.Slug = *input.Slug
	}
	if input.Locale != nil {
		record.Locale = *input.Locale
	}
	if input.Status != nil {
		record.Status = *input.Status
	}
	if input.Data != nil {
		record.Data = input.Data
	}
	updated, err := r.PageSvc.Update(r.crudContext(ctx), record)
	if err != nil {
		if herr := r.HandleError(ctx, "Page", "update", err); herr != nil {
			return nil, herr
		}
	}
	return &updated, nil
}

func (r *mutationResolver) DeletePage(ctx context.Context, id model.UUID) (bool, error) {
	if err := r.AuthGuard(ctx, "Page", "delete"); err != nil {
		return false, err
	}
	if err := r.ScopeHook(ctx, "Page", "delete"); err != nil {
		return false, err
	}
	if err := r.guard(ctx, "Page", "delete"); err != nil {
		return false, err
	}
	if r.PageSvc == nil {
		return false, errors.New("page service not configured")
	}
	if err := r.PageSvc.Delete(r.crudContext(ctx), model.Page{Id: string(id)}); err != nil {
		if herr := r.HandleError(ctx, "Page", "delete", err); herr != nil {
			return false, herr
		}
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateContentType(ctx context.Context, input model.CreateContentTypeInput) (*model.ContentType, error) {
	if err := r.AuthGuard(ctx, "ContentType", "create"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "ContentType", "create"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "ContentType", "create"); err != nil {
		return nil, err
	}
	if r.ContentTypeSvc == nil {
		return nil, errors.New("content type service not configured")
	}
	record, err := r.ContentTypeSvc.Create(r.crudContext(ctx), model.ContentType{
		Name:         input.Name,
		Slug:         input.Slug,
		Description:  input.Description,
		Schema:       input.Schema,
		Capabilities: input.Capabilities,
		Icon:         input.Icon,
	})
	if err != nil {
		if herr := r.HandleError(ctx, "ContentType", "create", err); herr != nil {
			return nil, herr
		}
	}
	return &record, nil
}

func (r *mutationResolver) UpdateContentType(ctx context.Context, id model.UUID, input model.UpdateContentTypeInput) (*model.ContentType, error) {
	if err := r.AuthGuard(ctx, "ContentType", "update"); err != nil {
		return nil, err
	}
	if err := r.ScopeHook(ctx, "ContentType", "update"); err != nil {
		return nil, err
	}
	if err := r.guard(ctx, "ContentType", "update"); err != nil {
		return nil, err
	}
	if r.ContentTypeSvc == nil {
		return nil, errors.New("content type service not configured")
	}
	record := model.ContentType{Id: string(id)}
	if input.Name != nil {
		record.Name = *input.Name
	}
	if input.Slug != nil {
		record.Slug = *input.Slug
	}
	if input.Description != nil {
		record.Description = input.Description
	}
	if input.Schema != nil {
		record.Schema = input.Schema
	}
	if input.Capabilities != nil {
		record.Capabilities = input.Capabilities
	}
	if input.Icon != nil {
		record.Icon = input.Icon
	}
	updated, err := r.ContentTypeSvc.Update(r.crudContext(ctx), record)
	if err != nil {
		if herr := r.HandleError(ctx, "ContentType", "update", err); herr != nil {
			return nil, herr
		}
	}
	return &updated, nil
}

func (r *mutationResolver) DeleteContentType(ctx context.Context, id model.UUID) (bool, error) {
	if err := r.AuthGuard(ctx, "ContentType", "delete"); err != nil {
		return false, err
	}
	if err := r.ScopeHook(ctx, "ContentType", "delete"); err != nil {
		return false, err
	}
	if err := r.guard(ctx, "ContentType", "delete"); err != nil {
		return false, err
	}
	if r.ContentTypeSvc == nil {
		return false, errors.New("content type service not configured")
	}
	if err := r.ContentTypeSvc.Delete(r.crudContext(ctx), model.ContentType{Id: string(id)}); err != nil {
		if herr := r.HandleError(ctx, "ContentType", "delete", err); herr != nil {
			return false, herr
		}
		return false, err
	}
	return true, nil
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

func buildContentTypeConnection(records []model.ContentType, total int) *model.ContentTypeConnection {
	edges := make([]*model.ContentTypeEdge, 0, len(records))
	for i, record := range records {
		node := record
		edges = append(edges, &model.ContentTypeEdge{
			Cursor: encodeCursor(i),
			Node:   &node,
		})
	}
	return &model.ContentTypeConnection{
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
