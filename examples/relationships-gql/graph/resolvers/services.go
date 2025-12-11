package resolvers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/goliatone/go-crud"
	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
	"github.com/goliatone/go-crud/pkg/activity"
	repository "github.com/goliatone/go-repository-bun"
)

type services struct {
	publishingHouse crud.Service[model.PublishingHouse]
	headquarters    crud.Service[model.Headquarters]
	author          crud.Service[model.Author]
	authorProfile   crud.Service[model.AuthorProfile]
	book            crud.Service[model.Book]
	chapter         crud.Service[model.Chapter]
	tag             crud.Service[model.Tag]

	domain domainServices
	inst   *instrumentation
}

type domainServices struct {
	publishingHouse crud.Service[*relationships.PublishingHouse]
	headquarters    crud.Service[*relationships.Headquarters]
	author          crud.Service[*relationships.Author]
	authorProfile   crud.Service[*relationships.AuthorProfile]
	book            crud.Service[*relationships.Book]
	chapter         crud.Service[*relationships.Chapter]
	tag             crud.Service[*relationships.Tag]
}

type servicePair[Dom any, GQL any] struct {
	gql    crud.Service[GQL]
	domain crud.Service[Dom]
}

type serviceDeps[Dom any, GQL any] struct {
	repo          repository.Repository[Dom]
	resource      string
	relations     []string
	toDomain      func(GQL) (Dom, error)
	toDomainBatch func([]GQL) ([]Dom, error)
	toModel       func(Dom) (GQL, error)
	toModelBatch  func([]Dom) ([]GQL, error)
	inst          *instrumentation
}

type instrumentation struct {
	virtualBefore map[string]int
	virtualAfter  map[string]int
	activities    []activity.Event
}

func newInstrumentation() *instrumentation {
	return &instrumentation{
		virtualBefore: make(map[string]int),
		virtualAfter:  make(map[string]int),
	}
}

func (i *instrumentation) bumpVirtual(resource string, before bool, count int) {
	if i == nil {
		return
	}
	if before {
		i.virtualBefore[resource] += count
		return
	}
	i.virtualAfter[resource] += count
}

func (i *instrumentation) recordActivity(event activity.Event) {
	if i == nil {
		return
	}
	i.activities = append(i.activities, event)
}

type trackingVirtuals[T any] struct {
	inst     *instrumentation
	resource string
}

func (t *trackingVirtuals[T]) BeforeSave(_ crud.HookContext, _ T) error {
	t.inst.bumpVirtual(t.resource, true, 1)
	return nil
}

func (t *trackingVirtuals[T]) AfterLoad(_ crud.HookContext, _ T) error {
	t.inst.bumpVirtual(t.resource, false, 1)
	return nil
}

func (t *trackingVirtuals[T]) AfterLoadBatch(_ crud.HookContext, records []T) error {
	t.inst.bumpVirtual(t.resource, false, len(records))
	return nil
}

