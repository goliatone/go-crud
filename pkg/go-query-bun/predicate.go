package querybun

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ParsePredicateKey splits field__operator keys into field and operator parts.
func ParsePredicateKey(key string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(key), "__", 2)
	field := strings.TrimSpace(parts[0])
	operator := "eq"
	if len(parts) == 2 {
		if op := strings.ToLower(strings.TrimSpace(parts[1])); op != "" {
			operator = op
		}
	}
	return field, operator
}

// NormalizePredicates converts ListOptions filters or predicates into a
// normalized predicate list. Explicit Predicates take precedence over Filters.
func NormalizePredicates(opts ListOptions) []Predicate {
	predicates, _ := NormalizePredicatesWithUnsupported(opts)
	return predicates
}

// NormalizePredicatesWithUnsupported converts ListOptions filters or predicates
// into normalized predicates and reports values that cannot be represented.
func NormalizePredicatesWithUnsupported(opts ListOptions) ([]Predicate, []UnsupportedPredicate) {
	if len(opts.Predicates) > 0 {
		out := make([]Predicate, 0, len(opts.Predicates))
		unsupported := make([]UnsupportedPredicate, 0)
		for _, predicate := range opts.Predicates {
			field := strings.TrimSpace(predicate.Field)
			if field == "" {
				unsupported = append(unsupported, UnsupportedPredicate{
					Field:    field,
					Operator: predicate.Operator,
					RawKey:   predicate.RawKey,
					RawValue: predicate.RawValue,
					Reason:   UnsupportedUnknownField,
				})
				continue
			}
			values, ok := NormalizeValueStrings(predicate.Values)
			if !ok {
				unsupported = append(unsupported, UnsupportedPredicate{
					Field:    field,
					Operator: predicate.Operator,
					RawKey:   predicate.RawKey,
					RawValue: predicate.RawValue,
					Reason:   UnsupportedValueShape,
				})
				continue
			}
			if len(values) == 0 {
				unsupported = append(unsupported, UnsupportedPredicate{
					Field:    field,
					Operator: predicate.Operator,
					RawKey:   predicate.RawKey,
					RawValue: predicate.RawValue,
					Reason:   UnsupportedEmptyValue,
				})
				continue
			}
			operator := strings.ToLower(strings.TrimSpace(predicate.Operator))
			out = append(out, Predicate{
				Field:    field,
				Operator: operator,
				Values:   values,
				RawKey:   predicate.RawKey,
				RawValue: predicate.RawValue,
			})
		}
		return out, unsupported
	}

	if len(opts.Filters) == 0 {
		return nil, nil
	}

	out := make([]Predicate, 0, len(opts.Filters))
	unsupported := make([]UnsupportedPredicate, 0)
	for rawKey, rawValue := range opts.Filters {
		field, operator := ParsePredicateKey(rawKey)
		if field == "" {
			unsupported = append(unsupported, UnsupportedPredicate{
				Field:    field,
				Operator: operator,
				RawKey:   rawKey,
				RawValue: rawValue,
				Reason:   UnsupportedUnknownField,
			})
			continue
		}
		values, ok := NormalizeValueStrings(rawValue)
		if !ok {
			unsupported = append(unsupported, UnsupportedPredicate{
				Field:    field,
				Operator: operator,
				RawKey:   rawKey,
				RawValue: rawValue,
				Reason:   UnsupportedValueShape,
			})
			continue
		}
		if len(values) == 0 {
			unsupported = append(unsupported, UnsupportedPredicate{
				Field:    field,
				Operator: operator,
				RawKey:   rawKey,
				RawValue: rawValue,
				Reason:   UnsupportedEmptyValue,
			})
			continue
		}
		out = append(out, Predicate{
			Field:    field,
			Operator: operator,
			Values:   values,
			RawKey:   rawKey,
			RawValue: rawValue,
		})
	}
	return out, unsupported
}

// NormalizeValueStrings converts supported scalar and slice values into
// trimmed, comma-split strings.
func NormalizeValueStrings(raw any) ([]string, bool) {
	switch typed := raw.(type) {
	case nil:
		return nil, true
	case string:
		return normalizeStringValues([]string{typed}), true
	case []string:
		return normalizeStringValues(typed), true
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			values, ok := NormalizeValueStrings(value)
			if !ok {
				return nil, false
			}
			out = append(out, values...)
		}
		return out, true
	case encoding.TextMarshaler:
		text, err := typed.MarshalText()
		if err != nil {
			return nil, false
		}
		return normalizeStringValues([]string{string(text)}), true
	case fmt.Stringer:
		return normalizeStringValues([]string{typed.String()}), true
	case int:
		return normalizeStringValues([]string{strconv.Itoa(typed)}), true
	case int8, int16, int32, int64:
		return normalizeStringValues([]string{fmt.Sprintf("%d", typed)}), true
	case uint, uint8, uint16, uint32, uint64:
		return normalizeStringValues([]string{fmt.Sprintf("%d", typed)}), true
	case float32:
		return normalizeStringValues([]string{strconv.FormatFloat(float64(typed), 'f', -1, 32)}), true
	case float64:
		return normalizeStringValues([]string{strconv.FormatFloat(typed, 'f', -1, 64)}), true
	case bool:
		return normalizeStringValues([]string{strconv.FormatBool(typed)}), true
	}

	value := reflect.ValueOf(raw)
	if !value.IsValid() {
		return nil, true
	}
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]string, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			values, ok := NormalizeValueStrings(value.Index(i).Interface())
			if !ok {
				return nil, false
			}
			out = append(out, values...)
		}
		return out, true
	case reflect.Map, reflect.Struct, reflect.Func, reflect.Chan:
		return nil, false
	default:
		return normalizeStringValues([]string{fmt.Sprint(raw)}), true
	}
}

func normalizeStringValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}
