package crud

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

const (
	tagVirtualPrefix   = "virtual:"
	tagOptionMerge     = "merge"
	tagOptionAllowZero = "allow_zero"
)

// VirtualFieldHandlerConfig controls extraction/injection behavior.
type VirtualFieldHandlerConfig struct {
	PreserveVirtualKeys *bool  // keep virtual keys in the backing map after load (default: true)
	CopyMetadata        bool   // defensive copy of map before mutation
	ClearVirtualValues  *bool  // clear virtual field after BeforeSave (default: true)
	AllowZeroTag        string // tag option name to opt-in zero moves for value fields
	Dialect             string // virtual field SQL dialect (default: postgres)
}

// VirtualFieldDef describes a virtual field extracted from struct tags.
type VirtualFieldDef struct {
	FieldName     string
	JSONName      string
	SourceField   string
	FieldType     reflect.Type
	FieldIndex    int
	AllowZero     bool
	MergeStrategy string // e.g. deep|shallow|replace (used by service merge semantics)
}

// VirtualFieldHandler moves virtual fields into/out of a map field (e.g. metadata).
type VirtualFieldHandler[T any] struct {
	fieldDefs []VirtualFieldDef
	config    VirtualFieldHandlerConfig
	once      sync.Once
}

// NewVirtualFieldHandler builds a handler using defaults.
func NewVirtualFieldHandler[T any]() *VirtualFieldHandler[T] {
	return NewVirtualFieldHandlerWithConfig[T](VirtualFieldHandlerConfig{})
}

// NewVirtualFieldHandlerWithConfig builds a handler with custom config.
func NewVirtualFieldHandlerWithConfig[T any](cfg VirtualFieldHandlerConfig) *VirtualFieldHandler[T] {
	handler := &VirtualFieldHandler[T]{
		config: normalizeVirtualConfig(cfg),
	}
	handler.init()
	return handler
}

func normalizeVirtualConfig(cfg VirtualFieldHandlerConfig) VirtualFieldHandlerConfig {
	if cfg.AllowZeroTag == "" {
		cfg.AllowZeroTag = tagOptionAllowZero
	}
	if cfg.PreserveVirtualKeys == nil {
		defaultPreserve := true
		cfg.PreserveVirtualKeys = &defaultPreserve
	}
	if cfg.ClearVirtualValues == nil {
		defaultClear := true
		cfg.ClearVirtualValues = &defaultClear
	}
	if cfg.Dialect == "" {
		cfg.Dialect = VirtualDialectPostgres
	}
	return cfg
}

func (h *VirtualFieldHandler[T]) init() {
	h.once.Do(func() {
		h.fieldDefs = extractVirtualFieldDefs[T](h.config.AllowZeroTag)
	})
}

// FieldDefs returns a copy of detected virtual field definitions.
func (h *VirtualFieldHandler[T]) FieldDefs() []VirtualFieldDef {
	out := make([]VirtualFieldDef, len(h.fieldDefs))
	copy(out, h.fieldDefs)
	return out
}

// BeforeSave moves virtual values into the backing map and optionally clears the fields.
func (h *VirtualFieldHandler[T]) BeforeSave(ctx HookContext, model T) error {
	v, err := valueOfModel(model)
	if err != nil {
		return err
	}

	for _, def := range h.fieldDefs {
		fieldVal := v.Field(def.FieldIndex)
		if shouldSkipField(fieldVal, def.AllowZero) {
			continue
		}

		targetField := v.FieldByName(def.SourceField)
		if !targetField.IsValid() {
			return fmt.Errorf("virtual field %s: missing source field %s", def.FieldName, def.SourceField)
		}
		if err := ensureStringMap(targetField, def); err != nil {
			return err
		}
		if targetField.IsNil() {
			targetField.Set(reflect.MakeMap(targetField.Type()))
		}

		value := extractValue(fieldVal)
		targetField.SetMapIndex(reflect.ValueOf(def.JSONName), reflect.ValueOf(value))

		if h.config.ClearVirtualValues != nil && *h.config.ClearVirtualValues {
			fieldVal.Set(reflect.Zero(def.FieldType))
		}
	}
	return nil
}

// AfterLoad hydrates virtual fields from the backing map, preserving or removing keys based on config.
func (h *VirtualFieldHandler[T]) AfterLoad(ctx HookContext, model T) error {
	_, err := h.processAfterLoad(ctx, model)
	return err
}

// AfterLoadBatch hydrates virtuals for a slice of models.
func (h *VirtualFieldHandler[T]) AfterLoadBatch(ctx HookContext, models []T) error {
	for i := range models {
		updated, err := h.processAfterLoad(ctx, models[i])
		if err != nil {
			return err
		}
		models[i] = updated
	}
	return nil
}

func (h *VirtualFieldHandler[T]) processAfterLoad(_ HookContext, model T) (T, error) {
	v := reflect.ValueOf(model)

	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return model, nil
		}
		if err := h.applyAfterLoad(v.Elem()); err != nil {
			return model, err
		}
		return model, nil
	}

	// Make an addressable copy so we can mutate struct values.
	copyVal := reflect.New(v.Type()).Elem()
	copyVal.Set(v)
	if err := h.applyAfterLoad(copyVal); err != nil {
		return model, err
	}
	return copyVal.Interface().(T), nil
}

