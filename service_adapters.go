package crud

import (
	"fmt"

	repository "github.com/goliatone/go-repository-bun"
)

// ReadService exposes read-only operations for controller adapters.
type ReadService[T any] interface {
	Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error)
	Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error)
}

// WriteService exposes write-only operations for controller adapters.
type WriteService[T any] interface {
	Create(ctx Context, record T) (T, error)
	CreateBatch(ctx Context, records []T) ([]T, error)

	Update(ctx Context, record T) (T, error)
	UpdateBatch(ctx Context, records []T) ([]T, error)

	Delete(ctx Context, record T) error
	DeleteBatch(ctx Context, records []T) error
}

// UnsupportedOperationError reports that a service adapter does not implement an operation.
type UnsupportedOperationError struct {
	Operation CrudOperation
}

func (e UnsupportedOperationError) Error() string {
	return fmt.Sprintf("crud: %s operation not supported", e.Operation)
}

// ReadOnlyService adapts a read-only service to the full Service interface.
// Write operations return UnsupportedOperationError.
func ReadOnlyService[T any](svc ReadService[T]) Service[T] {
	if svc == nil {
		return nil
	}
	return &readOnlyServiceAdapter[T]{read: svc}
}

// WriteOnlyService adapts a write-only service to the full Service interface.
// Read operations use the optional fallback service when provided.
func WriteOnlyService[T any](svc WriteService[T], fallback ...Service[T]) Service[T] {
	if svc == nil {
		return nil
	}
	var readFallback Service[T]
	if len(fallback) > 0 {
		readFallback = fallback[0]
	}
	return &writeOnlyServiceAdapter[T]{write: svc, readFallback: readFallback}
}

type readOnlyServiceAdapter[T any] struct {
	read ReadService[T]
}

func (s *readOnlyServiceAdapter[T]) Create(ctx Context, record T) (T, error) {
	var zero T
	return zero, UnsupportedOperationError{Operation: OpCreate}
}

func (s *readOnlyServiceAdapter[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	return nil, UnsupportedOperationError{Operation: OpCreateBatch}
}

func (s *readOnlyServiceAdapter[T]) Update(ctx Context, record T) (T, error) {
	var zero T
	return zero, UnsupportedOperationError{Operation: OpUpdate}
}

func (s *readOnlyServiceAdapter[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	return nil, UnsupportedOperationError{Operation: OpUpdateBatch}
}

func (s *readOnlyServiceAdapter[T]) Delete(ctx Context, record T) error {
	return UnsupportedOperationError{Operation: OpDelete}
}

func (s *readOnlyServiceAdapter[T]) DeleteBatch(ctx Context, records []T) error {
	return UnsupportedOperationError{Operation: OpDeleteBatch}
}

func (s *readOnlyServiceAdapter[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	return s.read.Index(ctx, criteria)
}

func (s *readOnlyServiceAdapter[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	return s.read.Show(ctx, id, criteria)
}

type writeOnlyServiceAdapter[T any] struct {
	write        WriteService[T]
	readFallback Service[T]
}

func (s *writeOnlyServiceAdapter[T]) Create(ctx Context, record T) (T, error) {
	return s.write.Create(ctx, record)
}

func (s *writeOnlyServiceAdapter[T]) CreateBatch(ctx Context, records []T) ([]T, error) {
	return s.write.CreateBatch(ctx, records)
}

func (s *writeOnlyServiceAdapter[T]) Update(ctx Context, record T) (T, error) {
	return s.write.Update(ctx, record)
}

func (s *writeOnlyServiceAdapter[T]) UpdateBatch(ctx Context, records []T) ([]T, error) {
	return s.write.UpdateBatch(ctx, records)
}

func (s *writeOnlyServiceAdapter[T]) Delete(ctx Context, record T) error {
	return s.write.Delete(ctx, record)
}

func (s *writeOnlyServiceAdapter[T]) DeleteBatch(ctx Context, records []T) error {
	return s.write.DeleteBatch(ctx, records)
}

func (s *writeOnlyServiceAdapter[T]) Index(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	if s.readFallback == nil {
		return nil, 0, UnsupportedOperationError{Operation: OpList}
	}
	return s.readFallback.Index(ctx, criteria)
}

func (s *writeOnlyServiceAdapter[T]) Show(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	if s.readFallback == nil {
		var zero T
		return zero, UnsupportedOperationError{Operation: OpRead}
	}
	return s.readFallback.Show(ctx, id, criteria)
}
