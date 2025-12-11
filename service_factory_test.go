package crud

import (
	"context"
	"errors"
	"testing"

	"github.com/goliatone/go-crud/pkg/activity"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type testModel struct {
	ID       string
	Metadata map[string]any
	Name     string
}

type stubRepo struct {
	calls         *[]string
	listResp      []testModel
	listCount     int
	showResp      testModel
	createResp    testModel
	updateResp    testModel
	deleteErr     error
	criteriaCount int
}

func (r *stubRepo) Raw(ctx context.Context, sql string, args ...any) ([]testModel, error) {
	return nil, nil
}
func (r *stubRepo) RawTx(ctx context.Context, tx bun.IDB, sql string, args ...any) ([]testModel, error) {
	return nil, nil
}
func (r *stubRepo) Get(ctx context.Context, criteria ...repository.SelectCriteria) (testModel, error) {
	return r.showResp, nil
}
func (r *stubRepo) GetTx(ctx context.Context, tx bun.IDB, criteria ...repository.SelectCriteria) (testModel, error) {
	return r.showResp, nil
}
func (r *stubRepo) GetByID(ctx context.Context, id string, criteria ...repository.SelectCriteria) (testModel, error) {
	r.criteriaCount = len(criteria)
	if r.calls != nil {
		*r.calls = append(*r.calls, "repo:get")
	}
	return r.showResp, nil
}
func (r *stubRepo) GetByIDTx(ctx context.Context, tx bun.IDB, id string, criteria ...repository.SelectCriteria) (testModel, error) {
	return r.GetByID(ctx, id, criteria...)
}
func (r *stubRepo) List(ctx context.Context, criteria ...repository.SelectCriteria) ([]testModel, int, error) {
	r.criteriaCount = len(criteria)
	if r.calls != nil {
		*r.calls = append(*r.calls, "repo:list")
	}
	return r.listResp, r.listCount, nil
}
func (r *stubRepo) ListTx(ctx context.Context, tx bun.IDB, criteria ...repository.SelectCriteria) ([]testModel, int, error) {
	return r.List(ctx, criteria...)
}
func (r *stubRepo) Count(ctx context.Context, criteria ...repository.SelectCriteria) (int, error) {
	return 0, nil
}
func (r *stubRepo) CountTx(ctx context.Context, tx bun.IDB, criteria ...repository.SelectCriteria) (int, error) {
	return 0, nil
}
func (r *stubRepo) Create(ctx context.Context, record testModel, criteria ...repository.InsertCriteria) (testModel, error) {
	if r.calls != nil {
		*r.calls = append(*r.calls, "repo:create")
	}
	if r.createResp.ID != "" {
		return r.createResp, nil
	}
	return record, nil
}
func (r *stubRepo) CreateTx(ctx context.Context, tx bun.IDB, record testModel, criteria ...repository.InsertCriteria) (testModel, error) {
	return r.Create(ctx, record, criteria...)
}
func (r *stubRepo) CreateMany(ctx context.Context, records []testModel, criteria ...repository.InsertCriteria) ([]testModel, error) {
	if r.calls != nil {
		*r.calls = append(*r.calls, "repo:createMany")
	}
	return records, nil
}
func (r *stubRepo) CreateManyTx(ctx context.Context, tx bun.IDB, records []testModel, criteria ...repository.InsertCriteria) ([]testModel, error) {
	return r.CreateMany(ctx, records, criteria...)
}
func (r *stubRepo) GetOrCreate(ctx context.Context, record testModel) (testModel, error) {
	return record, nil
}
func (r *stubRepo) GetOrCreateTx(ctx context.Context, tx bun.IDB, record testModel) (testModel, error) {
	return record, nil
}
func (r *stubRepo) GetByIdentifier(ctx context.Context, identifier string, criteria ...repository.SelectCriteria) (testModel, error) {
	return testModel{}, nil
}
func (r *stubRepo) GetByIdentifierTx(ctx context.Context, tx bun.IDB, identifier string, criteria ...repository.SelectCriteria) (testModel, error) {
	return testModel{}, nil
}
func (r *stubRepo) Update(ctx context.Context, record testModel, criteria ...repository.UpdateCriteria) (testModel, error) {
	if r.calls != nil {
		*r.calls = append(*r.calls, "repo:update")
	}
	if r.updateResp.ID != "" {
		return r.updateResp, nil
	}
	return record, nil
}
func (r *stubRepo) UpdateTx(ctx context.Context, tx bun.IDB, record testModel, criteria ...repository.UpdateCriteria) (testModel, error) {
	return r.Update(ctx, record, criteria...)
}
func (r *stubRepo) UpdateMany(ctx context.Context, records []testModel, criteria ...repository.UpdateCriteria) ([]testModel, error) {
	return records, nil
}
func (r *stubRepo) UpdateManyTx(ctx context.Context, tx bun.IDB, records []testModel, criteria ...repository.UpdateCriteria) ([]testModel, error) {
	return records, nil
}
func (r *stubRepo) Upsert(ctx context.Context, record testModel, criteria ...repository.UpdateCriteria) (testModel, error) {
	return record, nil
}
func (r *stubRepo) UpsertTx(ctx context.Context, tx bun.IDB, record testModel, criteria ...repository.UpdateCriteria) (testModel, error) {
	return record, nil
}
func (r *stubRepo) UpsertMany(ctx context.Context, records []testModel, criteria ...repository.UpdateCriteria) ([]testModel, error) {
	return records, nil
}
func (r *stubRepo) UpsertManyTx(ctx context.Context, tx bun.IDB, records []testModel, criteria ...repository.UpdateCriteria) ([]testModel, error) {
	return records, nil
}
func (r *stubRepo) Delete(ctx context.Context, record testModel) error {
	if r.calls != nil {
		*r.calls = append(*r.calls, "repo:delete")
	}
	return r.deleteErr
}
func (r *stubRepo) DeleteTx(ctx context.Context, tx bun.IDB, record testModel) error {
	return r.Delete(ctx, record)
}
func (r *stubRepo) DeleteMany(ctx context.Context, criteria ...repository.DeleteCriteria) error {
	return r.deleteErr
}
func (r *stubRepo) DeleteManyTx(ctx context.Context, tx bun.IDB, criteria ...repository.DeleteCriteria) error {
	return r.deleteErr
}
func (r *stubRepo) DeleteWhere(ctx context.Context, criteria ...repository.DeleteCriteria) error {
	return r.deleteErr
}
func (r *stubRepo) DeleteWhereTx(ctx context.Context, tx bun.IDB, criteria ...repository.DeleteCriteria) error {
	return r.deleteErr
}
func (r *stubRepo) ForceDelete(ctx context.Context, record testModel) error { return r.deleteErr }
func (r *stubRepo) ForceDeleteTx(ctx context.Context, tx bun.IDB, record testModel) error {
	return r.deleteErr
}
func (r *stubRepo) Handlers() repository.ModelHandlers[testModel] {
	return repository.ModelHandlers[testModel]{
		NewRecord: func() testModel { return testModel{} },
		GetID: func(t testModel) uuid.UUID {
			if t.ID == "" {
				return uuid.Nil
			}
			id, _ := uuid.Parse(t.ID)
			return id
		},
		SetID:              func(t testModel, id uuid.UUID) {},
		GetIdentifier:      func() string { return "" },
		GetIdentifierValue: func(t testModel) string { return t.ID },
		ResolveIdentifier:  nil,
	}
}
func (r *stubRepo) RegisterScope(name string, scope repository.ScopeDefinition) {}
func (r *stubRepo) SetScopeDefaults(defaults repository.ScopeDefaults) error    { return nil }
func (r *stubRepo) GetScopeDefaults() repository.ScopeDefaults                  { return repository.ScopeDefaults{} }
func (r *stubRepo) Validate() error                                             { return nil }
func (r *stubRepo) MustValidate()                                               {}

// ---- virtual field stub ----

type stubVirtualFields struct{ calls *[]string }

func (s *stubVirtualFields) BeforeSave(hctx HookContext, record testModel) error {
	if s.calls != nil {
		*s.calls = append(*s.calls, "virtual:before")
	}
	return nil
}
func (s *stubVirtualFields) AfterLoad(hctx HookContext, record testModel) error {
	if s.calls != nil {
		*s.calls = append(*s.calls, "virtual:after")
	}
	return nil
}
func (s *stubVirtualFields) AfterLoadBatch(hctx HookContext, records []testModel) error {
	if s.calls != nil {
		*s.calls = append(*s.calls, "virtual:afterBatch")
	}
	return nil
}

func TestNewService_ComposesWrappersInDefaultOrder(t *testing.T) {
	calls := []string{}
	repo := &stubRepo{calls: &calls}

	vf := &stubVirtualFields{calls: &calls}
	validator := func(ctx Context, m testModel) error {
		calls = append(calls, "validate")
		return nil
	}
	hooks := LifecycleHooks[testModel]{
		BeforeCreate: []HookFunc[testModel]{func(hctx HookContext, m testModel) error {
			calls = append(calls, "hook:beforeCreate")
			return nil
		}},
		AfterCreate: []HookFunc[testModel]{func(hctx HookContext, m testModel) error {
			calls = append(calls, "hook:afterCreate")
			return nil
		}},
	}
	scopeGuard := func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
		calls = append(calls, "scope:guard")
		sf := ScopeFilter{}
		sf.AddColumnFilter("tenant_id", "=", "t1")
		return ActorContext{ActorID: "actor"}, sf, nil
	}
	fieldPolicyProvider := func(req FieldPolicyRequest[testModel]) (FieldPolicy, error) {
		calls = append(calls, "fieldpolicy:resolve")
		return FieldPolicy{}, nil
	}
	captured := activity.CaptureHook{}

	svc := NewService(ServiceConfig[testModel]{
		Repository:     repo,
		VirtualFields:  vf,
		Validator:      validator,
		Hooks:          hooks,
		ScopeGuard:     scopeGuard,
		FieldPolicy:    fieldPolicyProvider,
		ActivityHooks:  activity.Hooks{captured.Hook()},
		ActivityConfig: activity.Config{Enabled: true},
	})

	ctx := newMockRequest()
	model := testModel{ID: "abc", Name: "demo"}

	created, err := svc.Create(ctx, model)
	require.NoError(t, err)
	assert.Equal(t, model, created)

	expectedOrder := []string{
		"scope:guard",
		"hook:beforeCreate",
		"validate",
		"virtual:before",
		"repo:create",
		"virtual:after",
		"hook:afterCreate",
		"fieldpolicy:resolve",
	}
	assert.Equal(t, expectedOrder, calls)

	// activity emitted with default channel; object type/id may be empty but hook should have been invoked
	emitted := captured.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, string(OpCreate), emitted[0].Verb)
}

