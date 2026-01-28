# Minimal example

This folder holds a tiny `router.SchemaMetadata` payload and the generator output so you can see the expected layout without running gqlgen.

- Input: `metadata.json`
- Output snapshot: `output/gqlgen.yml` and `output/graph/...`
- Union support: if a property is modeled as `oneOf` in the OpenAPI projection, the generator emits a GraphQL `union`, adds a union model mapping in `gqlgen.yml`, and scaffolds the union interface in `models_gen.go`. Union discriminator overrides can be supplied via `x-gql.unionTypeMap`.

Regenerate (optional):
```
go run ./gql/cmd/graphqlgen --metadata-file ./gql/examples/minimal/metadata.json --out ./gql/examples/minimal/output/graph --config ./gql/examples/minimal/output/gqlgen.yml
```
