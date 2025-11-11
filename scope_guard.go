package crud

import (
	"slices"
	"strings"

	repository "github.com/goliatone/go-repository-bun"
)

// ActorContext captures normalized actor metadata attached to a request.
// It mirrors the payload emitted by go-auth middleware so guard adapters
// can run without importing that package.
type ActorContext struct {
	ActorID        string
	Subject        string
	Role           string
	ResourceRoles  map[string]string
	TenantID       string
	OrganizationID string
	Metadata       map[string]any
	ImpersonatorID string
	IsImpersonated bool
}

// Clone returns a shallow copy guarding internal maps from mutation.
func (a ActorContext) Clone() ActorContext {
	clone := a
	if len(a.ResourceRoles) > 0 {
		clone.ResourceRoles = make(map[string]string, len(a.ResourceRoles))
		for k, v := range a.ResourceRoles {
			clone.ResourceRoles[k] = v
		}
	}
	if len(a.Metadata) > 0 {
		clone.Metadata = make(map[string]any, len(a.Metadata))
		for k, v := range a.Metadata {
			clone.Metadata[k] = v
		}
	}
	return clone
}

// IsZero reports whether the actor context carries any meaningful identifier.
func (a ActorContext) IsZero() bool {
	return a.ActorID == "" &&
		a.Subject == "" &&
		a.Role == "" &&
		a.TenantID == "" &&
		a.OrganizationID == ""
}

// ScopeColumnFilter captures a single column restriction enforced by the guard.
type ScopeColumnFilter struct {
	Column   string
	Operator string
	Values   []string
}

// ScopeFilter describes the row-level restrictions returned by a guard.
// Controllers apply these filters before executing repository calls.
type ScopeFilter struct {
	// ColumnFilters expresses equality/IN restrictions (guard authoritative).
	ColumnFilters []ScopeColumnFilter
	// Bypass instructs controllers to skip automatic filter application.
	// Guard implementations must set this explicitly; it never defaults to true.
	Bypass bool
	// Labels provide optional structured metadata for logging/auditing.
	Labels map[string]string
	// Raw stores arbitrary data that higher layers may need for observability.
	Raw map[string]any
}

// AddColumnFilter appends a new filter, ignoring empty columns/values.
func (sf *ScopeFilter) AddColumnFilter(column string, operator string, values ...string) {
	column = strings.TrimSpace(column)
	if column == "" || len(values) == 0 {
		return
	}

	v := slices.DeleteFunc(slices.Clone(values), func(s string) bool {
		return strings.TrimSpace(s) == ""
	})
	if len(v) == 0 {
		return
	}

	op := strings.TrimSpace(operator)
	sf.ColumnFilters = append(sf.ColumnFilters, ScopeColumnFilter{
		Column:   column,
		Operator: op,
		Values:   v,
	})
}

// HasFilters reports whether the guard produced any column filters.
func (sf ScopeFilter) HasFilters() bool {
	return len(sf.ColumnFilters) > 0
}

func (sf ScopeFilter) clone() ScopeFilter {
	clone := sf
	if len(sf.ColumnFilters) > 0 {
		clone.ColumnFilters = make([]ScopeColumnFilter, len(sf.ColumnFilters))
		copy(clone.ColumnFilters, sf.ColumnFilters)
	}
	if len(sf.Labels) > 0 {
		clone.Labels = make(map[string]string, len(sf.Labels))
		for k, v := range sf.Labels {
			clone.Labels[k] = v
		}
	}
	if len(sf.Raw) > 0 {
		clone.Raw = make(map[string]any, len(sf.Raw))
		for k, v := range sf.Raw {
			clone.Raw[k] = v
		}
	}
	return clone
}

// selectCriteria converts guard filters into repository criteria.
func (sf ScopeFilter) selectCriteria() []repository.SelectCriteria {
	if sf.Bypass || len(sf.ColumnFilters) == 0 {
		return nil
	}

	out := make([]repository.SelectCriteria, 0, len(sf.ColumnFilters))
	for _, filter := range sf.ColumnFilters {
		if filter.Column == "" || len(filter.Values) == 0 {
			continue
		}

		op := strings.ToUpper(strings.TrimSpace(filter.Operator))
		switch {
		case op == "NOT IN":
			out = append(out, repository.SelectColumnNotIn(filter.Column, filter.Values))
		case op == "IN" || len(filter.Values) > 1:
			out = append(out, repository.SelectColumnIn(filter.Column, filter.Values))
		default:
			value := filter.Values[0]
			if value == "" {
				continue
			}
			if op == "" {
				op = "="
			}
			out = append(out, repository.SelectBy(filter.Column, op, value))
		}
	}
	return out
}

// ScopeGuardFunc resolves the actor + scope restrictions for a CRUD operation.
type ScopeGuardFunc[T any] func(ctx Context, op CrudOperation) (ActorContext, ScopeFilter, error)
