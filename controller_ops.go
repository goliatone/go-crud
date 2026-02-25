package crud

import (
	"fmt"
	"strings"

	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
)

// ShowByID resolves a single record using guard + field policy semantics.
func (c *Controller[T]) ShowByID(ctx Context, id string, criteria []repository.SelectCriteria) (T, error) {
	ctx = c.applyContextFactory(ctx)
	svc := c.resolvedReadService()
	meta, err := c.resolveGuardContext(ctx, OpRead)
	if err != nil {
		var zero T
		return zero, err
	}

	policy, err := c.resolveFieldPolicy(ctx, OpRead, meta)
	if err != nil {
		var zero T
		return zero, err
	}
	c.logFieldPolicyDecision(policy)
	c.attachHookContext(ctx, OpRead)

	effective := append([]repository.SelectCriteria(nil), criteria...)
	effective = c.applyScopeCriteria(effective, meta.scope)
	effective = c.applyFieldPolicyCriteria(effective, policy)

	record, err := svc.Show(ctx, strings.TrimSpace(id), effective)
	if err != nil {
		var zero T
		return zero, &NotFoundError{err}
	}
	applyFieldPolicyToRecord(record, policy)
	return record, nil
}

// IndexWith resolves records using guard + field policy semantics and provided criteria.
func (c *Controller[T]) IndexWith(ctx Context, criteria []repository.SelectCriteria) ([]T, int, error) {
	ctx = c.applyContextFactory(ctx)
	svc := c.resolvedReadService()
	meta, err := c.resolveGuardContext(ctx, OpList)
	if err != nil {
		return nil, 0, err
	}

	policy, err := c.resolveFieldPolicy(ctx, OpList, meta)
	if err != nil {
		return nil, 0, err
	}
	c.logFieldPolicyDecision(policy)
	c.attachHookContext(ctx, OpList)

	effective := append([]repository.SelectCriteria(nil), criteria...)
	effective = c.applyScopeCriteria(effective, meta.scope)
	effective = c.applyFieldPolicyCriteria(effective, policy)

	records, count, err := svc.Index(ctx, effective)
	if err != nil {
		return nil, 0, err
	}
	applyFieldPolicyToSlice(records, policy)
	return records, count, nil
}

// CreateRecord persists a single record using guard/activity semantics.
func (c *Controller[T]) CreateRecord(ctx Context, record T) (T, error) {
	ctx = c.applyContextFactory(ctx)
	svc := c.resolvedWriteService()
	meta, err := c.resolveGuardContext(ctx, OpCreate)
	if err != nil {
		var zero T
		return zero, err
	}

	policy, err := c.resolveFieldPolicy(ctx, OpCreate, meta)
	if err != nil {
		var zero T
		return zero, err
	}
	c.logFieldPolicyDecision(policy)
	c.attachHookContext(ctx, OpCreate)

	createdRecord, err := svc.Create(ctx, record)
	if err != nil {
		c.emitActivityEvents(ctx, OpCreate, meta, []T{record}, err)
		var zero T
		return zero, err
	}

	c.emitActivityEvents(ctx, OpCreate, meta, []T{createdRecord}, nil)
	applyFieldPolicyToRecord(createdRecord, policy)
	return createdRecord, nil
}

// CreateRecords persists records in batch using guard/activity semantics.
func (c *Controller[T]) CreateRecords(ctx Context, records []T) ([]T, error) {
	ctx = c.applyContextFactory(ctx)
	svc := c.resolvedWriteService()
	meta, err := c.resolveGuardContext(ctx, OpCreateBatch)
	if err != nil {
		return nil, err
	}

	policy, err := c.resolveFieldPolicy(ctx, OpCreateBatch, meta)
	if err != nil {
		return nil, err
	}
	c.logFieldPolicyDecision(policy)
	c.attachHookContext(ctx, OpCreateBatch)

	createdRecords, err := svc.CreateBatch(ctx, records)
	if err != nil {
		c.emitActivityEvents(ctx, OpCreateBatch, meta, records, err)
		return nil, err
	}

	c.emitActivityEvents(ctx, OpCreateBatch, meta, createdRecords, nil)
	applyFieldPolicyToSlice(createdRecords, policy)
	return createdRecords, nil
}

// UpdateRecord updates a single record while preserving merge + virtual map semantics.
func (c *Controller[T]) UpdateRecord(ctx Context, id string, patch T) (T, error) {
	ctx = c.applyContextFactory(ctx)
	svc := c.resolvedWriteService()
	meta, err := c.resolveGuardContext(ctx, OpUpdate)
	if err != nil {
		var zero T
		return zero, err
	}

	policy, err := c.resolveFieldPolicy(ctx, OpUpdate, meta)
	if err != nil {
		var zero T
		return zero, err
	}
	c.logFieldPolicyDecision(policy)
	c.attachHookContext(ctx, OpUpdate)

	idStr := strings.TrimSpace(id)
	parsedID, err := uuid.Parse(idStr)
	if err != nil {
		c.emitActivityEvents(ctx, OpUpdate, meta, nil, err)
		var zero T
		return zero, &ValidationError{err}
	}
	c.Repo.Handlers().SetID(patch, parsedID)

	criteria := c.applyScopeCriteria(nil, meta.scope)
	criteria = c.applyFieldPolicyCriteria(criteria, policy)
	existingRecord, err := svc.Show(ctx, idStr, criteria)
	if err != nil {
		c.emitActivityEvents(ctx, OpUpdate, meta, []T{patch}, err)
		var zero T
		return zero, &NotFoundError{err}
	}

	record, err := mergeRecordWithExisting(patch, existingRecord)
	if err != nil {
		c.emitActivityEvents(ctx, OpUpdate, meta, []T{patch}, err)
		var zero T
		return zero, err
	}
	record = mergeVirtualMaps(existingRecord, record, c.virtualFieldDefs, c.mergePolicy)

	updatedRecord, err := svc.Update(ctx, record)
	if err != nil {
		c.emitActivityEvents(ctx, OpUpdate, meta, []T{record}, err)
		var zero T
		return zero, err
	}

	c.emitActivityEvents(ctx, OpUpdate, meta, []T{updatedRecord}, nil)
	applyFieldPolicyToRecord(updatedRecord, policy)
	return updatedRecord, nil
}