func newServices(repos relationships.Repositories) services {
	inst := newInstrumentation()

	pub := makeService(serviceDeps[*relationships.PublishingHouse, model.PublishingHouse]{
		repo:      repos.Publishers,
		resource:  "publishing_house",
		relations: []string{"Headquarters", "Authors", "Books"},
		toDomain:  publishingHouseFromModel,
		toModel: func(src *relationships.PublishingHouse) (model.PublishingHouse, error) {
			if src == nil {
				return model.PublishingHouse{}, fmt.Errorf("nil publishing house")
			}
			if dst := toModelPublishingHouse(src, true); dst != nil {
				return *dst, nil
			}
			return model.PublishingHouse{}, fmt.Errorf("convert publishing house: empty model")
		},
		toModelBatch: func(src []*relationships.PublishingHouse) ([]model.PublishingHouse, error) {
			return publishingHouseModels(src, true), nil
		},
		inst: inst,
	})

	hq := makeService(serviceDeps[*relationships.Headquarters, model.Headquarters]{
		repo:      repos.Headquarters,
		resource:  "headquarters",
		relations: []string{"Publisher"},
		toDomain:  headquartersFromModel,
		toModel: func(src *relationships.Headquarters) (model.Headquarters, error) {
			if src == nil {
				return model.Headquarters{}, fmt.Errorf("nil headquarters")
			}
			if dst := toModelHeadquarters(src, true); dst != nil {
				return *dst, nil
			}
			return model.Headquarters{}, fmt.Errorf("convert headquarters: empty model")
		},
		toModelBatch: func(src []*relationships.Headquarters) ([]model.Headquarters, error) {
			return headquartersModels(src, true), nil
		},
		inst: inst,
	})

	author := makeService(serviceDeps[*relationships.Author, model.Author]{
		repo:      repos.Authors,
		resource:  "author",
		relations: []string{"Publisher", "Profile", "Books", "Tags"},
		toDomain:  authorFromModel,
		toModel: func(src *relationships.Author) (model.Author, error) {
			if src == nil {
				return model.Author{}, fmt.Errorf("nil author")
			}
			if dst := toModelAuthor(src, true); dst != nil {
				return *dst, nil
			}
			return model.Author{}, fmt.Errorf("convert author: empty model")
		},
		toModelBatch: func(src []*relationships.Author) ([]model.Author, error) {
			return authorModels(src, true), nil
		},
		inst: inst,
	})

	authorProfile := makeService(serviceDeps[*relationships.AuthorProfile, model.AuthorProfile]{
		repo:      repos.AuthorProfiles,
		resource:  "author_profile",
		relations: []string{"Author"},
		toDomain:  authorProfileFromModel,
		toModel: func(src *relationships.AuthorProfile) (model.AuthorProfile, error) {
			if src == nil {
				return model.AuthorProfile{}, fmt.Errorf("nil author profile")
			}
			if dst := toModelAuthorProfile(src, true); dst != nil {
				return *dst, nil
			}
			return model.AuthorProfile{}, fmt.Errorf("convert author profile: empty model")
		},
		toModelBatch: func(src []*relationships.AuthorProfile) ([]model.AuthorProfile, error) {
			return authorProfileModels(src, true), nil
		},
		inst: inst,
	})

	book := makeService(serviceDeps[*relationships.Book, model.Book]{
		repo:      repos.Books,
		resource:  "book",
		relations: []string{"Publisher", "Author", "Chapters", "Tags"},
		toDomain:  bookFromModel,
		toModel: func(src *relationships.Book) (model.Book, error) {
			if src == nil {
				return model.Book{}, fmt.Errorf("nil book")
			}
			if dst := toModelBook(src, true); dst != nil {
				return *dst, nil
			}
			return model.Book{}, fmt.Errorf("convert book: empty model")
		},
		toModelBatch: func(src []*relationships.Book) ([]model.Book, error) {
			return bookModels(src, true), nil
		},
		inst: inst,
	})

	chapter := makeService(serviceDeps[*relationships.Chapter, model.Chapter]{
		repo:      repos.Chapters,
		resource:  "chapter",
		relations: []string{"Book"},
		toDomain:  chapterFromModel,
		toModel: func(src *relationships.Chapter) (model.Chapter, error) {
			if src == nil {
				return model.Chapter{}, fmt.Errorf("nil chapter")
			}
			if dst := toModelChapter(src, true); dst != nil {
				return *dst, nil
			}
			return model.Chapter{}, fmt.Errorf("convert chapter: empty model")
		},
		toModelBatch: func(src []*relationships.Chapter) ([]model.Chapter, error) {
			return chapterModels(src, true), nil
		},
		inst: inst,
	})

	tag := makeService(serviceDeps[*relationships.Tag, model.Tag]{
		repo:      repos.Tags,
		resource:  "tag",
		relations: []string{"Books", "Authors"},
		toDomain:  tagFromModel,
		toModel: func(src *relationships.Tag) (model.Tag, error) {
			if src == nil {
				return model.Tag{}, fmt.Errorf("nil tag")
			}
			if dst := toModelTag(src, true); dst != nil {
				return *dst, nil
			}
			return model.Tag{}, fmt.Errorf("convert tag: empty model")
		},
		toModelBatch: func(src []*relationships.Tag) ([]model.Tag, error) {
			return tagModels(src, true), nil
		},
		inst: inst,
	})

	return services{
		publishingHouse: pub.gql,
		headquarters:    hq.gql,
		author:          author.gql,
		authorProfile:   authorProfile.gql,
		book:            book.gql,
		chapter:         chapter.gql,
		tag:             tag.gql,
		inst:            inst,
		domain: domainServices{
			publishingHouse: pub.domain,
			headquarters:    hq.domain,
			author:          author.domain,
			authorProfile:   authorProfile.domain,
			book:            book.domain,
			chapter:         chapter.domain,
			tag:             tag.domain,
		},
	}
}

