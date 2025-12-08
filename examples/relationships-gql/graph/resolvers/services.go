package resolvers

import (
	"context"
	"fmt"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
	repository "github.com/goliatone/go-repository-bun"
)

func baseContext(ctx crud.Context) context.Context {
	if ctx == nil || ctx.UserContext() == nil {
		return context.Background()
	}
	return ctx.UserContext()
}

func appendRelations(relations []string, criteria []repository.SelectCriteria) []repository.SelectCriteria {
	out := make([]repository.SelectCriteria, 0, len(relations)+len(criteria))
	for _, rel := range relations {
		out = append(out, repository.SelectRelation(rel))
	}
	out = append(out, criteria...)
	return out
}

type publishingHouseService struct {
	repo      repository.Repository[*relationships.PublishingHouse]
	relations []string
}

func newPublishingHouseService(repo repository.Repository[*relationships.PublishingHouse]) crud.Service[model.PublishingHouse] {
	return &publishingHouseService{
		repo:      repo,
		relations: []string{"Headquarters", "Authors", "Books"},
	}
}

func (s *publishingHouseService) Create(ctx crud.Context, record model.PublishingHouse) (model.PublishingHouse, error) {
	domain, err := publishingHouseFromModel(record)
	if err != nil {
		return model.PublishingHouse{}, err
	}
	created, err := s.repo.Create(baseContext(ctx), domain)
	if err != nil {
		return model.PublishingHouse{}, err
	}
	return *toModelPublishingHouse(created, true), nil
}

func (s *publishingHouseService) CreateBatch(ctx crud.Context, records []model.PublishingHouse) ([]model.PublishingHouse, error) {
	domain := make([]*relationships.PublishingHouse, 0, len(records))
	for _, record := range records {
		item, err := publishingHouseFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	created, err := s.repo.CreateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.PublishingHouse, 0, len(created))
	for _, item := range created {
		result = append(result, *toModelPublishingHouse(item, true))
	}
	return result, nil
}

func (s *publishingHouseService) Update(ctx crud.Context, record model.PublishingHouse) (model.PublishingHouse, error) {
	domain, err := publishingHouseFromModel(record)
	if err != nil {
		return model.PublishingHouse{}, err
	}
	updated, err := s.repo.Update(baseContext(ctx), domain)
	if err != nil {
		return model.PublishingHouse{}, err
	}
	return *toModelPublishingHouse(updated, true), nil
}

func (s *publishingHouseService) UpdateBatch(ctx crud.Context, records []model.PublishingHouse) ([]model.PublishingHouse, error) {
	domain := make([]*relationships.PublishingHouse, 0, len(records))
	for _, record := range records {
		item, err := publishingHouseFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	updated, err := s.repo.UpdateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.PublishingHouse, 0, len(updated))
	for _, item := range updated {
		result = append(result, *toModelPublishingHouse(item, true))
	}
	return result, nil
}

func (s *publishingHouseService) Delete(ctx crud.Context, record model.PublishingHouse) error {
	domain, err := publishingHouseFromModel(record)
	if err != nil {
		return err
	}
	return s.repo.Delete(baseContext(ctx), domain)
}

