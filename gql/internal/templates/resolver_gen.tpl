package resolvers

// {{ Notice }}

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"time"
{% if NeedsErrorsImport %}	"errors"
{% endif %}
{% if AuthEnabled and AuthImportRequired %}	auth "{{ AuthPackage }}"
{% endif %}
{% if AuthEnabled %}	"github.com/goliatone/go-crud/gql/helpers"
{% endif %}

{% for imp in Hooks.Imports %}	"{{ imp }}"
{% endfor %}
	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/uptrace/bun"
{% if EmitDataloader %}
	"{{ DataloaderPackage }}"
{% endif %}

	"{{ ModelPackage }}"
)

type ScopeGuardFunc func(ctx context.Context, entity, action string) error
type ContextFactory func(ctx context.Context) crud.Context

type criteriaField struct {
	Column       string
	Relation     string
	RelationType string
	PivotTable   string
	SourceColumn string
	TargetColumn string
	SourcePivot  string
	TargetPivot  string
	TargetTable  string
}

var criteriaConfig = map[string]map[string]criteriaField{
{% for entity in ResolverEntities %}
	"{{ entity.Name }}": {
	{% if Criteria and Criteria[entity.Name] %}
	{% for field in Criteria[entity.Name] %}		"{{ field.Field | lower }}": {Column: "{{ field.Column }}"{% if field.Relation %}, Relation: "{{ field.Relation }}"{% endif %}{% if field.RelationType %}, RelationType: "{{ field.RelationType }}"{% endif %}{% if field.PivotTable %}, PivotTable: "{{ field.PivotTable }}"{% endif %}{% if field.SourceColumn %}, SourceColumn: "{{ field.SourceColumn }}"{% endif %}{% if field.TargetColumn %}, TargetColumn: "{{ field.TargetColumn }}"{% endif %}{% if field.SourcePivot %}, SourcePivot: "{{ field.SourcePivot }}"{% endif %}{% if field.TargetPivot %}, TargetPivot: "{{ field.TargetPivot }}"{% endif %}{% if field.TargetTable %}, TargetTable: "{{ field.TargetTable }}"{% endif %}},
	{% endfor %}
	{% endif %}	},
{% endfor %}
}

func findOriginalNameForJoinKey(entity, joinKey string) string {
	joinKey = strings.ToLower(strings.TrimSpace(joinKey))
	if joinKey == "" {
		return ""
	}
	base := strings.SplitN(joinKey, ".", 2)[0]
	fields, ok := criteriaConfig[entity]
	if !ok {
		return base
	}
	if field, ok := fields[base]; ok && field.Relation != "" {
		return field.Relation
	}
	for key, field := range fields {
		prefix := strings.SplitN(key, ".", 2)[0]
		if prefix == base && field.Relation != "" {
			return field.Relation
		}
	}
	return base
}

{% if Subscriptions %}var subscriptionTopics = map[string]map[string]string{}

func init() {
{% for sub in Subscriptions %}	addSubscriptionTopic("{{ sub.Entity }}", "{{ sub.Event }}", "{{ sub.Topic }}")
{% endfor %}}

func addSubscriptionTopic(entity, event, topic string) {
	if topic == "" {
		return
	}
	if subscriptionTopics[entity] == nil {
		subscriptionTopics[entity] = make(map[string]string)
	}
	subscriptionTopics[entity][strings.ToLower(event)] = topic
}

func subscriptionTopic(entity, event string) string {
	events, ok := subscriptionTopics[entity]
	if !ok {
		return ""
	}
	return events[strings.ToLower(event)]
}

func hasSubscription(entity, event string) bool {
	return subscriptionTopic(entity, event) != ""
}
{% endif %}

