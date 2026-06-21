package crud

import (
	"encoding/json"
	"reflect"
)

// mergeVirtualMaps merges map-backed virtual sources from incoming into current based on merge policy.
func mergeVirtualMaps[T any](current, incoming T, defs []VirtualFieldDef, policy MergePolicy) T {
	if len(defs) == 0 {
		return incoming
	}

	cv := mergeSourceValue(current)
	iv, isPtr, ok := mergeTargetValue(incoming)
	if !ok {
		return incoming
	}

	for _, def := range defs {
		curField := cv.FieldByName(def.SourceField)
		inField := iv.FieldByName(def.SourceField)
		if !curField.IsValid() || !inField.IsValid() {
			continue
		}
		if isNilValue(curField) && isNilValue(inField) {
			continue
		}
		strategy := mergeStrategyForField(def, policy)
		merged := mergeMap(curField, inField, strategy, policy.DeleteWithNull)
		inField.Set(merged)
	}

	if isPtr {
		outPtr := reflect.New(iv.Type())
		outPtr.Elem().Set(iv)
		if out, ok := valueAs[T](outPtr); ok {
			return out
		}
		return incoming
	}
	if out, ok := valueAs[T](iv); ok {
		return out
	}
	return incoming
}

func mergeSourceValue[T any](current T) reflect.Value {
	cv := reflect.ValueOf(current)
	if cv.Kind() == reflect.Pointer && !cv.IsNil() {
		return cv.Elem()
	}
	return cv
}

func mergeTargetValue[T any](incoming T) (reflect.Value, bool, bool) {
	iv := reflect.ValueOf(incoming)
	isPtr := iv.Kind() == reflect.Pointer
	if isPtr {
		if iv.IsNil() {
			return reflect.Value{}, true, false
		}
		iv = iv.Elem()
	}
	if !iv.CanSet() {
		copyVal := reflect.New(iv.Type()).Elem()
		copyVal.Set(iv)
		iv = copyVal
	}
	return iv, isPtr, true
}

func mergeStrategyForField(def VirtualFieldDef, policy MergePolicy) string {
	strategy := def.MergeStrategy
	if strategy == "" && policy.FieldMergeStrategy != nil {
		strategy = policy.FieldMergeStrategy[def.SourceField]
	}
	if strategy == "" {
		return "deep"
	}
	return strategy
}

func valueAs[T any](value reflect.Value) (T, bool) {
	var zero T
	if !value.IsValid() || !value.CanInterface() {
		return zero, false
	}
	out, ok := value.Interface().(T)
	return out, ok
}

func mergeMap(current, incoming reflect.Value, strategy string, deleteWithNull bool) reflect.Value {
	if current.IsNil() {
		return incoming
	}
	if incoming.IsNil() {
		return current
	}
	out := reflect.MakeMapWithSize(current.Type(), current.Len())
	iter := current.MapRange()
	for iter.Next() {
		out.SetMapIndex(iter.Key(), iter.Value())
	}

	iter = incoming.MapRange()
	for iter.Next() {
		key := iter.Key()
		val := iter.Value()
		if isJSONNull(val) && deleteWithNull {
			out.SetMapIndex(key, reflect.Value{})
			continue
		}
		if strategy == "shallow" {
			out.SetMapIndex(key, val)
			continue
		}
		// deep merge for map[string]any values
		out.SetMapIndex(key, deepMergeValue(out.MapIndex(key), val, deleteWithNull))
	}
	return out
}

func deepMergeValue(dst, src reflect.Value, deleteWithNull bool) reflect.Value {
	src, done := unwrapSourceValue(src, deleteWithNull)
	if done {
		return src
	}
	dst = unwrapDestinationValue(dst)

	if !src.IsValid() {
		return dst
	}
	if !dst.IsValid() || isNilValue(dst) {
		return src
	}
	if isJSONNull(src) && deleteWithNull {
		return reflect.Value{}
	}
	if src.Kind() == reflect.Map && dst.Kind() == reflect.Map {
		return deepMergeMapValue(dst, src, deleteWithNull)
	}

	return src
}

func unwrapSourceValue(src reflect.Value, deleteWithNull bool) (reflect.Value, bool) {
	if src.Kind() != reflect.Interface {
		return src, false
	}
	if src.IsNil() {
		if deleteWithNull {
			return reflect.Value{}, true
		}
		return src, true
	}
	return src.Elem(), false
}

func unwrapDestinationValue(dst reflect.Value) reflect.Value {
	if dst.Kind() != reflect.Interface {
		return dst
	}
	if dst.IsNil() {
		return reflect.Value{}
	}
	return dst.Elem()
}

func deepMergeMapValue(dst, src reflect.Value, deleteWithNull bool) reflect.Value {
	out := reflect.MakeMapWithSize(dst.Type(), dst.Len())
	iter := dst.MapRange()
	for iter.Next() {
		out.SetMapIndex(iter.Key(), iter.Value())
	}
	iter = src.MapRange()
	for iter.Next() {
		key := iter.Key()
		val := iter.Value()
		if isJSONNull(val) && deleteWithNull {
			out.SetMapIndex(key, reflect.Value{})
			continue
		}
		out.SetMapIndex(key, deepMergeValue(out.MapIndex(key), val, deleteWithNull))
	}
	return out
}

func isJSONNull(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer, reflect.Slice, reflect.Map:
		if v.IsNil() {
			return true
		}
	}
	if v.Kind() == reflect.Interface {
		return isJSONNull(v.Elem())
	}
	if v.Kind() == reflect.Map || v.Kind() == reflect.Slice {
		return false
	}

	raw, err := json.Marshal(v.Interface())
	return err == nil && string(raw) == "null"
}
