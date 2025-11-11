package crud

import "strings"

// AdminScopeMetadata documents the required scope/claims for a resource.
type AdminScopeMetadata struct {
	Level       string   `json:"level,omitempty"`
	Description string   `json:"description,omitempty"`
	Claims      []string `json:"claims,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

func (m AdminScopeMetadata) toMap() map[string]any {
	ext := make(map[string]any)
	if level := strings.TrimSpace(m.Level); level != "" {
		ext["level"] = level
	}
	if desc := strings.TrimSpace(m.Description); desc != "" {
		ext["description"] = desc
	}
	if len(m.Claims) > 0 {
		ext["claims"] = cloneStringSlice(m.Claims)
	}
	if len(m.Roles) > 0 {
		ext["roles"] = cloneStringSlice(m.Roles)
	}
	if len(m.Labels) > 0 {
		ext["labels"] = cloneStringSlice(m.Labels)
	}
	if len(ext) == 0 {
		return nil
	}
	return ext
}

// AdminMenuMetadata hints how the CMS should organize the resource.
type AdminMenuMetadata struct {
	Group  string `json:"group,omitempty"`
	Label  string `json:"label,omitempty"`
	Icon   string `json:"icon,omitempty"`
	Order  int    `json:"order,omitempty"`
	Path   string `json:"path,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

func (m AdminMenuMetadata) toMap() map[string]any {
	ext := make(map[string]any)
	if group := strings.TrimSpace(m.Group); group != "" {
		ext["group"] = group
	}
	if label := strings.TrimSpace(m.Label); label != "" {
		ext["label"] = label
	}
	if icon := strings.TrimSpace(m.Icon); icon != "" {
		ext["icon"] = icon
	}
	if m.Order != 0 {
		ext["order"] = m.Order
	}
	if path := strings.TrimSpace(m.Path); path != "" {
		ext["path"] = path
	}
	if m.Hidden {
		ext["hidden"] = true
	}
	if len(ext) == 0 {
		return nil
	}
	return ext
}

// RowFilterHint documents guard/policy criteria applied to the resource.
type RowFilterHint struct {
	Field       string `json:"field"`
	Operator    string `json:"operator,omitempty"`
	Description string `json:"description,omitempty"`
}

func cloneRowFilterHints(hints []RowFilterHint) []RowFilterHint {
	if len(hints) == 0 {
		return nil
	}
	cloned := make([]RowFilterHint, len(hints))
	copy(cloned, hints)
	return cloned
}