func (h *VirtualFieldHandler[T]) applyAfterLoad(v reflect.Value) error {
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("model must be a struct or pointer to struct")
	}

	for _, def := range h.fieldDefs {
		targetField := v.FieldByName(def.SourceField)
		if !targetField.IsValid() || isNilValue(targetField) {
			continue
		}
		if err := ensureStringMap(targetField, def); err != nil {
			return err
		}

		mapVal := targetField
		if mapVal.Kind() == reflect.Ptr {
			if mapVal.IsNil() {
				continue
			}
			mapVal = mapVal.Elem()
		}

		if h.config.CopyMetadata {
			copied := copyMap(mapVal)
			if targetField.Kind() == reflect.Ptr {
				ptr := reflect.New(mapVal.Type())
				ptr.Elem().Set(copied)
				targetField.Set(ptr)
				mapVal = ptr.Elem()
			} else {
				targetField.Set(copied)
				mapVal = targetField
			}
		}

		value := mapVal.MapIndex(reflect.ValueOf(def.JSONName))
		if !value.IsValid() {
			continue
		}

		fieldVal := v.Field(def.FieldIndex)
		if err := setFieldValue(fieldVal, value.Interface()); err != nil {
			return fmt.Errorf("failed to set virtual field %s: %w", def.FieldName, err)
		}

		if h.config.PreserveVirtualKeys != nil && !*h.config.PreserveVirtualKeys {
			mapVal.SetMapIndex(reflect.ValueOf(def.JSONName), reflect.Value{})
		}
	}
	return nil
}

func extractVirtualFieldDefs[T any](allowZeroTag string) []VirtualFieldDef {
	return extractVirtualFieldDefsForType(typeOf[T](), allowZeroTag)
}

func buildVirtualFieldDef(field reflect.StructField, index int, crudTag, allowZeroTag string) (VirtualFieldDef, bool) {
	jsonName := getJSONFieldName(field)
	if jsonName == "" || jsonName == "-" {
		return VirtualFieldDef{}, false
	}

	tagValue := strings.TrimPrefix(crudTag, tagVirtualPrefix)
	parts := strings.Split(tagValue, ",")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return VirtualFieldDef{}, false
	}
	sourceField := strings.TrimSpace(parts[0])

	def := VirtualFieldDef{
		FieldName:   field.Name,
		JSONName:    jsonName,
		SourceField: sourceField,
		FieldType:   field.Type,
		FieldIndex:  index,
	}

	for _, opt := range parts[1:] {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}
		if opt == allowZeroTag {
			def.AllowZero = true
			continue
		}
		if key, val, found := strings.Cut(opt, ":"); found {
			switch key {
			case tagOptionMerge:
				def.MergeStrategy = strings.ToLower(strings.TrimSpace(val))
			}
		}
	}

	return def, true
}

func hasStringMapField(t reflect.Type, name string) bool {
	sf, ok := t.FieldByName(name)
	if !ok {
		return false
	}
	ft := sf.Type
	if ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	if ft.Kind() != reflect.Map {
		return false
	}
	return ft.Key().Kind() == reflect.String
}

func shouldSkipField(v reflect.Value, allowZero bool) bool {
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return true
	}
	if allowZero {
		return false
	}
	return isZeroValue(v)
}

func extractValue(v reflect.Value) any {
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		return v.Elem().Interface()
	}
	return v.Interface()
}

func copyMap(v reflect.Value) reflect.Value {
	out := reflect.MakeMapWithSize(v.Type(), v.Len())
	iter := v.MapRange()
	for iter.Next() {
		out.SetMapIndex(iter.Key(), iter.Value())
	}
	return out
}

func ensureStringMap(v reflect.Value, def VirtualFieldDef) error {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Map || v.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("virtual field %s: source %s must be map[string]any", def.FieldName, def.SourceField)
	}
	return nil
}

func valueOfModel[T any](model T) (reflect.Value, error) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, fmt.Errorf("nil model")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("model must be a struct or pointer to struct")
	}
	return v, nil
}

func setFieldValue(field reflect.Value, value any) error {
	if !field.CanSet() {
		return fmt.Errorf("field cannot be set")
	}
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	targetType := field.Type()

	// Handle pointers by descending into Elem.
	if targetType.Kind() == reflect.Ptr {
		if val.Kind() == reflect.Ptr {
			if val.IsNil() {
				field.Set(reflect.Zero(targetType))
				return nil
			}
			val = val.Elem()
		}
		if field.IsNil() {
			field.Set(reflect.New(targetType.Elem()))
		}
		return setFieldValue(field.Elem(), val.Interface())
	}

	if val.Type().AssignableTo(targetType) {
		field.Set(val)
		return nil
	}

	if val.Type().ConvertibleTo(targetType) {
		field.Set(val.Convert(targetType))
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, field.Addr().Interface())
}

func isNilValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return v.IsNil()
	default:
		return false
	}
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}

func getJSONFieldName(field reflect.StructField) string {
	jsonTag := field.Tag.Get(TAG_JSON)
	if jsonTag == "" || jsonTag == "-" {
		return field.Name
	}
	parts := strings.Split(jsonTag, ",")
	return parts[0]
}