{% if AuthEnabled %}func GraphQLContext(ctx context.Context) crud.Context {
	crudCtx := helpers.GraphQLToCrudContext(ctx)
	if crudCtx == nil {
		return nil
	}

	base := crudCtx.UserContext()
	if existing := crud.ActorFromContext(base); !existing.IsZero() {
		return crudCtx
	}

	actor, ok := auth.ActorFromContext(base)
	if !ok || actor == nil {
		return crudCtx
	}

	mapped := crud.ActorContext{
		ActorID:        actor.ActorID,
		Subject:        actor.Subject,
		Role:           actor.Role,
		ResourceRoles:  actor.ResourceRoles,
		TenantID:       actor.TenantID,
		OrganizationID: actor.OrganizationID,
		Metadata:       actor.Metadata,
		ImpersonatorID: actor.ImpersonatorID,
		IsImpersonated: actor.IsImpersonated,
	}

	updated := crud.ContextWithActor(base, mapped)
	if setter, ok := crudCtx.(interface{ SetUserContext(context.Context) }); ok && updated != nil {
		setter.SetUserContext(updated)
	}
	return crudCtx
}

{% endif %}func (r *Resolver) crudContext(ctx context.Context) crud.Context {
	if r.ContextFactory != nil {
		return r.ContextFactory(ctx)
	}
	{% if AuthEnabled %}return GraphQLContext(ctx){% else %}return nil{% endif %}
}

func (r *Resolver) guard(ctx context.Context, entity, action string) error {
	if r.ScopeGuard != nil {
		if err := r.ScopeGuard(ctx, entity, action); err != nil {
			return err
		}
	}
	{% if PolicyHook %}return {{ PolicyHook }}(ctx, entity, action){% else %}return nil{% endif %}
}

{% if Subscriptions %}func (r *Resolver) publishEvent(ctx context.Context, entity, event string, payload any) error {
	if r.Events == nil {
		return nil
	}
	topic := subscriptionTopic(entity, event)
	if topic == "" {
		return nil
	}
	return r.Events.Publish(ctx, topic, payload)
}

func (r *Resolver) subscribe(ctx context.Context, entity, event string) (<-chan EventMessage, error) {
	if r.Events == nil {
		return nil, errors.New("subscriptions are not configured")
	}
	topic := subscriptionTopic(entity, event)
	if topic == "" {
		return nil, errors.New("subscription is not enabled for " + entity + ":" + event)
	}
	return r.Events.Subscribe(ctx, topic)
}
{% endif %}

{% if EmitDataloader %}
func (r *Resolver) loader(ctx context.Context) *dataloader.Loader {
	if r.Loaders != nil {
		return r.Loaders
	}
	if ctx == nil {
		return nil
	}
	loader, _ := dataloader.FromContext(ctx)
	return loader
}
{% endif %}

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
		relation := findOriginalNameForJoinKey(entity, ob.Field)
		relType := field.RelationType

		criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
			var col string
			q, col = applyRelation(q, relation, relType, field, column)
			if col == "" {
				col = column
			}
			return q.OrderExpr("? ?", bun.Safe(col), bun.Safe(dir))
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
		relation := findOriginalNameForJoinKey(entity, filter.Field)
		relType := field.RelationType

		switch op {
		case "IN", "NOT IN":
			values := splitList(value)
			if len(values) == 0 {
				continue
			}
				criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
					var col string
					q, col = applyRelation(q, relation, relType, field, column)
					if col == "" {
						col = column
					}
					expr := fmt.Sprintf("%s IN (?)", col)
					if op == "NOT IN" {
						expr = fmt.Sprintf("%s NOT IN (?)", col)
					}
					return q.Where(expr, bun.In(values))
				})
		default:
			criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
				var col string
				q, col = applyRelation(q, relation, relType, field, column)
				if col == "" {
					col = column
				}
				return q.Where(fmt.Sprintf("%s %s ?", col, op), value)
			})
		}
	}

	return criteria
}