func (s *publishingHouseService) DeleteBatch(ctx crud.Context, records []model.PublishingHouse) error {
	for _, record := range records {
		if err := s.Delete(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *publishingHouseService) Index(ctx crud.Context, criteria []repository.SelectCriteria) ([]model.PublishingHouse, int, error) {
	criteria = appendRelations(s.relations, criteria)
	records, total, err := s.repo.List(baseContext(ctx), criteria...)
	if err != nil {
		return nil, 0, err
	}
	return publishingHouseModels(records, true), total, nil
}

func (s *publishingHouseService) Show(ctx crud.Context, id string, criteria []repository.SelectCriteria) (model.PublishingHouse, error) {
	criteria = appendRelations(s.relations, criteria)
	record, err := s.repo.GetByID(baseContext(ctx), id, criteria...)
	if err != nil {
		return model.PublishingHouse{}, err
	}
	return *toModelPublishingHouse(record, true), nil
}

type headquartersService struct {
	repo      repository.Repository[*relationships.Headquarters]
	relations []string
}

func newHeadquartersService(repo repository.Repository[*relationships.Headquarters]) crud.Service[model.Headquarters] {
	return &headquartersService{
		repo:      repo,
		relations: []string{"Publisher"},
	}
}

func (s *headquartersService) Create(ctx crud.Context, record model.Headquarters) (model.Headquarters, error) {
	domain, err := headquartersFromModel(record)
	if err != nil {
		return model.Headquarters{}, err
	}
	created, err := s.repo.Create(baseContext(ctx), domain)
	if err != nil {
		return model.Headquarters{}, err
	}
	return *toModelHeadquarters(created, true), nil
}

func (s *headquartersService) CreateBatch(ctx crud.Context, records []model.Headquarters) ([]model.Headquarters, error) {
	domain := make([]*relationships.Headquarters, 0, len(records))
	for _, record := range records {
		item, err := headquartersFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	created, err := s.repo.CreateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Headquarters, 0, len(created))
	for _, item := range created {
		result = append(result, *toModelHeadquarters(item, true))
	}
	return result, nil
}

func (s *headquartersService) Update(ctx crud.Context, record model.Headquarters) (model.Headquarters, error) {
	domain, err := headquartersFromModel(record)
	if err != nil {
		return model.Headquarters{}, err
	}
	updated, err := s.repo.Update(baseContext(ctx), domain)
	if err != nil {
		return model.Headquarters{}, err
	}
	return *toModelHeadquarters(updated, true), nil
}

func (s *headquartersService) UpdateBatch(ctx crud.Context, records []model.Headquarters) ([]model.Headquarters, error) {
	domain := make([]*relationships.Headquarters, 0, len(records))
	for _, record := range records {
		item, err := headquartersFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	updated, err := s.repo.UpdateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Headquarters, 0, len(updated))
	for _, item := range updated {
		result = append(result, *toModelHeadquarters(item, true))
	}
	return result, nil
}

func (s *headquartersService) Delete(ctx crud.Context, record model.Headquarters) error {
	domain, err := headquartersFromModel(record)
	if err != nil {
		return err
	}
	return s.repo.Delete(baseContext(ctx), domain)
}

func (s *headquartersService) DeleteBatch(ctx crud.Context, records []model.Headquarters) error {
	for _, record := range records {
		if err := s.Delete(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *headquartersService) Index(ctx crud.Context, criteria []repository.SelectCriteria) ([]model.Headquarters, int, error) {
	criteria = appendRelations(s.relations, criteria)
	records, total, err := s.repo.List(baseContext(ctx), criteria...)
	if err != nil {
		return nil, 0, err
	}
	return headquartersModels(records, true), total, nil
}

func (s *headquartersService) Show(ctx crud.Context, id string, criteria []repository.SelectCriteria) (model.Headquarters, error) {
	criteria = appendRelations(s.relations, criteria)
	record, err := s.repo.GetByID(baseContext(ctx), id, criteria...)
	if err != nil {
		return model.Headquarters{}, err
	}
	return *toModelHeadquarters(record, true), nil
}

type authorService struct {
	repo      repository.Repository[*relationships.Author]
	relations []string
}

func newAuthorService(repo repository.Repository[*relationships.Author]) crud.Service[model.Author] {
	return &authorService{
		repo:      repo,
		relations: []string{"Publisher", "Profile", "Books", "Tags"},
	}
}

func (s *authorService) Create(ctx crud.Context, record model.Author) (model.Author, error) {
	domain, err := authorFromModel(record)
	if err != nil {
		return model.Author{}, err
	}
	created, err := s.repo.Create(baseContext(ctx), domain)
	if err != nil {
		return model.Author{}, err
	}
	return *toModelAuthor(created, true), nil
}

func (s *authorService) CreateBatch(ctx crud.Context, records []model.Author) ([]model.Author, error) {
	domain := make([]*relationships.Author, 0, len(records))
	for _, record := range records {
		item, err := authorFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	created, err := s.repo.CreateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Author, 0, len(created))
	for _, item := range created {
		result = append(result, *toModelAuthor(item, true))
	}
	return result, nil
}

func (s *authorService) Update(ctx crud.Context, record model.Author) (model.Author, error) {
	domain, err := authorFromModel(record)
	if err != nil {
		return model.Author{}, err
	}
	updated, err := s.repo.Update(baseContext(ctx), domain)
	if err != nil {
		return model.Author{}, err
	}
	return *toModelAuthor(updated, true), nil
}

func (s *authorService) UpdateBatch(ctx crud.Context, records []model.Author) ([]model.Author, error) {
	domain := make([]*relationships.Author, 0, len(records))
	for _, record := range records {
		item, err := authorFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	updated, err := s.repo.UpdateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Author, 0, len(updated))
	for _, item := range updated {
		result = append(result, *toModelAuthor(item, true))
	}
	return result, nil
}

func (s *authorService) Delete(ctx crud.Context, record model.Author) error {
	domain, err := authorFromModel(record)
	if err != nil {
		return err
	}
	return s.repo.Delete(baseContext(ctx), domain)
}

func (s *authorService) DeleteBatch(ctx crud.Context, records []model.Author) error {
	for _, record := range records {
		if err := s.Delete(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *authorService) Index(ctx crud.Context, criteria []repository.SelectCriteria) ([]model.Author, int, error) {
	criteria = appendRelations(s.relations, criteria)
	records, total, err := s.repo.List(baseContext(ctx), criteria...)
	if err != nil {
		return nil, 0, err
	}
	return authorModels(records, true), total, nil
}

func (s *authorService) Show(ctx crud.Context, id string, criteria []repository.SelectCriteria) (model.Author, error) {
	criteria = appendRelations(s.relations, criteria)
	record, err := s.repo.GetByID(baseContext(ctx), id, criteria...)
	if err != nil {
		return model.Author{}, err
	}
	return *toModelAuthor(record, true), nil
}

type authorProfileService struct {
	repo      repository.Repository[*relationships.AuthorProfile]
	relations []string
}

func newAuthorProfileService(repo repository.Repository[*relationships.AuthorProfile]) crud.Service[model.AuthorProfile] {
	return &authorProfileService{
		repo:      repo,
		relations: []string{"Author"},
	}
}

func (s *authorProfileService) Create(ctx crud.Context, record model.AuthorProfile) (model.AuthorProfile, error) {
	domain, err := authorProfileFromModel(record)
	if err != nil {
		return model.AuthorProfile{}, err
	}
	created, err := s.repo.Create(baseContext(ctx), domain)
	if err != nil {
		return model.AuthorProfile{}, err
	}
	return *toModelAuthorProfile(created, true), nil
}

func (s *authorProfileService) CreateBatch(ctx crud.Context, records []model.AuthorProfile) ([]model.AuthorProfile, error) {
	domain := make([]*relationships.AuthorProfile, 0, len(records))
	for _, record := range records {
		item, err := authorProfileFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	created, err := s.repo.CreateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.AuthorProfile, 0, len(created))
	for _, item := range created {
		result = append(result, *toModelAuthorProfile(item, true))
	}
	return result, nil
}

func (s *authorProfileService) Update(ctx crud.Context, record model.AuthorProfile) (model.AuthorProfile, error) {
	domain, err := authorProfileFromModel(record)
	if err != nil {
		return model.AuthorProfile{}, err
	}
	updated, err := s.repo.Update(baseContext(ctx), domain)
	if err != nil {
		return model.AuthorProfile{}, err
	}
	return *toModelAuthorProfile(updated, true), nil
}

func (s *authorProfileService) UpdateBatch(ctx crud.Context, records []model.AuthorProfile) ([]model.AuthorProfile, error) {
	domain := make([]*relationships.AuthorProfile, 0, len(records))
	for _, record := range records {
		item, err := authorProfileFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	updated, err := s.repo.UpdateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.AuthorProfile, 0, len(updated))
	for _, item := range updated {
		result = append(result, *toModelAuthorProfile(item, true))
	}
	return result, nil
}

func (s *authorProfileService) Delete(ctx crud.Context, record model.AuthorProfile) error {
	domain, err := authorProfileFromModel(record)
	if err != nil {
		return err
	}
	return s.repo.Delete(baseContext(ctx), domain)
}

func (s *authorProfileService) DeleteBatch(ctx crud.Context, records []model.AuthorProfile) error {
	for _, record := range records {
		if err := s.Delete(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *authorProfileService) Index(ctx crud.Context, criteria []repository.SelectCriteria) ([]model.AuthorProfile, int, error) {
	criteria = appendRelations(s.relations, criteria)
	records, total, err := s.repo.List(baseContext(ctx), criteria...)
	if err != nil {
		return nil, 0, err
	}
	return authorProfileModels(records, true), total, nil
}

func (s *authorProfileService) Show(ctx crud.Context, id string, criteria []repository.SelectCriteria) (model.AuthorProfile, error) {
	criteria = appendRelations(s.relations, criteria)
	record, err := s.repo.GetByID(baseContext(ctx), id, criteria...)
	if err != nil {
		return model.AuthorProfile{}, err
	}
	return *toModelAuthorProfile(record, true), nil
}

type bookService struct {
	repo      repository.Repository[*relationships.Book]
	relations []string
}

func newBookService(repo repository.Repository[*relationships.Book]) crud.Service[model.Book] {
	return &bookService{
		repo:      repo,
		relations: []string{"Publisher", "Author", "Chapters", "Tags"},
	}
}

func (s *bookService) Create(ctx crud.Context, record model.Book) (model.Book, error) {
	domain, err := bookFromModel(record)
	if err != nil {
		return model.Book{}, err
	}
	created, err := s.repo.Create(baseContext(ctx), domain)
	if err != nil {
		return model.Book{}, err
	}
	return *toModelBook(created, true), nil
}

func (s *bookService) CreateBatch(ctx crud.Context, records []model.Book) ([]model.Book, error) {
	domain := make([]*relationships.Book, 0, len(records))
	for _, record := range records {
		item, err := bookFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	created, err := s.repo.CreateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Book, 0, len(created))
	for _, item := range created {
		result = append(result, *toModelBook(item, true))
	}
	return result, nil
}

func (s *bookService) Update(ctx crud.Context, record model.Book) (model.Book, error) {
	domain, err := bookFromModel(record)
	if err != nil {
		return model.Book{}, err
	}
	updated, err := s.repo.Update(baseContext(ctx), domain)
	if err != nil {
		return model.Book{}, err
	}
	return *toModelBook(updated, true), nil
}

func (s *bookService) UpdateBatch(ctx crud.Context, records []model.Book) ([]model.Book, error) {
	domain := make([]*relationships.Book, 0, len(records))
	for _, record := range records {
		item, err := bookFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	updated, err := s.repo.UpdateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Book, 0, len(updated))
	for _, item := range updated {
		result = append(result, *toModelBook(item, true))
	}
	return result, nil
}

func (s *bookService) Delete(ctx crud.Context, record model.Book) error {
	domain, err := bookFromModel(record)
	if err != nil {
		return err
	}
	return s.repo.Delete(baseContext(ctx), domain)
}

func (s *bookService) DeleteBatch(ctx crud.Context, records []model.Book) error {
	for _, record := range records {
		if err := s.Delete(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *bookService) Index(ctx crud.Context, criteria []repository.SelectCriteria) ([]model.Book, int, error) {
	criteria = appendRelations(s.relations, criteria)
	records, total, err := s.repo.List(baseContext(ctx), criteria...)
	if err != nil {
		return nil, 0, err
	}
	return bookModels(records, true), total, nil
}

func (s *bookService) Show(ctx crud.Context, id string, criteria []repository.SelectCriteria) (model.Book, error) {
	criteria = appendRelations(s.relations, criteria)
	record, err := s.repo.GetByID(baseContext(ctx), id, criteria...)
	if err != nil {
		return model.Book{}, err
	}
	return *toModelBook(record, true), nil
}

type chapterService struct {
	repo      repository.Repository[*relationships.Chapter]
	relations []string
}

func newChapterService(repo repository.Repository[*relationships.Chapter]) crud.Service[model.Chapter] {
	return &chapterService{
		repo:      repo,
		relations: []string{"Book"},
	}
}

func (s *chapterService) Create(ctx crud.Context, record model.Chapter) (model.Chapter, error) {
	domain, err := chapterFromModel(record)
	if err != nil {
		return model.Chapter{}, err
	}
	created, err := s.repo.Create(baseContext(ctx), domain)
	if err != nil {
		return model.Chapter{}, err
	}
	return *toModelChapter(created, true), nil
}

func (s *chapterService) CreateBatch(ctx crud.Context, records []model.Chapter) ([]model.Chapter, error) {
	domain := make([]*relationships.Chapter, 0, len(records))
	for _, record := range records {
		item, err := chapterFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	created, err := s.repo.CreateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Chapter, 0, len(created))
	for _, item := range created {
		result = append(result, *toModelChapter(item, true))
	}
	return result, nil
}

func (s *chapterService) Update(ctx crud.Context, record model.Chapter) (model.Chapter, error) {
	domain, err := chapterFromModel(record)
	if err != nil {
		return model.Chapter{}, err
	}
	updated, err := s.repo.Update(baseContext(ctx), domain)
	if err != nil {
		return model.Chapter{}, err
	}
	return *toModelChapter(updated, true), nil
}

func (s *chapterService) UpdateBatch(ctx crud.Context, records []model.Chapter) ([]model.Chapter, error) {
	domain := make([]*relationships.Chapter, 0, len(records))
	for _, record := range records {
		item, err := chapterFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	updated, err := s.repo.UpdateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Chapter, 0, len(updated))
	for _, item := range updated {
		result = append(result, *toModelChapter(item, true))
	}
	return result, nil
}

func (s *chapterService) Delete(ctx crud.Context, record model.Chapter) error {
	domain, err := chapterFromModel(record)
	if err != nil {
		return err
	}
	return s.repo.Delete(baseContext(ctx), domain)
}

func (s *chapterService) DeleteBatch(ctx crud.Context, records []model.Chapter) error {
	for _, record := range records {
		if err := s.Delete(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *chapterService) Index(ctx crud.Context, criteria []repository.SelectCriteria) ([]model.Chapter, int, error) {
	criteria = appendRelations(s.relations, criteria)
	records, total, err := s.repo.List(baseContext(ctx), criteria...)
	if err != nil {
		return nil, 0, err
	}
	return chapterModels(records, true), total, nil
}

func (s *chapterService) Show(ctx crud.Context, id string, criteria []repository.SelectCriteria) (model.Chapter, error) {
	criteria = appendRelations(s.relations, criteria)
	record, err := s.repo.GetByID(baseContext(ctx), id, criteria...)
	if err != nil {
		return model.Chapter{}, err
	}
	return *toModelChapter(record, true), nil
}

type tagService struct {
	repo      repository.Repository[*relationships.Tag]
	relations []string
}

func newTagService(repo repository.Repository[*relationships.Tag]) crud.Service[model.Tag] {
	return &tagService{
		repo:      repo,
		relations: []string{"Books", "Authors"},
	}
}

func (s *tagService) Create(ctx crud.Context, record model.Tag) (model.Tag, error) {
	domain, err := tagFromModel(record)
	if err != nil {
		return model.Tag{}, err
	}
	created, err := s.repo.Create(baseContext(ctx), domain)
	if err != nil {
		return model.Tag{}, err
	}
	return *toModelTag(created, true), nil
}

func (s *tagService) CreateBatch(ctx crud.Context, records []model.Tag) ([]model.Tag, error) {
	domain := make([]*relationships.Tag, 0, len(records))
	for _, record := range records {
		item, err := tagFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	created, err := s.repo.CreateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Tag, 0, len(created))
	for _, item := range created {
		result = append(result, *toModelTag(item, true))
	}
	return result, nil
}

func (s *tagService) Update(ctx crud.Context, record model.Tag) (model.Tag, error) {
	domain, err := tagFromModel(record)
	if err != nil {
		return model.Tag{}, err
	}
	updated, err := s.repo.Update(baseContext(ctx), domain)
	if err != nil {
		return model.Tag{}, err
	}
	return *toModelTag(updated, true), nil
}

func (s *tagService) UpdateBatch(ctx crud.Context, records []model.Tag) ([]model.Tag, error) {
	domain := make([]*relationships.Tag, 0, len(records))
	for _, record := range records {
		item, err := tagFromModel(record)
		if err != nil {
			return nil, err
		}
		domain = append(domain, item)
	}
	updated, err := s.repo.UpdateMany(baseContext(ctx), domain)
	if err != nil {
		return nil, err
	}
	result := make([]model.Tag, 0, len(updated))
	for _, item := range updated {
		result = append(result, *toModelTag(item, true))
	}
	return result, nil
}

func (s *tagService) Delete(ctx crud.Context, record model.Tag) error {
	domain, err := tagFromModel(record)
	if err != nil {
		return err
	}
	return s.repo.Delete(baseContext(ctx), domain)
}

func (s *tagService) DeleteBatch(ctx crud.Context, records []model.Tag) error {
	for _, record := range records {
		if err := s.Delete(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *tagService) Index(ctx crud.Context, criteria []repository.SelectCriteria) ([]model.Tag, int, error) {
	criteria = appendRelations(s.relations, criteria)
	records, total, err := s.repo.List(baseContext(ctx), criteria...)
	if err != nil {
		return nil, 0, err
	}
	return tagModels(records, true), total, nil
}

func (s *tagService) Show(ctx crud.Context, id string, criteria []repository.SelectCriteria) (model.Tag, error) {
	criteria = appendRelations(s.relations, criteria)
	record, err := s.repo.GetByID(baseContext(ctx), id, criteria...)
	if err != nil {
		return model.Tag{}, err
	}
	return *toModelTag(record, true), nil
}

type services struct {
	publishingHouse crud.Service[model.PublishingHouse]
	headquarters    crud.Service[model.Headquarters]
	author          crud.Service[model.Author]
	authorProfile   crud.Service[model.AuthorProfile]
	book            crud.Service[model.Book]
	chapter         crud.Service[model.Chapter]
	tag             crud.Service[model.Tag]
}

func newServices(repos relationships.Repositories) services {
	return services{
		publishingHouse: newPublishingHouseService(repos.Publishers),
		headquarters:    newHeadquartersService(repos.Headquarters),
		author:          newAuthorService(repos.Authors),
		authorProfile:   newAuthorProfileService(repos.AuthorProfiles),
		book:            newBookService(repos.Books),
		chapter:         newChapterService(repos.Chapters),
		tag:             newTagService(repos.Tags),
	}
}

type graphqlCrudContext struct {
	ctx context.Context
}

func (g *graphqlCrudContext) UserContext() context.Context {
	if g.ctx == nil {
		return context.Background()
	}
	return g.ctx
}

func (g *graphqlCrudContext) Params(string, ...string) string { return "" }
func (g *graphqlCrudContext) BodyParser(out any) error        { return fmt.Errorf("not supported") }
func (g *graphqlCrudContext) Query(string, ...string) string  { return "" }
func (g *graphqlCrudContext) QueryInt(string, ...int) int     { return 0 }
func (g *graphqlCrudContext) Queries() map[string]string      { return map[string]string{} }
func (g *graphqlCrudContext) Body() []byte                    { return nil }
func (g *graphqlCrudContext) Status(int) crud.Response        { return g }
func (g *graphqlCrudContext) JSON(any, ...string) error       { return nil }
func (g *graphqlCrudContext) SendStatus(int) error            { return nil }

// NewCRUDContext adapts a standard context into a crud.Context for service calls.
func NewCRUDContext(ctx context.Context) crud.Context {
	return &graphqlCrudContext{ctx: ctx}
}
