# Relationships example

GraphQL snapshot that mirrors the `../examples/relationships` REST demo (publishing houses, authors, books, chapters, tags). Use this to see how complex relations map into the generator output without running the full service.

- Input metadata: `metadata.json`
- Output snapshot: `output/gqlgen.yml` and `output/graph/...` (resolver Go files are tagged `//go:build ignore` so they won't compile during `go test ./...`).

Regenerate:
```
go run ./gql/cmd/graphqlgen \
  --metadata-file ./gql/examples/relationships/metadata.json \
  --out ./gql/examples/relationships/output/graph \
  --config ./gql/examples/relationships/output/gqlgen.yml
```
