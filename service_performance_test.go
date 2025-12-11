package crud

import (
	"context"
	"testing"

	"github.com/goliatone/go-crud/pkg/activity"
)

// benchContext wraps mockContext and allows mutation of the user context so
// scope guards can attach actor/scope metadata during benchmarks.
type benchContext struct {
	*mockContext
}

func newBenchContext() *benchContext {
	return &benchContext{mockContext: newMockRequest()}
}

func (b *benchContext) SetUserContext(ctx context.Context) {
	b.userCtx = ctx
}

func (b *benchContext) Header(key string) string { return "" }

func buildBenchmarkServices() (Service[testModel], Service[testModel]) {
	repoBaseline := &stubRepo{
		showResp:   testModel{ID: "bench-id", Name: "baseline"},
		createResp: testModel{ID: "bench-id", Name: "baseline"},
	}
	repoWrapped := &stubRepo{
		showResp:   testModel{ID: "bench-id", Name: "wrapped"},
		createResp: testModel{ID: "bench-id", Name: "wrapped"},
	}

	wrapped := NewService(ServiceConfig[testModel]{
		Repository: repoWrapped,
		VirtualFields: &stubVirtualFields{
			calls: nil,
		},
		Validator: func(_ Context, _ testModel) error { return nil },
		Hooks: LifecycleHooks[testModel]{
			BeforeCreate: []HookFunc[testModel]{func(HookContext, testModel) error { return nil }},
			AfterCreate:  []HookFunc[testModel]{func(HookContext, testModel) error { return nil }},
			AfterRead:    []HookFunc[testModel]{func(HookContext, testModel) error { return nil }},
			AfterList:    []HookBatchFunc[testModel]{func(HookContext, []testModel) error { return nil }},
		},
		ScopeGuard: func(ctx Context, _ CrudOperation) (ActorContext, ScopeFilter, error) {
			var scope ScopeFilter
			scope.AddColumnFilter("tenant_id", "=", "tenant-123")
			return ActorContext{ActorID: "actor-123", TenantID: "tenant-123"}, scope, nil
		},
		FieldPolicy: func(FieldPolicyRequest[testModel]) (FieldPolicy, error) {
			return FieldPolicy{
				Name:  "bench",
				Allow: []string{"id", "name"},
			}, nil
		},
		ActivityHooks: activity.Hooks{
			activity.HookFunc(func(context.Context, activity.Event) error { return nil }),
		},
		ActivityConfig: activity.Config{Enabled: true},
		ResourceName:   "test_model",
	})

	return NewRepositoryService[testModel](repoBaseline), wrapped
}

func BenchmarkService_Create(b *testing.B) {
	baseline, wrapped := buildBenchmarkServices()
	record := testModel{ID: "bench-id", Name: "bench"}

	b.Run("repository_only", func(b *testing.B) {
		ctx := newBenchContext()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := baseline.Create(ctx, record); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("service_with_wrappers", func(b *testing.B) {
		ctx := newBenchContext()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := wrapped.Create(ctx, record); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestHotPathsVirtualFieldsAndScopeGuard(t *testing.T) {
	calls := []string{}
	repo := &stubRepo{
		listResp: []testModel{{ID: "abc", Name: "hot"}}, listCount: 1,
		showResp:   testModel{ID: "abc", Name: "hot"},
		createResp: testModel{ID: "abc", Name: "hot"},
	}

	service := NewService(ServiceConfig[testModel]{
		Repository: repo,
		VirtualFields: &stubVirtualFields{
			calls: &calls,
		},
		ScopeGuard: func(ctx Context, _ CrudOperation) (ActorContext, ScopeFilter, error) {
			var scope ScopeFilter
			scope.AddColumnFilter("tenant_id", "=", "tenant-hot")
			return ActorContext{ActorID: "actor-hot", TenantID: "tenant-hot"}, scope, nil
		},
		Hooks: LifecycleHooks[testModel]{
			AfterRead: []HookFunc[testModel]{func(HookContext, testModel) error { return nil }},
		},
	})

	ctx := newBenchContext()

	record, err := service.Show(ctx, "abc", nil)
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if record.ID == "" {
		t.Fatalf("expected record ID to be populated")
	}
	if len(calls) == 0 {
		t.Fatalf("virtual fields should run after load")
	}

	_, _, err = service.Index(ctx, nil)
	if err != nil {
		t.Fatalf("index failed: %v", err)
	}
	if repo.criteriaCount == 0 {
		t.Fatalf("scope guard should apply select criteria")
	}

	allocs := testing.AllocsPerRun(100, func() {
		_, _ = service.Show(ctx, "abc", nil)
	})
	if allocs > 20 {
		t.Fatalf("show hot path allocated too much: got %.2f allocs/op", allocs)
	}
}
