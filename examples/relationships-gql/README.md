# relationships-gql example

This example mirrors the REST relationships demo but serves the data over GraphQL using gqlgen, go-crud services, and an in-memory sqlite/bun setup seeded via fixtures (hashid-generated IDs).

## Running

```
./taskfile graphqlgen   # regenerate schema + gqlgen.yml from metadata
./taskfile gqlgen       # run gqlgen to refresh generated code
./taskfile serve        # starts the server on :9091
```

GraphQL endpoint: `http://localhost:9091/graphql`  
Playground: `http://localhost:9091/playground`

All list operations return Relay connections (`edges { node cursor }` + `pageInfo`) so clients can page using cursors while still seeing total counts.

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
  getAuthor(id: "06ef3339-bd72-333e-9c28-352b9c2cc612") {
    id
    fullName
    profile { biography favoriteGenre }
    publisher { name }
    books { title status }
  }
}
```

Fetch a book with relations:

```graphql
query {
  getBook(id: "b3b97f32-4153-3d91-aeaa-8d472c56ba48") {
    id
    title
    status
    author { id fullName }
    publisher { id name }
    chapters { id chapterIndex title wordCount }
    tags { id name category }
  }
}
```

Fetch authors by tag filter:

```graphql
query {
  listAuthor(filter: [{ field: "tags.name", operator: EQ, value: "Science Fiction" }]) {
    pageInfo { total hasNextPage hasPreviousPage }
    edges {
      node {
        id
        fullName
        tags { name }
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

Regeneration inputs live in `gql/examples/relationships/metadata.json`; outputs are written to `graph/`.
