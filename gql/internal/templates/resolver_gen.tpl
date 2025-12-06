package resolvers

// {{ Notice }}

import (
	"context"
	"reflect"
	"time"

	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"

	"{{ ModelPackage }}"
)

type ScopeGuardFunc func(ctx context.Context, entity, action string) error
type ContextFactory func(ctx context.Context) crud.Context

func (r *Resolver) crudContext(ctx context.Context) crud.Context {
	if r.ContextFactory != nil {
		return r.ContextFactory(ctx)
	}
	return nil
}

func (r *Resolver) guard(ctx context.Context, entity, action string) error {
	if r.ScopeGuard != nil {
		if err := r.ScopeGuard(ctx, entity, action); err != nil {
			return err
		}
	}
	{% if PolicyHook %}return {{ PolicyHook }}(ctx, entity, action){% else %}return nil{% endif %}
}

func buildCriteria(p *model.PaginationInput, order []*model.OrderByInput, filters []*model.FilterInput) []repository.SelectCriteria {
	return nil
}

func applyInput[T any, I any](dst *T, src I) {
	dv := reflect.ValueOf(dst).Elem()
	sv := reflect.ValueOf(src)
	if sv.Kind() == reflect.Pointer && !sv.IsNil() {
		sv = sv.Elem()
	}
	for i := 0; i < sv.NumField(); i++ {
		sf := sv.Field(i)
		df := dv.FieldByName(sv.Type().Field(i).Name)
		if !df.IsValid() || !df.CanSet() {
			continue
		}
		if sf.Kind() == reflect.Pointer {
			if sf.IsNil() {
				continue
			}
			df.Set(sf.Elem())
			continue
		}
		df.Set(sf)
	}
}

func setID[T any](dst *T, id string) {
	dv := reflect.ValueOf(dst).Elem()
	for _, name := range []string{"ID", "Id"} {
		f := dv.FieldByName(name)
		if f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
			f.SetString(id)
			return
		}
	}
}

func asUUID(id string) model.UUID { return model.UUID(id) }

func asTime(t *time.Time) *model.Time {
	if t == nil {
		return nil
	}
	val := model.Time(*t)
	return &val
}

func setUUID(dst *string, data model.UUID) {
	*dst = string(data)
}

func setUUIDPtr(dst **string, data *model.UUID) {
	if data == nil {
		*dst = nil
		return
	}
	val := string(*data)
	*dst = &val
}

func setTimePtr(dst **time.Time, data *model.Time) {
	if data == nil {
		*dst = nil
		return
	}
	val := time.Time(*data)
	*dst = &val
}

{% for entity in ResolverEntities %}
func (r *Resolver) {{ entity.Name }}Service() crud.Service[model.{{ entity.Name }}] {
	if r.{{ entity.Name }}Svc == nil {
		panic("{{ entity.Name }}Service is not configured on Resolver")
	}
	return r.{{ entity.Name }}Svc
}

func (r *Resolver) Get{{ entity.Name }}(ctx context.Context, id string) (*model.{{ entity.Name }}, error) {
	if err := r.guard(ctx, "{{ entity.Name }}", "show"); err != nil {
		return nil, err
	}
	record, err := r.{{ entity.Name }}Service().Show(r.crudContext(ctx), id, nil)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Resolver) List{{ entity.Name }}(ctx context.Context, pagination *model.PaginationInput, orderBy []*model.OrderByInput, filter []*model.FilterInput) ([]*model.{{ entity.Name }}, error) {
	if err := r.guard(ctx, "{{ entity.Name }}", "index"); err != nil {
		return nil, err
	}
	criteria := buildCriteria(pagination, orderBy, filter)
	records, _, err := r.{{ entity.Name }}Service().Index(r.crudContext(ctx), criteria)
	if err != nil {
		return nil, err
	}
	result := make([]*model.{{ entity.Name }}, 0, len(records))
	for i := range records {
		result = append(result, &records[i])
	}
	return result, nil
}

func (r *Resolver) Create{{ entity.Name }}(ctx context.Context, input model.Create{{ entity.Name }}Input) (*model.{{ entity.Name }}, error) {
	if err := r.guard(ctx, "{{ entity.Name }}", "create"); err != nil {
		return nil, err
	}
	var record model.{{ entity.Name }}
	applyInput(&record, input)
	record, err := r.{{ entity.Name }}Service().Create(r.crudContext(ctx), record)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Resolver) Update{{ entity.Name }}(ctx context.Context, id string, input model.Update{{ entity.Name }}Input) (*model.{{ entity.Name }}, error) {
	if err := r.guard(ctx, "{{ entity.Name }}", "update"); err != nil {
		return nil, err
	}
	var record model.{{ entity.Name }}
	setID(&record, id)
	applyInput(&record, input)
	record, err := r.{{ entity.Name }}Service().Update(r.crudContext(ctx), record)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Resolver) Delete{{ entity.Name }}(ctx context.Context, id string) (bool, error) {
	if err := r.guard(ctx, "{{ entity.Name }}", "delete"); err != nil {
		return false, err
	}
	var record model.{{ entity.Name }}
	setID(&record, id)
	if err := r.{{ entity.Name }}Service().Delete(r.crudContext(ctx), record); err != nil {
		return false, err
	}
	return true, nil
}

{% endfor %}