// UpdateRecords updates records in batch while preserving merge semantics.
func (c *Controller[T]) UpdateRecords(ctx Context, records []T) ([]T, error) {
	ctx = c.applyContextFactory(ctx)
	svc := c.resolvedWriteService()
	meta, err := c.resolveGuardContext(ctx, OpUpdateBatch)
	if err != nil {
		return nil, err
	}

	policy, err := c.resolveFieldPolicy(ctx, OpUpdateBatch, meta)
	if err != nil {
		return nil, err
	}
	c.logFieldPolicyDecision(policy)
	c.attachHookContext(ctx, OpUpdateBatch)

	criteria := c.applyScopeCriteria(nil, meta.scope)
	criteria = c.applyFieldPolicyCriteria(criteria, policy)
	for i, rec := range records {
		id := c.Repo.Handlers().GetID(rec)
		existing, err := svc.Show(ctx, id.String(), criteria)
		if err != nil {
			c.emitActivityEvents(ctx, OpUpdateBatch, meta, records, err)
			return nil, &NotFoundError{err}
		}
		merged, err := mergeRecordWithExisting(rec, existing)
		if err != nil {
			c.emitActivityEvents(ctx, OpUpdateBatch, meta, records, err)
			return nil, err
		}
		merged = mergeVirtualMaps(existing, merged, c.virtualFieldDefs, c.mergePolicy)
		records[i] = merged
	}

	updatedRecords, err := svc.UpdateBatch(ctx, records)
	if err != nil {
		c.emitActivityEvents(ctx, OpUpdateBatch, meta, records, err)
		return nil, err
	}

	c.emitActivityEvents(ctx, OpUpdateBatch, meta, updatedRecords, nil)
	applyFieldPolicyToSlice(updatedRecords, policy)
	return updatedRecords, nil
}

// DeleteByID deletes a single record after scoped lookup.
func (c *Controller[T]) DeleteByID(ctx Context, id string) error {
	ctx = c.applyContextFactory(ctx)
	svc := c.resolvedWriteService()
	meta, err := c.resolveGuardContext(ctx, OpDelete)
	if err != nil {
		return err
	}

	policy, err := c.resolveFieldPolicy(ctx, OpDelete, meta)
	if err != nil {
		return err
	}
	c.logFieldPolicyDecision(policy)
	c.attachHookContext(ctx, OpDelete)

	idStr := strings.TrimSpace(id)
	criteria := c.applyScopeCriteria(nil, meta.scope)
	criteria = c.applyFieldPolicyCriteria(criteria, policy)
	record, err := svc.Show(ctx, idStr, criteria)
	if err != nil {
		c.emitActivityEvents(ctx, OpDelete, meta, nil, err)
		return &NotFoundError{err}
	}

	if err := svc.Delete(ctx, record); err != nil {
		c.emitActivityEvents(ctx, OpDelete, meta, []T{record}, err)
		return err
	}

	c.emitActivityEvents(ctx, OpDelete, meta, []T{record}, nil)
	return nil
}

// DeleteRecords deletes records in batch.
func (c *Controller[T]) DeleteRecords(ctx Context, records []T) error {
	ctx = c.applyContextFactory(ctx)
	svc := c.resolvedWriteService()
	meta, err := c.resolveGuardContext(ctx, OpDeleteBatch)
	if err != nil {
		return err
	}

	policy, err := c.resolveFieldPolicy(ctx, OpDeleteBatch, meta)
	if err != nil {
		return err
	}
	c.logFieldPolicyDecision(policy)
	c.attachHookContext(ctx, OpDeleteBatch)

	if err := svc.DeleteBatch(ctx, records); err != nil {
		c.emitActivityEvents(ctx, OpDeleteBatch, meta, records, err)
		return err
	}

	c.emitActivityEvents(ctx, OpDeleteBatch, meta, records, nil)
	return nil
}

// RecordsFromIDs builds records with IDs set using repository handlers.
func (c *Controller[T]) RecordsFromIDs(ids []string) ([]T, error) {
	return c.recordsFromIDs(ids)
}

func (c *Controller[T]) recordsFromIDs(ids []string) ([]T, error) {
	handlers := c.Repo.Handlers()
	if handlers.SetID == nil {
		return nil, fmt.Errorf("missing record id setter")
	}

	records := make([]T, 0, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, fmt.Errorf("empty record id")
		}
		parsed, err := uuid.Parse(id)
		if err != nil {
			return nil, err
		}

		var record T
		if handlers.NewRecord != nil {
			record = handlers.NewRecord()
		}
		handlers.SetID(record, parsed)
		records = append(records, record)
	}

	return records, nil
}