func makeService[Dom any, GQL any](deps serviceDeps[Dom, GQL]) servicePair[Dom, GQL] {
	domain := crud.NewService(crud.ServiceConfig[Dom]{
		Repository:    deps.repo,
		VirtualFields: &trackingVirtuals[Dom]{inst: deps.inst, resource: deps.resource},
		ScopeGuard:    tenantScopeGuard[Dom](deps.resource),
		FieldPolicy:   defaultFieldPolicy[Dom](deps.resource),
		ActivityHooks: activity.Hooks{
			activity.HookFunc(func(ctx context.Context, event activity.Event) error {
				deps.inst.recordActivity(event)
				return nil
			}),
		},
		ActivityConfig: activity.Config{Enabled: true, Channel: "gql"},
		ResourceName:   deps.resource,
	})

	adapter := &serviceAdapter[GQL, Dom]{
		next:           domain,
		toDomain:       deps.toDomain,
		toDomainBatch:  deps.toDomainBatch,
		toModel:        deps.toModel,
		toModelBatch:   deps.toModelBatch,
		defaultInclude: deps.relations,
	}

	return servicePair[Dom, GQL]{
		gql:    adapter,
		domain: domain,
	}
}

func tenantScopeGuard[T any](resource string) crud.ScopeGuardFunc[T] {
	column := publisherColumn[T]()
	return func(ctx crud.Context, op crud.CrudOperation) (crud.ActorContext, crud.ScopeFilter, error) {
		actor := crud.ActorFromContext(ctx.UserContext())
		if actor.IsZero() {
			return crud.ActorContext{}, crud.ScopeFilter{}, errors.New("unauthorized")
		}
		scope := crud.ScopeFilter{}
		if column != "" && actor.TenantID != "" {
			scope.AddColumnFilter(column, "=", actor.TenantID)
		}
		_ = op
		if resource != "" {
			return actor, scope, nil
		}
		return actor, scope, nil
	}
}

func defaultFieldPolicy[T any](resource string) crud.FieldPolicyProvider[T] {
	return func(req crud.FieldPolicyRequest[T]) (crud.FieldPolicy, error) {
		policy := crud.FieldPolicy{
			Name:  strings.TrimSpace(resource + "-policy"),
			Allow: nil,
			Deny:  nil,
		}
		role := strings.ToLower(strings.TrimSpace(req.Actor.Role))
		if role == "guest" {
			policy.Deny = []string{"email", "pen_name"}
		}
		if role == "masked" {
			policy.Mask = map[string]crud.FieldMaskFunc{
				"email": func(any) any {
					return "***"
				},
			}
		}
		return policy, nil
	}
}

type serviceAdapter[GQL any, Dom any] struct {
	next           crud.Service[Dom]
	toDomain       func(GQL) (Dom, error)
	toDomainBatch  func([]GQL) ([]Dom, error)
	toModel        func(Dom) (GQL, error)
	toModelBatch   func([]Dom) ([]GQL, error)
	defaultInclude []string
}

func (s *serviceAdapter[GQL, Dom]) Create(ctx crud.Context, record GQL) (GQL, error) {
	domain, err := s.toDomain(record)
	if err != nil {
		var zero GQL
		return zero, err
	}
	created, err := s.next.Create(ctx, domain)
	if err != nil {
		var zero GQL
		return zero, err
	}
	return s.toModel(created)
}

