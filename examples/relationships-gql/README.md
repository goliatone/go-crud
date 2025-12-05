# relationships-gql example

This example mirrors the REST relationships demo but serves the data over GraphQL using gqlgen, go-crud services, and an in-memory sqlite/bun setup.

## Running

```
./taskfile graphqlgen   # regenerate schema + gqlgen.yml from metadata
./taskfile gqlgen       # run gqlgen to refresh generated code
./taskfile serve        # starts the server on :9091
```

GraphQL endpoint: `http://localhost:9091/graphql`  
Playground: `http://localhost:9091/playground`

## Sample queries

List publishing houses with nested data:

```graphql
query {
  listPublishingHouse {
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
```

Fetch a single author with relations:

```graphql
query {
  getAuthor(id: "00000000-0000-0000-0000-000000000001") {
    id
    fullName
    profile { biography favoriteGenre }
    publisher { name }
    books { title status }
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

Regeneration inputs live in `gql/examples/relationships/metadata.json`; outputs are written to `graph/`.
