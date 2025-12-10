# relationships-gql example

This example mirrors the REST relationships demo but serves the data over GraphQL using gqlgen, go-crud services, and an in-memory sqlite/bun setup seeded via fixtures (hashid-generated IDs).

## Running

```
./taskfile graphqlgen   # regenerate schema + gqlgen.yml from live models via registrar
./taskfile gqlgen       # run gqlgen to refresh generated code
./taskfile serve        # starts the server on :9091
./taskfile gen          # go generate (uses the registrar + live models)
./taskfile metadata     # optional: emit metadata.json snapshot for fixtures/docs
```

GraphQL endpoint: `http://localhost:9091/graphql` (HTTP + WebSocket)  
Playground: `http://localhost:9091/playground`

`graphqlgen` runs with `--emit-subscriptions` and `--emit-dataloader`, so the generated schema includes Subscription fields and resolver field loaders by default (see `graph/resolvers/resolver_gen.go` and `graph/dataloader/dataloader_gen.go`).

All list operations return Relay connections (`edges { node cursor }` + `pageInfo`) so clients can page using cursors while still seeing total counts.

### Dataloaders
- Requests automatically wrap the gqlgen handler with a per-request dataloader (see `internal/loader/loader.go`). It batches relation lookups (including many-to-many pivots) and is discovered in resolvers via `dataloader.FromContext`.
- The generated loader lives in `graph/dataloader/dataloader_gen.go`; keep it in sync with `./taskfile graphqlgen --emit-dataloader`.
- Sanity check the batching/grouping logic with `go test ./graph/dataloader -run TestLoader_GroupsDeterministically`.

### Subscriptions (websocket)
- Websocket endpoint shares `/graphql` (`graphql-transport-ws`), configured in `cmd/server/main.go` via go-router’s Fiber adapter and the gqlgen websocket transport ported onto `router.WebSocketContext`.
- An in-memory event bus in `graph/resolvers/resolver_custom.go` drives `tagCreated/tagUpdated/tagDeleted` (and equivalents for every entity). Example subscription:
```graphql
subscription {
  tagCreated {
    id
    name
    category
  }
}
```
- Topics use `<entity>.<event>` keys (e.g., `tag.created`, `book.updated`); the websocket smoke test (`go test ./graph/resolvers -run WebSocketFlow`) asserts the same topics are used for subscribe/publish.
- Trigger events with mutations (e.g., `createTag`). A smoke test covers websocket upgrade + event flow: `go test ./graph/resolvers -run WebSocketFlow`.

## Hooks (optional auth/scope snippets)
- Inject an auth guard into generated resolvers:
```
go run ./gql/cmd/graphqlgen \
  --schema-package ./registrar \
  --out graph --config gqlgen.yml \
  --auth-guard "auth.FromContext(ctx)" \
  --auth-package github.com/goliatone/go-auth
```
- Or pass a hooks overlay (example):
```yaml
hooks:
  imports: [github.com/goliatone/go-auth]
  default:
    auth_guard: |
      user, ok := auth.FromContext(ctx)
      if !ok || user == nil { return nil, errors.New("unauthorized") }
```

## Auth tokens (demo)
- The server currently trusts any bearer token string; there is no signing/verification step.
- Send any token you like via `Authorization: Bearer <token>` and it will be accepted for auth checks:
```
curl -X POST http://localhost:9091/graphql \
  -H 'Authorization: Bearer demo-user-123' \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ listAuthor { edges { node { id fullName } } } }"}'
```
- To use real signed tokens, wire go-auth’s JWT issuance/verification into `cmd/server/main.go` and replace the demo middleware.

## Seeded IDs (hashid)
- Publisher Aurora: `ddfe89a9-c118-334b-ad2f-941166ef26f4`
- Publisher Nimbus: `83669b17-c772-3c97-8556-23f067d05ba3`
- Author Lina Ortiz: `06ef3339-bd72-333e-9c28-352b9c2cc612`
- Author Miles Dorsey: `ffa617ff-004c-37f2-8877-46a7af753ce2`
- Author Esha Kapur: `3d745fdd-0d5f-3e3c-820e-2c964e445c15`
- Book Contact Shadows: `b3b97f32-4153-3d91-aeaa-8d472c56ba48`
- Book Exo-Archive Accord: `20b5c628-e576-3857-aa21-20ae5281edab`
- Book Microburst Protocol: `325833c4-da6a-382a-881e-4a728e44612d`
- Chapters (Contact Shadows): `03f80682-74cd-3119-87a5-f0da388194f5`, `3f8a7e22-5cba-31ae-be61-1bf6ff5236bf`
- Chapter (Microburst Protocol): `75a5f3e5-9791-367b-9c9c-12fd4fff0f90`
- Tags: Sci-Fi `c13a4cbf-e9ee-3573-8fdd-4ef70a8d1bb0`, Space Opera `452da0f6-bbb3-3127-9140-7a8ccaa0e91b`, Tech Thriller `fba3287e-f489-3457-a4b2-b4c533886008`

## Sample queries

List publishing houses with nested data:

```graphql
query {
  listPublishingHouse {
    pageInfo { total hasNextPage hasPreviousPage startCursor endCursor }
    edges {
      cursor
      node {
        id
        name
        headquarters { city country openedAt }
        authors {
          id
          fullName
          email
          tags { name category }
        }
        books {
          id
          title
          status
          chapters { title chapterIndex }
        }
      }
    }
  }
}
```

Fetch a single author with relations:

```graphql
query {
  listAuthor(filter: [{ field: "full_name", operator: EQ, value: "Lina Ortiz" }]) {
    edges {
      node {
        id
        fullName
        profile { biography favoriteGenre }
        publisher { name }
        books { title status }
        tags { name }
      }
    }
  }
}
```

Fetch a book with relations (filter by title so you don’t need to know the ID):

```graphql
query {
  listBook(filter: [{ field: "title", operator: EQ, value: "Contact Shadows" }]) {
    edges {
      node {
        id
        title
        status
        author { id fullName }
        publisher { id name }
        chapters { id chapterIndex title wordCount }
        tags { id name category }
      }
    }
  }
}
```

Fetch authors by tag filter:

```graphql
query {
  listAuthor(
    filter: [
      { field: "tags.name", operator: EQ, value: "Science Fiction" }
    ]
  ) {
    pageInfo { total hasNextPage hasPreviousPage }
    edges {
      node {
        id
        fullName
        tags { name category }
      }
    }
  }
}
```

Create a tag:

```graphql
mutation {
  createTag(input: { name: "New Genre", category: "genre" }) {
    id
    name
  }
}
```

## Regenerating after model changes
- Source of truth for models is `model.go`. Add new models/fields/relations there and keep the registrar wiring in `registrar/` in sync.
- Run `./taskfile gen` (or `go generate ./...`) to refresh `graph/schema.graphql`, `gqlgen.yml`, and resolver/model scaffolding from the in-memory registry (no metadata.json required).
- Then run `./taskfile gqlgen` to regenerate gqlgen outputs before serving.
- If you need a metadata.json snapshot for fixtures/docs, run `./taskfile metadata` (not used by the generator path).
