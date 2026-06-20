package querybun

const (
	DefaultLimit  = 25
	DefaultOffset = 0
)

// ListOptions provides a non-HTTP contract for list query planning.
type ListOptions struct {
	Page       int
	PerPage    int
	Limit      int
	LimitSet   bool
	Offset     int
	OffsetSet  bool
	SortBy     string
	SortDesc   bool
	Order      string
	Search     string
	Filters    map[string]any
	Predicates []Predicate
	Select     []string
	Include    []string
}

// Config controls field resolution, operator aliases, validation, and defaults.
type Config struct {
	AllowedFields                map[string]string
	SearchColumns                []string
	OperatorMap                  map[string]string
	StrictValidation             bool
	StrictSearchColumns          bool
	StrictFields                 bool
	FallbackUnsupportedOperators bool
	DefaultLimit                 int
	DefaultOffset                int
}
