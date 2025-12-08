package resolvers

// {{ Notice }}

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/uptrace/bun"

	"{{ ModelPackage }}"
)

type ScopeGuardFunc func(ctx context.Context, entity, action string) error
type ContextFactory func(ctx context.Context) crud.Context

type criteriaField struct {
	Column   string
	Relation string
}

var criteriaConfig = map[string]map[string]criteriaField{
{% for entity in ResolverEntities %}
	"{{ entity.Name }}": {
	{% if Criteria and Criteria[entity.Name] %}
	{% for field in Criteria[entity.Name] %}		"{{ field.Field | lower }}": {Column: "{{ field.Column }}"{% if field.Relation %}, Relation: "{{ field.Relation }}"{% endif %}},
	{% endfor %}
	{% endif %}	},
{% endfor %}
}

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

func buildCriteria(entity string, p *model.PaginationInput, order []*model.OrderByInput, filters []*model.FilterInput) []repository.SelectCriteria {
	fields := criteriaConfig[entity]
	criteria := make([]repository.SelectCriteria, 0, 1+len(order)+len(filters))

	if p != nil {
		if p.Limit != nil && *p.Limit > 0 {
			limit := *p.Limit
			criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Limit(limit)
			})
		}
		if p.Offset != nil && *p.Offset >= 0 {
			offset := *p.Offset
			criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Offset(offset)
			})
		}
	}

	for _, ob := range order {
		if ob == nil || ob.Field == "" {
			continue
		}
		field, ok := lookupField(fields, ob.Field)
		if !ok {
			continue
		}

		dir := normalizeDirection(ob.Direction)
		column := field.Column
		relation := field.Relation

		criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
			if relation != "" {
				q = q.Relation(relation)
			}
			return q.OrderExpr("? ?", bun.Safe(column), bun.Safe(dir))
		})
	}

	for _, filter := range filters {
		if filter == nil || filter.Field == "" {
			continue
		}
		field, ok := lookupField(fields, filter.Field)
		if !ok {
			continue
		}

		op := normalizeOperator(filter.Operator)
		if op == "" {
			continue
		}

		value := strings.TrimSpace(filter.Value)
		if value == "" {
			continue
		}

		column := field.Column
		relation := field.Relation

		switch op {
		case "IN", "NOT IN":
			values := splitList(value)
			if len(values) == 0 {
				continue
			}
			criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
				if relation != "" {
					q = q.Relation(relation)
				}
				expr := fmt.Sprintf("%s IN (?)", column)
				if op == "NOT IN" {
					expr = fmt.Sprintf("%s NOT IN (?)", column)
				}
				return q.Where(expr, bun.In(values))
			})
		default:
			criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
				if relation != "" {
					q = q.Relation(relation)
				}
				return q.Where(fmt.Sprintf("%s %s ?", column, op), value)
			})
		}
	}

	return criteria
}

func paginationBounds(p *model.PaginationInput, returned int) (limit int, offset int) {
	if p != nil {
		if p.Limit != nil && *p.Limit > 0 {
			limit = *p.Limit
		}
		if p.Offset != nil && *p.Offset > 0 {
			offset = *p.Offset
		}
	}
	if limit <= 0 {
		limit = returned
	}
	return limit, offset
}

func buildPageInfoMeta(offset, count, limit, total int) *model.PageInfo {
	if limit < 0 {
		limit = 0
	}
	if count < 0 {
		count = 0
	}

	hasNext := offset+count < total
	if limit > 0 {
		hasNext = offset+limit < total
	}

	var startCursor, endCursor string
	if count > 0 {
		startCursor = encodeCursor(offset)
		endCursor = encodeCursor(offset + count - 1)
	}

	return &model.PageInfo{
		Total:           total,
		HasNextPage:     hasNext,
		HasPreviousPage: offset > 0,
		StartCursor:     startCursor,
		EndCursor:       endCursor,
	}
}

func encodeCursor(offset int) string {
	if offset < 0 {
		offset = 0
	}
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("cursor:%d", offset)))
}

func normalizeDirection(dir *model.OrderDirection) string {
	if dir == nil {
		return "ASC"
	}
	value := strings.ToUpper(string(*dir))
	if value != "DESC" {
		return "ASC"
	}
	return value
}

func normalizeOperator(op model.FilterOperator) string {
	switch strings.ToUpper(string(op)) {
	case "EQ":
		return "="
	case "NE":
		return "<>"
	case "GT":
		return ">"
	case "LT":
		return "<"
	case "GTE":
		return ">="
	case "LTE":
		return "<="
	case "ILIKE":
		return "ILIKE"
	case "LIKE":
		return "LIKE"
	case "IN":
		return "IN"
	case "NOT_IN":
		return "NOT IN"
	default:
		return ""
	}
}

func lookupField(fields map[string]criteriaField, raw string) (criteriaField, bool) {
	if len(fields) == 0 {
		return criteriaField{}, false
	}
	key := strings.ToLower(strings.TrimSpace(raw))
	value, ok := fields[key]
	return value, ok
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		val := strings.TrimSpace(part)
		if val == "" {
			continue
		}
		out = append(out, val)
	}
	return out
}

func applyInput[T any, I any](dst *T, src I) {
	if dst == nil {
		return
	}

	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return
	}

	sv := reflect.ValueOf(src)
	if sv.Kind() == reflect.Pointer {
		if sv.IsNil() {
			return
		}
		sv = sv.Elem()
	}

	dv = dv.Elem()

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
			sf = sf.Elem()
		}
		assignField(df, sf)
	}
}

func assignField(dst, src reflect.Value) {
	switch {
	case !dst.IsValid() || !dst.CanSet():
		return
	case src.Type().AssignableTo(dst.Type()):
		dst.Set(src)
	case dst.Kind() == reflect.Pointer && src.Type().AssignableTo(dst.Type().Elem()):
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst.Elem().Set(src)
	case src.Type().ConvertibleTo(dst.Type()):
		dst.Set(src.Convert(dst.Type()))
	case dst.Kind() == reflect.Pointer && src.Type().ConvertibleTo(dst.Type().Elem()):
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst.Elem().Set(src.Convert(dst.Type().Elem()))
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

func (r *Resolver) List{{ entity.Name }}(ctx context.Context, pagination *model.PaginationInput, orderBy []*model.OrderByInput, filter []*model.FilterInput) (*model.{{ entity.Name }}Connection, error) {
	if err := r.guard(ctx, "{{ entity.Name }}", "index"); err != nil {
		return nil, err
	}
	criteria := buildCriteria("{{ entity.Name }}", pagination, orderBy, filter)
	records, total, err := r.{{ entity.Name }}Service().Index(r.crudContext(ctx), criteria)
	if err != nil {
		return nil, err
	}
	limit, offset := paginationBounds(pagination, len(records))
	result := make([]*model.{{ entity.Name }}, 0, len(records))
	for i := range records {
		result = append(result, &records[i])
	}
	edges := make([]*model.{{ entity.Name }}Edge, 0, len(result))
	for i := range result {
		edges = append(edges, &model.{{ entity.Name }}Edge{
			Cursor: encodeCursor(offset + i),
			Node:   result[i],
		})
	}
	return &model.{{ entity.Name }}Connection{
		Edges:    edges,
		PageInfo: buildPageInfoMeta(offset, len(result), limit, total),
	}, nil
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
	record, err := r.{{ entity.Name }}Service().Show(r.crudContext(ctx), id, nil)
	if err != nil {
		return nil, err
	}
	setID(&record, id)
	applyInput(&record, input)
	record, err = r.{{ entity.Name }}Service().Update(r.crudContext(ctx), record)
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
