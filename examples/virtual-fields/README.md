# Virtual Fields Example

This example shows how to expose virtual attributes as top-level API fields while persisting them into a JSON/JSONB map.

- Virtual fields are declared with `crud:"virtual:<mapField>"` and `bun:"-"`.
- The handler moves values into/out of the map automatically via lifecycle hooks.
- `PreserveVirtualKeys` defaults to `true` so responses keep both virtuals and raw metadata; set to `false` if you want the map to drop known virtual keys.
- Pointers (`*string`, `*bool`) let you distinguish “absent” from “explicit zero”; `allow_zero` tag opt-in exists for value fields.
- Merge semantics: `PUT` = replace, `PATCH` = merge (JSON Merge Patch) with delete-on-null; per-field `merge:deep|shallow` tag can override.

See `main.go` for a minimal setup with hooks and controller registration.
