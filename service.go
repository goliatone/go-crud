package crud

import (
	"github.com/goliatone/go-repository-bun"
)

// Service defines pluggable CRUD behaviours that the controller can delegate to.
type Service[T any] interface {
	Create(ctx Context, record T) (T, error)
	CreateBatch(ctx Context, records []T) ([]T, error)

	Update(ctx Context, record T) (T, error)
	UpdateBatch(ctx Context, records []T) ([]T, error)

	Delete(ctx Context, record T) error
	DeleteBatch(ctx Context, records []T) error

	Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error)
	Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error)
}

// ServiceFuncs allows callers to override specific service operations.
type ServiceFuncs[T any] struct {
	Create      func(ctx Context, record T) (T, error)
	CreateBatch func(ctx Context, records []T) ([]T, error)

	Update      func(ctx Context, record T) (T, error)
	UpdateBatch func(ctx Context, records []T) ([]T, error)

	Delete      func(ctx Context, record T) error
	DeleteBatch func(ctx Context, records []T) error

	Index func(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error)
	Show  func(ctx Context, id string, criteria []repository.SelectCriteria) (T, error)
}

// ComposeService returns a Service implementation that uses the given defaults
// and overrides any operation provided in funcs.
func ComposeService[T any](defaults Service[T], funcs ServiceFuncs[T]) Service[T] {
	return &serviceFuncAdapter[T]{
		defaults: defaults,
		funcs:    funcs,
	}
}

// CommandServiceFactory builds a Service implementation that can wrap the
// controller's default service (repository-backed) with command adapters.
type CommandServiceFactory[T any] func(defaults Service[T]) Service[T]

// CommandServiceFromFuncs returns a CommandServiceFactory that applies the given
// overrides on top of the provided defaults. It is useful when command adapters
// only need to intercept a subset of operations.
func CommandServiceFromFuncs[T any](overrides ServiceFuncs[T]) CommandServiceFactory[T] {
	return func(defaults Service[T]) Service[T] {
		return ComposeService(defaults, overrides)
	}
}

// NewRepositoryService returns a Service[T] that delegates to repository.Repository[T].
func NewRepositoryService[T any](repo repository.Repository[T]) Service[T] {
	return NewRepositoryServiceWithOptions(repo, RepositoryServiceOptions{})
}

// RepositoryServiceOptions configures repository-backed service behavior.
type RepositoryServiceOptions struct {
	BatchInsertCriteria []repository.InsertCriteria
	BatchUpdateCriteria []repository.UpdateCriteria
}

// NewRepositoryServiceWithOptions returns a Service[T] that delegates to repository.Repository[T].
func NewRepositoryServiceWithOptions[T any](repo repository.Repository[T], opts RepositoryServiceOptions) Service[T] {
	return &repositoryService[T]{
		repo:           repo,
		insertCriteria: opts.BatchInsertCriteria,
		updateCriteria: opts.BatchUpdateCriteria,
	}
}

// NewServiceFromFuncs builds a service backed by the repository and applying overrides.
func NewServiceFromFuncs[T any](repo repository.Repository[T], funcs ServiceFuncs[T]) Service[T] {
	return ComposeService(NewRepositoryService(repo), funcs)
}

type repositoryService[T any] struct {
	repo           repository.Repository[T]
	insertCriteria []repository.InsertCriteria
	updateCriteria []repository.UpdateCriteria
}

func (s *repositoryService[T]) Create(ctx Context, record T) (T, error) {
	return s.repo.Create(ctx.UserContext(), record)
}

func (s *repositoryService[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	if len(s.insertCriteria) == 0 {
		return s.repo.CreateMany(ctx.UserContext(), records)
	}
	return s.repo.CreateMany(ctx.UserContext(), records, s.insertCriteria...)
}

func (s *repositoryService[T]) Update(ctx Context, record T) (T, error) {
	return s.repo.Update(ctx.UserContext(), record)
}

func (s *repositoryService[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	if len(s.updateCriteria) == 0 {
		return s.repo.UpdateMany(ctx.UserContext(), records)
	}
	return s.repo.UpdateMany(ctx.UserContext(), records, s.updateCriteria...)
}

func (s *repositoryService[T]) Delete(ctx Context, record T) error {
	return s.repo.Delete(ctx.UserContext(), record)
}

func (s *repositoryService[T]) DeleteBatch(ctx Context, records []T) error {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		id := s.repo.Handlers().GetID(record)
		ids = append(ids, id.String())
	}
	if len(ids) == 0 {
		return nil
	}
	return s.repo.DeleteWhere(ctx.UserContext(), repository.DeleteByIDs(ids))
}

func (s *repositoryService[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	return s.repo.List(ctx.UserContext(), criteria...)
}

func (s *repositoryService[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	return s.repo.GetByID(ctx.UserContext(), id, criteria...)
}

type serviceFuncAdapter[T any] struct {
	defaults Service[T]
	funcs    ServiceFuncs[T]
}

func (a *serviceFuncAdapter[T]) Create(ctx Context, record T) (T, error) {
	if a.funcs.Create != nil {
		return a.funcs.Create(ctx, record)
	}
	return a.defaults.Create(ctx, record)
}

func (a *serviceFuncAdapter[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	if a.funcs.CreateBatch != nil {
		return a.funcs.CreateBatch(ctx, records)
	}
	return a.defaults.CreateBatch(ctx, records)
}

func (a *serviceFuncAdapter[T]) Update(ctx Context, record T) (T, error) {
	if a.funcs.Update != nil {
		return a.funcs.Update(ctx, record)
	}
	return a.defaults.Update(ctx, record)
}

func (a *serviceFuncAdapter[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	if a.funcs.UpdateBatch != nil {
		return a.funcs.UpdateBatch(ctx, records)
	}
	return a.defaults.UpdateBatch(ctx, records)
}

func (a *serviceFuncAdapter[T]) Delete(ctx Context, record T) error {
	if a.funcs.Delete != nil {
		return a.funcs.Delete(ctx, record)
	}
	return a.defaults.Delete(ctx, record)
}

func (a *serviceFuncAdapter[T]) DeleteBatch(ctx Context, records []T) error {
	if a.funcs.DeleteBatch != nil {
		return a.funcs.DeleteBatch(ctx, records)
	}
	return a.defaults.DeleteBatch(ctx, records)
}

func (a *serviceFuncAdapter[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	if a.funcs.Index != nil {
		return a.funcs.Index(ctx, criteria)
	}
	return a.defaults.Index(ctx, criteria)
}

func (a *serviceFuncAdapter[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	if a.funcs.Show != nil {
		return a.funcs.Show(ctx, id, criteria)
	}
	return a.defaults.Show(ctx, id, criteria)
}
