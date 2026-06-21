// Package querybun builds reusable Bun query plans from typed list options.
//
// The package is staged inside go-crud so predicate parsing and criteria
// planning can later move to github.com/goliatone/go-query-bun with a
// mechanical import path replacement.
//
// Planning is configured through Config, which groups query syntax, field
// resolution, semantic operators, search behavior, custom predicate builders,
// pagination defaults, and validation behavior. Use MapResolver for trusted
// field maps, SafeIdentifierResolver for strict dynamic identifiers, and
// OperatorSet for aliases or custom operator behavior.
//
// The default REST syntax supports field__operator keys, object-shaped operator
// maps, search aliases, and unsupported metadata. Unknown operators are never
// rendered as equality predicates.
package querybun
