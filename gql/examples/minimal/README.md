# Minimal example

This folder holds a tiny `router.SchemaMetadata` payload and the generator output so you can see the expected layout without running gqlgen.

- Input: `metadata.json`
- Output snapshot: `output/gqlgen.yml` and `output/graph/...`

Regenerate (optional):
```
go run ./gql/cmd/graphqlgen --metadata-file ./gql/examples/minimal/metadata.json --out ./gql/examples/minimal/output/graph --config ./gql/examples/minimal/output/gqlgen.yml
```
