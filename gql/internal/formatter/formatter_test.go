package formatter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goliatone/go-router"
)

func TestFormat_BuildsEntitiesAndRelations(t *testing.T) {
	schemas := []router.SchemaMetadata{
		{
			Name:        "test-user",
			Description: "Test user schema",
			LabelField:  "name",
			Required:    []string{"id", "email"},
			Properties: map[string]router.PropertyInfo{
				"id":          {Type: "string", Format: "uuid"},
				"email":       {Type: "string"},
				"created_at":  {Type: "string", Format: "date-time"},
				"profile":     {Type: "object", Description: "profile info", RelatedSchema: "profile"},
				"tags":        {Type: "array", Items: &router.PropertyInfo{Type: "string"}},
				"internal_id": {Type: "string", ReadOnly: true},
			},
			Relationships: map[string]*router.RelationshipInfo{
				"profile": {RelatedSchema: "profile", RelationType: "hasOne", Cardinality: "one", SourceField: "profile_id"},
			},
		},
		{
			Name: "profile",
			Properties: map[string]router.PropertyInfo{
				"id":  {Type: "string", Format: "uuid"},
				"bio": {Type: "string"},
			},
		},
	}

	doc, err := Format(schemas)
	require.NoError(t, err)
	require.Len(t, doc.Entities, 2)

	assert.Equal(t, "Profile", doc.Entities[0].Name)
	user := doc.Entities[1]
	assert.Equal(t, "TestUser", user.Name)
	assert.Equal(t, "name", user.LabelField)

	require.Len(t, user.Fields, 6)
	assert.Equal(t, []string{"id", "createdAt", "email", "internalId", "profile", "tags"}, fieldNames(user.Fields))

	idField := user.Fields[0]
	assert.Equal(t, "UUID", idField.Type)
	assert.True(t, idField.Required)
	assert.Nil(t, idField.Relation)

	profileField := user.Fields[4]
	require.NotNil(t, profileField.Relation)
	assert.Equal(t, "Profile", profileField.Type)
	assert.Equal(t, "profile_id", profileField.Relation.SourceField)
	assert.False(t, profileField.Relation.IsList)

	tags := user.Fields[5]
	assert.True(t, tags.IsList)
	assert.Equal(t, "String", tags.Type)

	require.Len(t, user.Relationships, 1)
	assert.Equal(t, "profile", strings.ToLower(user.Relationships[0].Name))
	assert.Equal(t, "Profile", user.Relationships[0].Type)
}

func TestFormat_AppliesCustomNamingAndTypeOverrides(t *testing.T) {
	schema := router.SchemaMetadata{
		Name: "order_item",
		Properties: map[string]router.PropertyInfo{
			"id":          {Type: "string", Format: "uuid"},
			"created_at":  {Type: "string", Format: "date-time"},
			"amount":      {Type: "number", OriginalType: "decimal.Decimal"},
			"customer_id": {Type: "string"},
		},
		Relationships: map[string]*router.RelationshipInfo{
			"customer_id": {RelatedSchema: "customer", Cardinality: "many"},
		},
	}

	doc, err := Format([]router.SchemaMetadata{schema},
		WithFieldNamer(func(s string) string { return s }),
		WithTypeNamer(func(s string) string { return strings.ToUpper(s) }),
		WithPinnedFields("created_at", "id"),
		WithTypeMappings(map[TypeRef]string{
			{Type: "string", Format: "uuid"}:      "ID",
			{GoType: "decimal.decimal"}:           "Decimal",
			{Type: "string", Format: "date-time"}: "Timestamp",
		}))
	require.NoError(t, err)

	require.Len(t, doc.Entities, 1)
	entity := doc.Entities[0]
	assert.Equal(t, "ORDER_ITEM", entity.Name)

	require.Len(t, entity.Fields, 4)
	assert.Equal(t, "created_at", entity.Fields[0].OriginalName)
	assert.Equal(t, "Timestamp", entity.Fields[0].Type)

	assert.Equal(t, "ID", entity.Fields[1].Type)

	amount := entity.Fields[2]
	assert.Equal(t, "Decimal", amount.Type)
	assert.Nil(t, amount.Relation)

	customer := entity.Fields[3]
	require.NotNil(t, customer.Relation)
	assert.True(t, customer.Relation.IsList)
	assert.Equal(t, "CUSTOMER", customer.Relation.Type)
}

func TestFormat_HandlesNilMapsAndSorts(t *testing.T) {
	schemas := []router.SchemaMetadata{
		{Name: "zeta"},
		{
			Name: "alpha",
			Properties: map[string]router.PropertyInfo{
				"id": {Type: "string"},
			},
		},
	}

	doc, err := Format(schemas)
	require.NoError(t, err)
	require.Len(t, doc.Entities, 2)

	assert.Equal(t, "Alpha", doc.Entities[0].Name)
	assert.Equal(t, "Zeta", doc.Entities[1].Name)
	require.Len(t, doc.Entities[0].Fields, 1)
	assert.Equal(t, "String", doc.Entities[0].Fields[0].Type)
}

func TestFormat_BuildsUnionMetadata(t *testing.T) {
	schemas := []router.SchemaMetadata{
		{
			Name: "blog_post",
			Properties: map[string]router.PropertyInfo{
				"blocks": {
					Type:  "array",
					Items: &router.PropertyInfo{Type: "object"},
					CustomTagData: map[string]any{
						unionMembersKey: []string{"hero_block", "rich_text_block"},
						unionDiscriminatorKey: map[string]string{
							"hero":      "hero_block",
							"rich_text": "rich_text_block",
						},
						unionOverridesKey: map[string]string{
							"hero": "HeroBlockOverride",
						},
					},
				},
			},
		},
		{Name: "hero_block"},
		{Name: "rich_text_block"},
	}

	doc, err := Format(schemas)
	require.NoError(t, err)
	require.Len(t, doc.Unions, 1)

	union := doc.Unions[0]
	assert.Equal(t, "BlogPostBlock", union.Name)
	assert.Equal(t, []string{"HeroBlock", "RichTextBlock"}, union.Types)
	assert.Equal(t, "HeroBlockOverride", union.TypeMap["hero"])
	assert.Equal(t, "RichTextBlock", union.TypeMap["rich_text"])

	var blog Entity
	for _, ent := range doc.Entities {
		if ent.RawName == "blog_post" {
			blog = ent
			break
		}
	}
	require.NotEmpty(t, blog.Name)

	var blocks Field
	for _, field := range blog.Fields {
		if field.OriginalName == "blocks" {
			blocks = field
			break
		}
	}
	require.Equal(t, "BlogPostBlock", blocks.Type)
	assert.True(t, blocks.IsList)
}

func fieldNames(fields []Field) []string {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return names
}
