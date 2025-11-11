package crud

import (
	"reflect"
	"sort"
	"strings"
)

// FieldMaskFunc receives the current field value and returns the masked version.
// Returning nil will zero the field when serialized.
type FieldMaskFunc func(value any) any

// FieldPolicy describes per-request column visibility rules returned by FieldPolicyProvider.
type FieldPolicy struct {
	// Name helps auditors identify which policy executed (e.g., "tenant-admin").
	Name string
	// Allow restricts responses to the listed JSON field names. Empty means inherit defaults.
	Allow []string
	// Deny removes the listed JSON field names from the response/queryable set.
	Deny []string
	// Mask applies field-level transformations before encoding the response.
	Mask map[string]FieldMaskFunc
	// RowFilter appends additional column filters after guard enforcement.
	RowFilter ScopeFilter
	// Labels stores arbitrary metadata surfaced in audit logs.
	Labels map[string]string
}

// FieldPolicyRequest conveys the context supplied to FieldPolicyProvider.
type FieldPolicyRequest[T any] struct {
	Context     Context
	Operation   CrudOperation
	Actor       ActorContext
	Scope       ScopeFilter
	Resource    string
	ResourceTyp reflect.Type
}

// FieldPolicyProvider returns the policy governing the given request.
type FieldPolicyProvider[T any] func(FieldPolicyRequest[T]) (FieldPolicy, error)

// FieldPolicyAudit captures the normalized details emitted to logs.
type FieldPolicyAudit struct {
	Policy    string
	Resource  string
	Operation CrudOperation
	Allowed   []string
	Denied    []string
	Masked    []string
	RowFilter []ScopeColumnFilter
	Labels    map[string]string
}

// LogFieldPolicyDecision writes the audit entry using the provided logger.
func LogFieldPolicyDecision(logger Logger, audit FieldPolicyAudit) {
	if logger == nil {
		return
	}

	fields := Fields{
		"resource":  audit.Resource,
		"operation": audit.Operation,
	}
	if audit.Policy != "" {
		fields["policy"] = audit.Policy
	}
	if len(audit.Allowed) > 0 {
		fields["allowed"] = strings.Join(audit.Allowed, ",")
	}
	if len(audit.Denied) > 0 {
		fields["denied"] = strings.Join(audit.Denied, ",")
	}
	if len(audit.Masked) > 0 {
		fields["masked"] = strings.Join(audit.Masked, ",")
	}
	if len(audit.RowFilter) > 0 {
		fields["row_filters"] = len(audit.RowFilter)
	}
	if len(audit.Labels) > 0 {
		fields["policy_labels"] = audit.Labels
	}

	if withFields, ok := logger.(loggerWithFields); ok {
		withFields.WithFields(fields).Info("field policy applied")
		return
	}
	logger.Info("field policy applied: %s", audit.Policy)
}

type resolvedFieldPolicy struct {
	allowedOverride map[string]string
	allowSet        map[string]struct{}
	denySet         map[string]struct{}
	maskers         map[string]FieldMaskFunc
	rowFilter       ScopeFilter
	audit           FieldPolicyAudit
}

func (r resolvedFieldPolicy) isZero() bool {
	return len(r.allowedOverride) == 0 &&
		len(r.allowSet) == 0 &&
		len(r.denySet) == 0 &&
		len(r.maskers) == 0 &&
		!r.rowFilter.HasFilters() &&
		len(r.audit.Masked) == 0 &&
		r.audit.Policy == ""
}

func (r resolvedFieldPolicy) allowedFields(base map[string]string) map[string]string {
	if len(r.allowedOverride) == 0 {
		return base
	}
	return r.allowedOverride
}

func (r resolvedFieldPolicy) allowedFieldOverride() map[string]string {
	return r.allowedOverride
}

func (r resolvedFieldPolicy) allowsField(field string) bool {
	key := normalizePolicyField(field)
	if len(r.allowSet) > 0 {
		_, ok := r.allowSet[key]
		return ok
	}
	if len(r.denySet) > 0 {
		_, denied := r.denySet[key]
		return !denied
	}
	return true
}

func (r resolvedFieldPolicy) maskFor(field string) FieldMaskFunc {
	if len(r.maskers) == 0 {
		return nil
	}
	return r.maskers[canonicalPolicyKey(field)]
}

func (r resolvedFieldPolicy) rowFilterCriteria() ScopeFilter {
	return r.rowFilter
}

func (r resolvedFieldPolicy) auditEntry() FieldPolicyAudit {
	return r.audit
}

func buildResolvedFieldPolicy[T any](policy FieldPolicy, base map[string]string, resource string, op CrudOperation) resolvedFieldPolicy {
	reverse := make(map[string]string, len(base))
	for field := range base {
		reverse[canonicalPolicyKey(field)] = field
	}

	allowSet := normalizePolicyList(policy.Allow, reverse)
	denySet := normalizePolicyList(policy.Deny, reverse)

	var override map[string]string
	if len(allowSet) > 0 || len(denySet) > 0 {
		override = make(map[string]string)
	}

	appliedAllowed := make([]string, 0, len(base))
	appliedDenied := make([]string, 0, len(base))

	for field, column := range base {
		key := canonicalPolicyKey(field)
		allowed := true
		if len(allowSet) > 0 {
			_, allowed = allowSet[key]
		}
		if allowed && len(denySet) > 0 {
			if _, denied := denySet[key]; denied {
				allowed = false
				appliedDenied = append(appliedDenied, field)
			}
		}
		if allowed {
			appliedAllowed = append(appliedAllowed, field)
			if override != nil {
				override[field] = column
			}
		} else if override == nil && len(denySet) == 0 {
			// no override map and no denies: nothing to copy
		}
	}

	maskers := normalizePolicyMask(policy.Mask, reverse)
	maskedFields := sortedKeys(maskers)

	rowFilter := policy.RowFilter.clone()
	rowFilter.Bypass = false

	audit := FieldPolicyAudit{
		Policy:    policy.Name,
		Resource:  resource,
		Operation: op,
		Allowed:   sortStrings(appliedAllowed),
		Denied:    sortStrings(appliedDenied),
		Masked:    maskedFields,
		RowFilter: rowFilter.ColumnFilters,
		Labels:    policy.Labels,
	}

	return resolvedFieldPolicy{
		allowedOverride: override,
		allowSet:        allowSet,
		denySet:         denySet,
		maskers:         maskers,
		rowFilter:       rowFilter,
		audit:           audit,
	}
}

func normalizePolicyList(values []string, reverse map[string]string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, raw := range values {
		key := canonicalPolicyKey(raw)
		if key == "" {
			continue
		}
		if _, ok := reverse[key]; ok {
			result[key] = struct{}{}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizePolicyMask(mask map[string]FieldMaskFunc, reverse map[string]string) map[string]FieldMaskFunc {
	if len(mask) == 0 {
		return nil
	}
	result := make(map[string]FieldMaskFunc, len(mask))
	for raw, fn := range mask {
		if fn == nil {
			continue
		}
		key := canonicalPolicyKey(raw)
		if key == "" {
			continue
		}
		if canonical, ok := reverse[key]; ok {
			result[canonical] = fn
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func canonicalPolicyKey(field string) string {
	return normalizePolicyField(field)
}

func normalizePolicyField(field string) string {
	return strings.TrimSpace(strings.ToLower(field))
}

func sortStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, len(values))
	copy(out, values)
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
