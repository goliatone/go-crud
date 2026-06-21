package querybun

import (
	"encoding"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// ParsePredicateKey splits field__operator keys into field and operator parts.
func ParsePredicateKey(key string) (string, OperatorKind) {
	return DefaultRESTSyntax().parsePredicateKey(key)
}

// NormalizePredicates converts ListOptions filters or predicates into a
// normalized predicate list. Explicit Predicates take precedence over Filters.
func NormalizePredicates(opts ListOptions, configs ...Config) []Predicate {
	predicates, _ := NormalizePredicatesWithUnsupported(opts, configs...)
	return predicates
}

// NormalizePredicatesWithUnsupported converts ListOptions filters or predicates
// into normalized predicates and reports values that cannot be represented.
func NormalizePredicatesWithUnsupported(opts ListOptions, configs ...Config) ([]Predicate, []UnsupportedPredicate) {
	cfg := DefaultConfig()
	if len(configs) > 0 {
		cfg = configs[0]
	}
	cfg = normalizeConfig(cfg)

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
			operator := cfg.Syntax.resolveOperatorAlias(string(predicate.Operator))
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
	keys := sortedFilterKeys(opts.Filters)
	for _, rawKey := range keys {
		rawValue := opts.Filters[rawKey]
		if _, ok := cfg.Syntax.resolveLogicalAlias(rawKey); ok {
			unsupported = append(unsupported, UnsupportedPredicate{
				Field:    strings.TrimSpace(rawKey),
				Operator: OperatorKind(normalizeOperatorToken(rawKey)),
				RawKey:   rawKey,
				RawValue: rawValue,
				Reason:   UnsupportedValueShape,
			})
			continue
		}
		field, operator := cfg.Syntax.parsePredicateKey(rawKey)
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

		if objectPredicates, objectUnsupported, ok := normalizeObjectPredicates(field, rawKey, rawValue, cfg); ok {
			out = append(out, objectPredicates...)
			unsupported = append(unsupported, objectUnsupported...)
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

func normalizeObjectPredicates(field, rawKey string, rawValue any, cfg Config) ([]Predicate, []UnsupportedPredicate, bool) {
	entries, ok := objectEntries(rawValue)
	if !ok {
		return nil, nil, false
	}
	if !looksLikeObjectPredicate(entries, cfg) {
		return nil, nil, false
	}
	predicates := make([]Predicate, 0, len(entries))
	unsupported := make([]UnsupportedPredicate, 0)
	for _, entry := range entries {
		operator := cfg.Syntax.resolveOperatorAlias(entry.key)
		if _, ok := cfg.Syntax.resolveLogicalAlias(entry.key); ok {
			unsupported = append(unsupported, UnsupportedPredicate{
				Field:    field,
				Operator: operator,
				RawKey:   rawKey + "." + entry.key,
				RawValue: entry.value,
				Reason:   UnsupportedValueShape,
			})
			continue
		}
		values, ok := NormalizeValueStrings(entry.value)
		if !ok {
			unsupported = append(unsupported, UnsupportedPredicate{
				Field:    field,
				Operator: operator,
				RawKey:   rawKey + "." + entry.key,
				RawValue: entry.value,
				Reason:   UnsupportedValueShape,
			})
			continue
		}
		if len(values) == 0 {
			unsupported = append(unsupported, UnsupportedPredicate{
				Field:    field,
				Operator: operator,
				RawKey:   rawKey + "." + entry.key,
				RawValue: entry.value,
				Reason:   UnsupportedEmptyValue,
			})
			continue
		}
		predicates = append(predicates, Predicate{
			Field:    field,
			Operator: operator,
			Values:   values,
			RawKey:   rawKey + "." + entry.key,
			RawValue: entry.value,
		})
	}
	return predicates, unsupported, true
}

func looksLikeObjectPredicate(entries []objectEntry, cfg Config) bool {
	for _, entry := range entries {
		key := normalizeOperatorToken(entry.key)
		if strings.HasPrefix(key, "$") {
			return true
		}
		if _, ok := cfg.Syntax.OperatorAliases[key]; ok {
			return true
		}
		if _, ok := cfg.Syntax.LogicalAliases[key]; ok {
			return true
		}
		if _, ok := cfg.Operators.Resolve(key); ok {
			return true
		}
	}
	return false
}

type objectEntry struct {
	key   string
	value any
}

func objectEntries(raw any) ([]objectEntry, bool) {
	if raw == nil {
		return nil, false
	}
	switch typed := raw.(type) {
	case map[string]any:
		entries := make([]objectEntry, 0, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			entries = append(entries, objectEntry{key: key, value: typed[key]})
		}
		return entries, true
	case map[string]string:
		entries := make([]objectEntry, 0, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			entries = append(entries, objectEntry{key: key, value: typed[key]})
		}
		return entries, true
	case map[OperatorKind]any:
		entries := make([]objectEntry, 0, len(typed))
		keys := make([]string, 0, len(typed))
		values := make(map[string]any, len(typed))
		for key, value := range typed {
			strKey := string(key)
			keys = append(keys, strKey)
			values[strKey] = value
		}
		sort.Strings(keys)
		for _, key := range keys {
			entries = append(entries, objectEntry{key: key, value: values[key]})
		}
		return entries, true
	}
	value := reflect.ValueOf(raw)
	if !value.IsValid() || value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	entries := make([]objectEntry, 0, value.Len())
	iter := value.MapRange()
	for iter.Next() {
		entries = append(entries, objectEntry{
			key:   iter.Key().String(),
			value: iter.Value().Interface(),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	return entries, true
}

func sortedFilterKeys(filters map[string]any) []string {
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