func applyRelation(q *bun.SelectQuery, relation, relType string, field criteriaField, column string) (*bun.SelectQuery, string) {
	rel := strings.ToLower(strings.ReplaceAll(relType, "-", ""))
	rel = strings.ReplaceAll(rel, "_", "")
	if (rel == "manytomany" || rel == "m2m") && field.PivotTable != "" && field.SourcePivot != "" && field.TargetPivot != "" && field.TargetTable != "" {
		targetAlias := strings.ToLower(strings.ReplaceAll(field.Relation, ".", "_"))
		pivotAlias := fmt.Sprintf("%s_pivot", targetAlias)
		q = q.Join(fmt.Sprintf("JOIN %s AS %s ON %s.%s = ?TableAlias.%s", field.PivotTable, pivotAlias, pivotAlias, field.SourcePivot, field.SourceColumn))
		q = q.Join(fmt.Sprintf("JOIN %s AS %s ON %s.%s = %s.%s", field.TargetTable, targetAlias, targetAlias, field.TargetColumn, pivotAlias, field.TargetPivot))
		if strings.Contains(column, ".") {
			parts := strings.SplitN(column, ".", 2)
			column = targetAlias + "." + parts[1]
		} else {
			column = targetAlias + "." + column
		}
		return q, column
	}
	if relation != "" {
		q = q.Relation(relation)
	}
	return q, column
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

func valueString(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case *string:
		if val == nil {
			return ""
		}
		return *val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprint(val)
	}
}

{% for entity in ResolverEntities %}
func (r *Resolver) {{ entity.Name }}Service() crud.Service[model.{{ entity.Name }}] {
	if r.{{ entity.Name }}Svc == nil {
		panic("{{ entity.Name }}Service is not configured on Resolver")
	}
	return r.{{ entity.Name }}Svc
}

