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
		return normalizeExplicitPredicates(opts.Predicates, cfg)
	}

	if len(opts.Filters) == 0 {
		return nil, nil
	}

	return normalizeFilterPredicates(opts.Filters, cfg)
}

func normalizeExplicitPredicates(predicates []Predicate, cfg Config) ([]Predicate, []UnsupportedPredicate) {
	out := make([]Predicate, 0, len(predicates))
	unsupported := make([]UnsupportedPredicate, 0)
	for _, predicate := range predicates {
		field := strings.TrimSpace(predicate.Field)
		values, reason, ok := normalizedPredicateValues(predicate.Values)
		if field == "" {
			unsupported = append(unsupported, unsupportedNormalizedPredicate(predicate, field, UnsupportedUnknownField))
			continue
		}
		if !ok {
			unsupported = append(unsupported, unsupportedNormalizedPredicate(predicate, field, reason))
			continue
		}
		out = append(out, Predicate{
			Field:    field,
			Operator: cfg.Syntax.resolveOperatorAlias(string(predicate.Operator)),
			Values:   values,
			RawKey:   predicate.RawKey,
			RawValue: predicate.RawValue,
		})
	}
	return out, unsupported
}

func normalizeFilterPredicates(filters map[string]any, cfg Config) ([]Predicate, []UnsupportedPredicate) {
	out := make([]Predicate, 0, len(filters))
	unsupported := make([]UnsupportedPredicate, 0)
	keys := sortedFilterKeys(filters)
	for _, rawKey := range keys {
		rawValue := filters[rawKey]
		if _, ok := cfg.Syntax.resolveLogicalAlias(rawKey); ok {
			unsupported = append(unsupported, unsupportedFilterPredicate(rawKey, rawKey, OperatorKind(normalizeOperatorToken(rawKey)), rawValue, UnsupportedValueShape))
			continue
		}
		field, operator := cfg.Syntax.parsePredicateKey(rawKey)
		if cfg.Syntax.isReservedKey(field) {
			continue
		}
		if field == "" {
			unsupported = append(unsupported, unsupportedFilterPredicate(field, rawKey, operator, rawValue, UnsupportedUnknownField))
			continue
		}

		if objectPredicates, objectUnsupported, ok := normalizeObjectPredicates(field, rawKey, rawValue, cfg); ok {
			out = append(out, objectPredicates...)
			unsupported = append(unsupported, objectUnsupported...)
			continue
		}

		values, reason, ok := normalizedPredicateValues(rawValue)
		if !ok {
			unsupported = append(unsupported, unsupportedFilterPredicate(field, rawKey, operator, rawValue, reason))
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

func normalizedPredicateValues(raw any) ([]string, UnsupportedReason, bool) {
	values, ok := NormalizeValueStrings(raw)
	if !ok {
		return nil, UnsupportedValueShape, false
	}
	if len(values) == 0 {
		return nil, UnsupportedEmptyValue, false
	}
	return values, "", true
}

func unsupportedNormalizedPredicate(predicate Predicate, field string, reason UnsupportedReason) UnsupportedPredicate {
	return UnsupportedPredicate{
		Field:    field,
		Operator: predicate.Operator,
		RawKey:   predicate.RawKey,
		RawValue: predicate.RawValue,
		Reason:   reason,
	}
}

func unsupportedFilterPredicate(field, rawKey string, operator OperatorKind, rawValue any, reason UnsupportedReason) UnsupportedPredicate {
	return UnsupportedPredicate{
		Field:    strings.TrimSpace(field),
		Operator: operator,
		RawKey:   rawKey,
		RawValue: rawValue,
		Reason:   reason,
	}
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
	}

	return normalizeReflectValueStrings(reflect.ValueOf(raw), raw)
}

func normalizeReflectValueStrings(value reflect.Value, raw any) ([]string, bool) {
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
	case reflect.String:
		return normalizeStringValues([]string{value.String()}), true
	case reflect.Bool:
		return normalizeStringValues([]string{strconv.FormatBool(value.Bool())}), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return normalizeStringValues([]string{strconv.FormatInt(value.Int(), 10)}), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return normalizeStringValues([]string{strconv.FormatUint(value.Uint(), 10)}), true
	case reflect.Float32:
		return normalizeStringValues([]string{strconv.FormatFloat(value.Float(), 'f', -1, 32)}), true
	case reflect.Float64:
		return normalizeStringValues([]string{strconv.FormatFloat(value.Float(), 'f', -1, 64)}), true
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
