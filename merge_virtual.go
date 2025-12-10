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
	cv := reflect.ValueOf(current)
	if cv.Kind() == reflect.Ptr && !cv.IsNil() {
		cv = cv.Elem()
	}

	iv := reflect.ValueOf(incoming)
	isPtr := iv.Kind() == reflect.Ptr
	if isPtr {
		if iv.IsNil() {
			return incoming
		}
		iv = iv.Elem()
	}
	if !iv.CanSet() {
		copyVal := reflect.New(iv.Type()).Elem()
		copyVal.Set(iv)
		iv = copyVal
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
		strategy := def.MergeStrategy
		if strategy == "" && policy.FieldMergeStrategy != nil {
			if v, ok := policy.FieldMergeStrategy[def.SourceField]; ok {
				strategy = v
			}
		}
		if strategy == "" {
			strategy = "deep"
		}
		merged := mergeMap(curField, inField, strategy, policy.DeleteWithNull)
		inField.Set(merged)
	}

	if isPtr {
		outPtr := reflect.New(iv.Type())
		outPtr.Elem().Set(iv)
		return outPtr.Interface().(T)
	}
	return iv.Interface().(T)
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
	// Unwrap interfaces so we can inspect underlying maps.
	if src.Kind() == reflect.Interface {
		if src.IsNil() {
			if deleteWithNull {
				return reflect.Value{}
			}
			return src
		}
		src = src.Elem()
	}
	if dst.Kind() == reflect.Interface {
		if dst.IsNil() {
			dst = reflect.Value{}
		} else {
			dst = dst.Elem()
		}
	}

	if !dst.IsValid() || dst.IsNil() {
		return src
	}
	if !src.IsValid() {
		return dst
	}

	if isJSONNull(src) && deleteWithNull {
		return reflect.Value{}
	}

	// both maps?
	if src.Kind() == reflect.Map && dst.Kind() == reflect.Map {
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

	return src
}

func isJSONNull(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Ptr, reflect.Slice, reflect.Map:
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
