package resolvers

import (
	"strings"
	"time"

	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
	"github.com/goliatone/go-crud/examples/relationships-gql/internal/data"
	"github.com/google/uuid"
)

func uuidString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func uuidFromString(raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(raw)
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func timeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func toModelPublishingHouse(src *data.PublishingHouse, withRelations bool) *model.PublishingHouse {
	if src == nil {
		return nil
	}
	dst := &model.PublishingHouse{
		Id:            uuidString(src.ID),
		Name:          src.Name,
		ImprintPrefix: src.ImprintPrefix,
		EstablishedAt: timePtr(src.EstablishedAt),
		CreatedAt:     timePtr(src.CreatedAt),
		UpdatedAt:     timePtr(src.UpdatedAt),
	}
	if withRelations {
		dst.Headquarters = toModelHeadquarters(src.Headquarters, false)
		dst.Authors = toModelAuthorSlice(src.Authors, false)
		dst.Books = toModelBookSlice(src.Books, false)
	}
	return dst
}

func toModelHeadquarters(src *data.Headquarters, withRelations bool) *model.Headquarters {
	if src == nil {
		return nil
	}
	dst := &model.Headquarters{
		Id:           uuidString(src.ID),
		PublisherId:  uuidString(src.PublisherID),
		AddressLine1: src.AddressLine1,
		AddressLine2: src.AddressLine2,
		City:         src.City,
		Country:      src.Country,
		OpenedAt:     timePtr(src.OpenedAt),
	}
	if withRelations {
		dst.Publisher = toModelPublishingHouse(src.Publisher, false)
	}
	return dst
}

func toModelAuthor(src *data.Author, withRelations bool) *model.Author {
	if src == nil {
		return nil
	}
	dst := &model.Author{
		Id:          uuidString(src.ID),
		PublisherId: uuidString(src.PublisherID),
		FullName:    src.FullName,
		PenName:     src.PenName,
		Email:       src.Email,
		Active:      src.Active,
		HiredAt:     timePtr(src.HiredAt),
		CreatedAt:   timePtr(src.CreatedAt),
		UpdatedAt:   timePtr(src.UpdatedAt),
	}
	if withRelations {
		dst.Publisher = toModelPublishingHouse(src.Publisher, false)
		dst.Profile = toModelAuthorProfile(src.Profile, false)
		dst.Books = toModelBookSlice(src.Books, false)
		dst.Tags = toModelTagSlice(src.Tags, false)
	}
	return dst
}

func toModelAuthorProfile(src *data.AuthorProfile, withRelations bool) *model.AuthorProfile {
	if src == nil {
		return nil
	}
	dst := &model.AuthorProfile{
		Id:            uuidString(src.ID),
		AuthorId:      uuidString(src.AuthorID),
		Biography:     src.Biography,
		FavoriteGenre: src.FavoriteGenre,
		WritingStyle:  src.WritingStyle,
	}
	if withRelations {
		dst.Author = toModelAuthor(src.Author, false)
	}
	return dst
}

func toModelBook(src *data.Book, withRelations bool) *model.Book {
	if src == nil {
		return nil
	}
	dst := &model.Book{
		Id:            uuidString(src.ID),
		PublisherId:   uuidString(src.PublisherID),
		AuthorId:      uuidString(src.AuthorID),
		Title:         src.Title,
		Isbn:          src.ISBN,
		Status:        src.Status,
		ReleaseDate:   timePtr(src.ReleaseDate),
		LastReprintAt: src.LastReprintAt,
		CreatedAt:     timePtr(src.CreatedAt),
		UpdatedAt:     timePtr(src.UpdatedAt),
	}
	if withRelations {
		dst.Publisher = toModelPublishingHouse(src.Publisher, false)
		dst.Author = toModelAuthor(src.Author, false)
		dst.Chapters = toModelChapterSlice(src.Chapters, false)
		dst.Tags = toModelTagSlice(src.Tags, false)
	}
	return dst
}

func toModelChapter(src *data.Chapter, withRelations bool) *model.Chapter {
	if src == nil {
		return nil
	}
	dst := &model.Chapter{
		Id:           uuidString(src.ID),
		BookId:       uuidString(src.BookID),
		Title:        src.Title,
		WordCount:    src.WordCount,
		ChapterIndex: src.ChapterIndex,
	}
	if withRelations {
		dst.Book = toModelBook(src.Book, false)
	}
	return dst
}

func toModelTag(src *data.Tag, withRelations bool) *model.Tag {
	if src == nil {
		return nil
	}
	dst := &model.Tag{
		Id:          uuidString(src.ID),
		Name:        src.Name,
		Category:    src.Category,
		Description: src.Description,
		CreatedAt:   timePtr(src.CreatedAt),
	}
	if withRelations {
		dst.Books = toModelBookSlice(src.Books, false)
		dst.Authors = toModelAuthorSlice(src.Authors, false)
	}
	return dst
}

func toModelPublishingHouseSlice(src []data.PublishingHouse, withRelations bool) []*model.PublishingHouse {
	result := make([]*model.PublishingHouse, 0, len(src))
	for i := range src {
		if item := toModelPublishingHouse(&src[i], withRelations); item != nil {
			result = append(result, item)
		}
	}
	return result
}

func toModelHeadquartersSlice(src []data.Headquarters, withRelations bool) []*model.Headquarters {
	result := make([]*model.Headquarters, 0, len(src))
	for i := range src {
		if item := toModelHeadquarters(&src[i], withRelations); item != nil {
			result = append(result, item)
		}
	}
	return result
}

func toModelAuthorSlice(src []data.Author, withRelations bool) []*model.Author {
	result := make([]*model.Author, 0, len(src))
	for i := range src {
		if item := toModelAuthor(&src[i], withRelations); item != nil {
			result = append(result, item)
		}
	}
	return result
}

func toModelAuthorProfileSlice(src []data.AuthorProfile, withRelations bool) []*model.AuthorProfile {
	result := make([]*model.AuthorProfile, 0, len(src))
	for i := range src {
		if item := toModelAuthorProfile(&src[i], withRelations); item != nil {
			result = append(result, item)
		}
	}
	return result
}

func toModelBookSlice(src []data.Book, withRelations bool) []*model.Book {
	result := make([]*model.Book, 0, len(src))
	for i := range src {
		if item := toModelBook(&src[i], withRelations); item != nil {
			result = append(result, item)
		}
	}
	return result
}

func toModelChapterSlice(src []data.Chapter, withRelations bool) []*model.Chapter {
	result := make([]*model.Chapter, 0, len(src))
	for i := range src {
		if item := toModelChapter(&src[i], withRelations); item != nil {
			result = append(result, item)
		}
	}
	return result
}

func toModelTagSlice(src []data.Tag, withRelations bool) []*model.Tag {
	result := make([]*model.Tag, 0, len(src))
	for i := range src {
		if item := toModelTag(&src[i], withRelations); item != nil {
			result = append(result, item)
		}
	}
	return result
}

func publishingHouseModels(src []*data.PublishingHouse, withRelations bool) []model.PublishingHouse {
	items := make([]model.PublishingHouse, 0, len(src))
	for _, record := range src {
		if item := toModelPublishingHouse(record, withRelations); item != nil {
			items = append(items, *item)
		}
	}
	return items
}

func headquartersModels(src []*data.Headquarters, withRelations bool) []model.Headquarters {
	items := make([]model.Headquarters, 0, len(src))
	for _, record := range src {
		if item := toModelHeadquarters(record, withRelations); item != nil {
			items = append(items, *item)
		}
	}
	return items
}

func authorModels(src []*data.Author, withRelations bool) []model.Author {
	items := make([]model.Author, 0, len(src))
	for _, record := range src {
		if item := toModelAuthor(record, withRelations); item != nil {
			items = append(items, *item)
		}
	}
	return items
}

func authorProfileModels(src []*data.AuthorProfile, withRelations bool) []model.AuthorProfile {
	items := make([]model.AuthorProfile, 0, len(src))
	for _, record := range src {
		if item := toModelAuthorProfile(record, withRelations); item != nil {
			items = append(items, *item)
		}
	}
	return items
}

func bookModels(src []*data.Book, withRelations bool) []model.Book {
	items := make([]model.Book, 0, len(src))
	for _, record := range src {
		if item := toModelBook(record, withRelations); item != nil {
			items = append(items, *item)
		}
	}
	return items
}

func chapterModels(src []*data.Chapter, withRelations bool) []model.Chapter {
	items := make([]model.Chapter, 0, len(src))
	for _, record := range src {
		if item := toModelChapter(record, withRelations); item != nil {
			items = append(items, *item)
		}
	}
	return items
}

func tagModels(src []*data.Tag, withRelations bool) []model.Tag {
	items := make([]model.Tag, 0, len(src))
	for _, record := range src {
		if item := toModelTag(record, withRelations); item != nil {
			items = append(items, *item)
		}
	}
	return items
}

func publishingHouseFromModel(src model.PublishingHouse) (*data.PublishingHouse, error) {
	id, err := uuidFromString(src.Id)
	if err != nil {
		return nil, err
	}
	return &data.PublishingHouse{
		ID:            id,
		Name:          src.Name,
		ImprintPrefix: src.ImprintPrefix,
		EstablishedAt: timeValue(src.EstablishedAt),
		CreatedAt:     timeValue(src.CreatedAt),
		UpdatedAt:     timeValue(src.UpdatedAt),
	}, nil
}

func headquartersFromModel(src model.Headquarters) (*data.Headquarters, error) {
	id, err := uuidFromString(src.Id)
	if err != nil {
		return nil, err
	}
	publisherID, err := uuidFromString(src.PublisherId)
	if err != nil {
		return nil, err
	}
	return &data.Headquarters{
		ID:           id,
		PublisherID:  publisherID,
		AddressLine1: src.AddressLine1,
		AddressLine2: src.AddressLine2,
		City:         src.City,
		Country:      src.Country,
		OpenedAt:     timeValue(src.OpenedAt),
	}, nil
}

func authorFromModel(src model.Author) (*data.Author, error) {
	id, err := uuidFromString(src.Id)
	if err != nil {
		return nil, err
	}
	publisherID, err := uuidFromString(src.PublisherId)
	if err != nil {
		return nil, err
	}
	return &data.Author{
		ID:          id,
		PublisherID: publisherID,
		FullName:    src.FullName,
		PenName:     src.PenName,
		Email:       src.Email,
		Active:      src.Active,
		HiredAt:     timeValue(src.HiredAt),
		CreatedAt:   timeValue(src.CreatedAt),
		UpdatedAt:   timeValue(src.UpdatedAt),
	}, nil
}

func authorProfileFromModel(src model.AuthorProfile) (*data.AuthorProfile, error) {
	id, err := uuidFromString(src.Id)
	if err != nil {
		return nil, err
	}
	authorID, err := uuidFromString(src.AuthorId)
	if err != nil {
		return nil, err
	}
	return &data.AuthorProfile{
		ID:            id,
		AuthorID:      authorID,
		Biography:     src.Biography,
		WritingStyle:  src.WritingStyle,
		FavoriteGenre: src.FavoriteGenre,
	}, nil
}

func bookFromModel(src model.Book) (*data.Book, error) {
	id, err := uuidFromString(src.Id)
	if err != nil {
		return nil, err
	}
	publisherID, err := uuidFromString(src.PublisherId)
	if err != nil {
		return nil, err
	}
	authorID, err := uuidFromString(src.AuthorId)
	if err != nil {
		return nil, err
	}
	return &data.Book{
		ID:            id,
		PublisherID:   publisherID,
		AuthorID:      authorID,
		Title:         src.Title,
		ISBN:          src.Isbn,
		Status:        src.Status,
		ReleaseDate:   timeValue(src.ReleaseDate),
		LastReprintAt: src.LastReprintAt,
		CreatedAt:     timeValue(src.CreatedAt),
		UpdatedAt:     timeValue(src.UpdatedAt),
	}, nil
}

func chapterFromModel(src model.Chapter) (*data.Chapter, error) {
	id, err := uuidFromString(src.Id)
	if err != nil {
		return nil, err
	}
	bookID, err := uuidFromString(src.BookId)
	if err != nil {
		return nil, err
	}
	return &data.Chapter{
		ID:           id,
		BookID:       bookID,
		Title:        src.Title,
		WordCount:    src.WordCount,
		ChapterIndex: src.ChapterIndex,
	}, nil
}

func tagFromModel(src model.Tag) (*data.Tag, error) {
	id, err := uuidFromString(src.Id)
	if err != nil {
		return nil, err
	}
	return &data.Tag{
		ID:          id,
		Name:        src.Name,
		Category:    src.Category,
		Description: src.Description,
		CreatedAt:   timeValue(src.CreatedAt),
	}, nil
}