func TestNewService_IndexAppliesScopeAndFieldPolicy(t *testing.T) {
	calls := []string{}
	repo := &stubRepo{calls: &calls, listResp: []testModel{{ID: "a"}}, listCount: 1}

	scopeGuard := func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error) {
		calls = append(calls, "scope:guard")
		sf := ScopeFilter{}
		sf.AddColumnFilter("tenant_id", "=", "t1")
		return ActorContext{}, sf, nil
	}

	fieldPolicyProvider := func(req FieldPolicyRequest[testModel]) (FieldPolicy, error) {
		calls = append(calls, "fieldpolicy:resolve")
		sf := ScopeFilter{}
		sf.AddColumnFilter("org_id", "=", "o1")
		return FieldPolicy{RowFilter: sf}, nil
	}

	vf := &stubVirtualFields{calls: &calls}

	svc := NewService(ServiceConfig[testModel]{
		Repository:    repo,
		ScopeGuard:    scopeGuard,
		FieldPolicy:   fieldPolicyProvider,
		VirtualFields: vf,
	})

	ctx := newMockRequest()
	records, total, err := svc.Index(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, records, 1)

	assert.Equal(t, 3, repo.criteriaCount, "scope + field policy criteria should be applied")
	assert.Contains(t, calls, "virtual:afterBatch")
}

func TestNewService_NoOptionalLayersFallsBackToRepo(t *testing.T) {
	repo := &stubRepo{createResp: testModel{ID: "xyz"}}
	svc := NewService(ServiceConfig[testModel]{Repository: repo})

	ctx := newMockRequest()
	created, err := svc.Create(ctx, testModel{ID: "xyz"})
	require.NoError(t, err)
	assert.Equal(t, "xyz", created.ID)
}

func TestValidationFailureShortCircuits(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(ServiceConfig[testModel]{
		Repository: repo,
		Validator: func(ctx Context, m testModel) error {
			return errors.New("invalid")
		},
	})

	_, err := svc.Create(newMockRequest(), testModel{ID: "bad"})
	require.Error(t, err)
}