func (r *Resolver) Get{{ entity.Name }}(ctx context.Context, id string) (*model.{{ entity.Name }}, error) {
{% if entity.Hooks.Get.AuthGuard %}	{{ entity.Hooks.Get.AuthGuard | safe }}
{% endif %}	if err := r.guard(ctx, "{{ entity.Name }}", "show"); err != nil {
		return nil, err
	}
{% if entity.Hooks.Get.ScopeGuard %}	if err := {{ entity.Hooks.Get.ScopeGuard | safe }}; err != nil {
		return nil, err
	}
{% endif %}{% if entity.Hooks.Get.Preload %}	{{ entity.Hooks.Get.Preload | safe }}
{% endif %}	svc := r.{{ entity.Name }}Service()
{% if entity.Hooks.Get.WrapRepo %}	{{ entity.Hooks.Get.WrapRepo | safe }}
{% endif %}	record, err := svc.Show(r.crudContext(ctx), id, nil)
{% if entity.Hooks.Get.ErrorHandler %}	if err != nil {
		err = {{ entity.Hooks.Get.ErrorHandler | safe }}
	}
{% endif %}	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Resolver) List{{ entity.Name }}(ctx context.Context, pagination *model.PaginationInput, orderBy []*model.OrderByInput, filter []*model.FilterInput) (*model.{{ entity.Name }}Connection, error) {
{% if entity.Hooks.List.AuthGuard %}	{{ entity.Hooks.List.AuthGuard | safe }}
{% endif %}	if err := r.guard(ctx, "{{ entity.Name }}", "index"); err != nil {
		return nil, err
	}
{% if entity.Hooks.List.ScopeGuard %}	if err := {{ entity.Hooks.List.ScopeGuard | safe }}; err != nil {
		return nil, err
	}
{% endif %}	criteria := buildCriteria("{{ entity.Name }}", pagination, orderBy, filter)
{% if entity.Hooks.List.Preload %}	{{ entity.Hooks.List.Preload | safe }}
{% endif %}	svc := r.{{ entity.Name }}Service()
{% if entity.Hooks.List.WrapRepo %}	{{ entity.Hooks.List.WrapRepo | safe }}
{% endif %}	records, total, err := svc.Index(r.crudContext(ctx), criteria)
{% if entity.Hooks.List.ErrorHandler %}	if err != nil {
		err = {{ entity.Hooks.List.ErrorHandler | safe }}
	}
{% endif %}	if err != nil {
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
{% if entity.Hooks.Create.AuthGuard %}	{{ entity.Hooks.Create.AuthGuard | safe }}
{% endif %}	if err := r.guard(ctx, "{{ entity.Name }}", "create"); err != nil {
		return nil, err
	}
{% if entity.Hooks.Create.ScopeGuard %}	if err := {{ entity.Hooks.Create.ScopeGuard | safe }}; err != nil {
		return nil, err
	}
{% endif %}{% if entity.Hooks.Create.Preload %}	{{ entity.Hooks.Create.Preload | safe }}
{% endif %}	var record model.{{ entity.Name }}
	applyInput(&record, input)
	svc := r.{{ entity.Name }}Service()
{% if entity.Hooks.Create.WrapRepo %}	{{ entity.Hooks.Create.WrapRepo | safe }}
{% endif %}	record, err := svc.Create(r.crudContext(ctx), record)
{% if entity.Hooks.Create.ErrorHandler %}	if err != nil {
		err = {{ entity.Hooks.Create.ErrorHandler | safe }}
	}
{% endif %}	if err != nil {
		return nil, err
	}
{% if Subscriptions %}	if err := r.publishEvent(ctx, "{{ entity.Name }}", "created", record); err != nil {
		return nil, err
	}
{% endif %}	return &record, nil
}

func (r *Resolver) Update{{ entity.Name }}(ctx context.Context, id string, input model.Update{{ entity.Name }}Input) (*model.{{ entity.Name }}, error) {
{% if entity.Hooks.Update.AuthGuard %}	{{ entity.Hooks.Update.AuthGuard | safe }}
{% endif %}	if err := r.guard(ctx, "{{ entity.Name }}", "update"); err != nil {
		return nil, err
	}
{% if entity.Hooks.Update.ScopeGuard %}	if err := {{ entity.Hooks.Update.ScopeGuard | safe }}; err != nil {
		return nil, err
	}
{% endif %}{% if entity.Hooks.Update.Preload %}	{{ entity.Hooks.Update.Preload | safe }}
{% endif %}	svc := r.{{ entity.Name }}Service()
{% if entity.Hooks.Update.WrapRepo %}	{{ entity.Hooks.Update.WrapRepo | safe }}
{% endif %}	record, err := svc.Show(r.crudContext(ctx), id, nil)
{% if entity.Hooks.Update.ErrorHandler %}	if err != nil {
		err = {{ entity.Hooks.Update.ErrorHandler | safe }}
	}
{% endif %}	if err != nil {
		return nil, err
	}
	setID(&record, id)
	applyInput(&record, input)
	record, err = svc.Update(r.crudContext(ctx), record)
{% if entity.Hooks.Update.ErrorHandler %}	if err != nil {
		err = {{ entity.Hooks.Update.ErrorHandler | safe }}
	}
{% endif %}	if err != nil {
		return nil, err
	}
{% if Subscriptions %}	if err := r.publishEvent(ctx, "{{ entity.Name }}", "updated", record); err != nil {
		return nil, err
	}
{% endif %}	return &record, nil
}

func (r *Resolver) Delete{{ entity.Name }}(ctx context.Context, id string) (bool, error) {
{% if entity.Hooks.Delete.AuthGuard %}	{{ entity.Hooks.Delete.AuthGuard | safe }}
{% endif %}	if err := r.guard(ctx, "{{ entity.Name }}", "delete"); err != nil {
		return false, err
	}
{% if entity.Hooks.Delete.ScopeGuard %}	if err := {{ entity.Hooks.Delete.ScopeGuard | safe }}; err != nil {
		return false, err
	}
{% endif %}{% if entity.Hooks.Delete.Preload %}	{{ entity.Hooks.Delete.Preload | safe }}
{% endif %}	svc := r.{{ entity.Name }}Service()
{% if entity.Hooks.Delete.WrapRepo %}	{{ entity.Hooks.Delete.WrapRepo | safe }}
{% endif %}	var record model.{{ entity.Name }}
	setID(&record, id)
{% if Subscriptions %}	var deleted *model.{{ entity.Name }}
	if hasSubscription("{{ entity.Name }}", "deleted") && r.Events != nil {
		current, err := svc.Show(r.crudContext(ctx), id, nil)
		if err == nil {
			deleted = &current
		}
	}
{% endif %}	if err := svc.Delete(r.crudContext(ctx), record); err != nil {
{% if entity.Hooks.Delete.ErrorHandler %}		err = {{ entity.Hooks.Delete.ErrorHandler | safe }}
{% endif %}		return false, err
	}
{% if Subscriptions %}	payload := record
	if deleted != nil {
		payload = *deleted
	}
	if err := r.publishEvent(ctx, "{{ entity.Name }}", "deleted", payload); err != nil {
		return false, err
	}
{% endif %}	return true, nil
}

{% endfor %}
{% if Subscriptions %}{% for sub in Subscriptions %}
func (r *Resolver) {{ sub.MethodName }}(ctx context.Context{% for arg in sub.Args %}, {{ arg.Name }} any{% endfor %}) (<-chan {% if sub.List %}[]*model.{{ sub.ReturnType }}{% else %}*model.{{ sub.ReturnType }}{% endif %}, error) {
	stream, err := r.subscribe(ctx, "{{ sub.Entity }}", "{{ sub.Event }}")
	if err != nil {
		return nil, err
	}

	out := make(chan {% if sub.List %}[]*model.{{ sub.ReturnType }}{% else %}*model.{{ sub.ReturnType }}{% endif %})
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-stream:
				if !ok {
					return
				}
				if msg.Err != nil {
					continue
				}
{% if sub.List %}				switch payload := msg.Payload.(type) {
				case []model.{{ sub.ReturnType }}:
					items := make([]*model.{{ sub.ReturnType }}, 0, len(payload))
					for i := range payload {
						items = append(items, &payload[i])
					}
					out <- items
				case []*model.{{ sub.ReturnType }}:
					out <- payload
				}
{% else %}				switch payload := msg.Payload.(type) {
				case *model.{{ sub.ReturnType }}:
					out <- payload
				case model.{{ sub.ReturnType }}:
					item := payload
					out <- &item
				}
{% endif %}			}
		}
	}()

	return out, nil
}
{% endfor %}{% endif %}
{% if EmitDataloader %}
{% for entity in DataloaderEntities %}{% for rel in entity.Relations %}
func (r *{{ entity.Resolver }}Resolver) {{ rel.FieldName }}(ctx context.Context, obj *model.{{ entity.Name }}) ({% if rel.IsList %}[]*model.{{ rel.Target }}{% else %}*model.{{ rel.Target }}{% endif %}, error) {
	if obj == nil {
		return {% if rel.IsList %}nil{% else %}nil{% endif %}, nil
	}
{% if rel.IsList %}	if obj.{{ rel.FieldName }} != nil && len(obj.{{ rel.FieldName }}) > 0 {
		return obj.{{ rel.FieldName }}, nil
	}
{% else %}	if obj.{{ rel.FieldName }} != nil {
		return obj.{{ rel.FieldName }}, nil
	}
{% endif %}	loader := r.loader(ctx)
	if loader == nil {
		return obj.{{ rel.FieldName }}, nil
	}
{% if rel.RelationType == "belongsTo" %}	key := valueString(obj.{{ rel.SourceFieldKey.FieldName }})
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}
	record, err := loader.{{ rel.Target }}ByID.Load(ctx, key)
	if err != nil {
		return nil, err
	}
	return record, nil
{% elif rel.RelationType == "hasOne" %}	key := valueString(obj.{{ entity.PK.FieldName }})
	if strings.TrimSpace(key) == "" {
		return obj.{{ rel.FieldName }}, nil
	}
	items, err := loader.{{ entity.Name }}{{ rel.FieldName }}.Load(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return items[0], nil
	}
	return nil, nil
{% else %}	key := valueString(obj.{{ entity.PK.FieldName }})
	if strings.TrimSpace(key) == "" {
		return obj.{{ rel.FieldName }}, nil
	}
	items, err := loader.{{ entity.Name }}{{ rel.FieldName }}.Load(ctx, key)
	if err != nil {
		return nil, err
	}
	return items, nil
{% endif %}
}

{% endfor %}{% endfor %}{% endif %}
