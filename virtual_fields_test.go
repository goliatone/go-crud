package crud

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type virtualModel struct {
	Metadata map[string]any `json:"metadata"`
	Author   *string        `bun:"-" json:"author" crud:"virtual:Metadata"`
	Active   bool           `bun:"-" json:"active" crud:"virtual:Metadata,allow_zero"`
}

func strPtr(v string) *string { return &v }

func TestVirtualFieldHandler_BeforeSave_PointerMovesValue(t *testing.T) {
	handler := NewVirtualFieldHandlerWithConfig[*virtualModel](VirtualFieldHandlerConfig{})
	model := &virtualModel{Author: strPtr("John")}

	err := handler.BeforeSave(HookContext{}, model)
	require.NoError(t, err)

	assert.Nil(t, model.Author)
	if assert.NotNil(t, model.Metadata) {
		assert.Equal(t, "John", model.Metadata["author"])
	}
}

func TestVirtualFieldHandler_AfterLoad_PreserveKeys(t *testing.T) {
	handler := NewVirtualFieldHandlerWithConfig[*virtualModel](VirtualFieldHandlerConfig{})
	model := &virtualModel{
		Metadata: map[string]any{
			"author":             "John",
			"post_referrer_code": "abc123",
		},
	}

	err := handler.AfterLoad(HookContext{}, model)
	require.NoError(t, err)

	require.NotNil(t, model.Author)
	assert.Equal(t, "John", *model.Author)
	assert.Equal(t, "abc123", model.Metadata["post_referrer_code"])
	assert.Equal(t, "John", model.Metadata["author"], "virtual key should be preserved by default")
}

func TestVirtualFieldHandler_AfterLoad_RemoveKeys(t *testing.T) {
	preserve := false
	handler := NewVirtualFieldHandlerWithConfig[*virtualModel](VirtualFieldHandlerConfig{
		PreserveVirtualKeys: &preserve,
	})
	model := &virtualModel{
		Metadata: map[string]any{
			"author": "Jane",
			"extra":  true,
		},
	}

	err := handler.AfterLoad(HookContext{}, model)
	require.NoError(t, err)

	require.NotNil(t, model.Author)
	assert.Equal(t, "Jane", *model.Author)
	assert.Equal(t, true, model.Metadata["extra"])
	_, exists := model.Metadata["author"]
	assert.False(t, exists, "virtual key should be stripped when preserve=false")
}

func TestVirtualFieldHandler_AfterLoad_CopyMetadata(t *testing.T) {
	preserve := false
	handler := NewVirtualFieldHandlerWithConfig[*virtualModel](VirtualFieldHandlerConfig{
		PreserveVirtualKeys: &preserve,
		CopyMetadata:        true,
	})

	meta := map[string]any{"author": "John"}
	model := &virtualModel{Metadata: meta}

	err := handler.AfterLoad(HookContext{}, model)
	require.NoError(t, err)

	assert.Equal(t, "John", *model.Author)
	assert.NotSame(t, &meta, &model.Metadata)
	_, stillThere := meta["author"]
	assert.True(t, stillThere, "original map should be untouched when CopyMetadata=true")
	_, removed := model.Metadata["author"]
	assert.False(t, removed, "copied metadata should remove virtual key when preserve=false")
}

func TestVirtualFieldHandler_BatchMutatesModels(t *testing.T) {
	handler := NewVirtualFieldHandlerWithConfig[virtualModel](VirtualFieldHandlerConfig{})
	models := []virtualModel{
		{Metadata: map[string]any{"author": "A"}},
		{Metadata: map[string]any{"author": "B"}},
	}

	err := handler.AfterLoadBatch(HookContext{}, models)
	require.NoError(t, err)

	require.NotNil(t, models[0].Author)
	require.NotNil(t, models[1].Author)
	assert.Equal(t, "A", *models[0].Author)
	assert.Equal(t, "B", *models[1].Author)
}

func TestVirtualFieldHandler_AllowZeroValue(t *testing.T) {
	handler := NewVirtualFieldHandlerWithConfig[*virtualModel](VirtualFieldHandlerConfig{})
	model := &virtualModel{Active: false, Metadata: map[string]any{}}

	err := handler.BeforeSave(HookContext{}, model)
	require.NoError(t, err)

	assert.Equal(t, false, model.Metadata["active"])
}

func TestMergeVirtualMaps_DeepMergeAndDelete(t *testing.T) {
	defs := []VirtualFieldDef{
		{
			FieldName:     "Metadata",
			JSONName:      "metadata",
			SourceField:   "Metadata",
			MergeStrategy: "deep",
		},
	}
	current := virtualModel{
		Metadata: map[string]any{
			"tags":  map[string]any{"a": true, "b": true},
			"field": "keep",
		},
	}
	incoming := virtualModel{
		Metadata: map[string]any{
			"tags":  map[string]any{"b": nil, "c": true},
			"field": nil,
		},
	}

	merged := mergeVirtualMaps(current, incoming, defs, MergePolicy{DeleteWithNull: true})

	assert.Equal(t, map[string]any{"a": true, "c": true}, merged.Metadata["tags"])
	_, ok := merged.Metadata["field"]
	assert.False(t, ok, "field should be deleted when null and DeleteWithNull=true")
}

func TestMergeVirtualMaps_ShallowMerge(t *testing.T) {
	defs := []VirtualFieldDef{
		{
			FieldName:     "Metadata",
			JSONName:      "metadata",
			SourceField:   "Metadata",
			MergeStrategy: "shallow",
		},
	}
	current := virtualModel{
		Metadata: map[string]any{"nested": map[string]any{"a": 1}},
	}
	incoming := virtualModel{
		Metadata: map[string]any{"nested": map[string]any{"b": 2}},
	}

	merged := mergeVirtualMaps(current, incoming, defs, MergePolicy{DeleteWithNull: true})

	assert.Equal(t, map[string]any{"b": 2}, merged.Metadata["nested"])
}