func (s *serviceAdapter[GQL, Dom]) CreateBatch(ctx crud.Context, records []GQL) ([]GQL, error) {
	domainRecords, err := s.convertBatch(records)
	if err != nil {
		var zero []GQL
		return zero, err
	}
	created, err := s.next.CreateBatch(ctx, domainRecords)
	if err != nil {
		var zero []GQL
		return zero, err
	}
	return s.toModelSlice(created)
}

func (s *serviceAdapter[GQL, Dom]) Update(ctx crud.Context, record GQL) (GQL, error) {
	domain, err := s.toDomain(record)
	if err != nil {
		var zero GQL
		return zero, err
	}
	updated, err := s.next.Update(ctx, domain)
	if err != nil {
		var zero GQL
		return zero, err
	}
	return s.toModel(updated)
}

func (s *serviceAdapter[GQL, Dom]) UpdateBatch(ctx crud.Context, records []GQL) ([]GQL, error) {
	domainRecords, err := s.convertBatch(records)
	if err != nil {
		var zero []GQL
		return zero, err
	}
	updated, err := s.next.UpdateBatch(ctx, domainRecords)
	if err != nil {
		var zero []GQL
		return zero, err
	}
	return s.toModelSlice(updated)
}

func (s *serviceAdapter[GQL, Dom]) Delete(ctx crud.Context, record GQL) error {
	domain, err := s.toDomain(record)
	if err != nil {
		return err
	}
	return s.next.Delete(ctx, domain)
}

func (s *serviceAdapter[GQL, Dom]) DeleteBatch(ctx crud.Context, records []GQL) error {
	domainRecords, err := s.convertBatch(records)
	if err != nil {
		return err
	}
	return s.next.DeleteBatch(ctx, domainRecords)
}

func (s *serviceAdapter[GQL, Dom]) Index(ctx crud.Context, criteria []repository.SelectCriteria) ([]GQL, int, error) {
	criteria = withRelations(s.defaultInclude, criteria)
	records, total, err := s.next.Index(ctx, criteria)
	if err != nil {
		var zero []GQL
		return zero, total, err
	}
	out, err := s.toModelSlice(records)
	return out, total, err
}

func (s *serviceAdapter[GQL, Dom]) Show(ctx crud.Context, id string, criteria []repository.SelectCriteria) (GQL, error) {
	criteria = withRelations(s.defaultInclude, criteria)
	record, err := s.next.Show(ctx, id, criteria)
	if err != nil {
		var zero GQL
		return zero, err
	}
	return s.toModel(record)
}

func (s *serviceAdapter[GQL, Dom]) convertBatch(records []GQL) ([]Dom, error) {
	if s.toDomainBatch != nil {
		return s.toDomainBatch(records)
	}
	out := make([]Dom, 0, len(records))
	for _, record := range records {
		item, err := s.toDomain(record)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *serviceAdapter[GQL, Dom]) toModelSlice(records []Dom) ([]GQL, error) {
	if s.toModelBatch != nil {
		return s.toModelBatch(records)
	}
	out := make([]GQL, 0, len(records))
	for _, record := range records {
		item, err := s.toModel(record)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func withRelations(relations []string, criteria []repository.SelectCriteria) []repository.SelectCriteria {
	if len(relations) == 0 {
		return criteria
	}
	out := make([]repository.SelectCriteria, 0, len(criteria)+len(relations))
	for _, rel := range relations {
		out = append(out, repository.SelectRelation(rel))
	}
	return append(out, criteria...)
}

func publisherColumn[T any]() string {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return ""
	}
	field, ok := t.FieldByName("PublisherID")
	if !ok {
		return ""
	}
	tag := strings.TrimSpace(field.Tag.Get("bun"))
	if tag == "" {
		return "publisher_id"
	}
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return "publisher_id"
	}
	first := strings.TrimSpace(parts[0])
	if first == "" || first == "-" {
		return "publisher_id"
	}
	return first
}
