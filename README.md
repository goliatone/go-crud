# relationships-gql example

GraphQL version of the relationships demo. It wires the generated schema/resolvers to go-crud services backed by an in-memory sqlite database (bun), then seeds a small catalog so you can query immediately.

## What this shows
- Generating GraphQL schema + gqlgen config from metadata (`gql/examples/relationships/metadata.json`) via our `graphqlgen`.
- Running gqlgen on top of the generated artifacts.
- go-crud services layered on bun repositories with relations (belongs-to, has-one, has-many, many-to-many).
- In-memory sqlite setup and seeding with publishers, authors, books, chapters, tags, and pivot rows.

## How it works
1) `./taskfile build` runs `graphqlgen` to emit `graph/schema.graphql`, `gqlgen.yml`, model/resolver stubs, then runs `gqlgen generate`.
2) The app boots an in-memory sqlite DB, registers bun models, migrates tables, seeds data, constructs CRUD services, and mounts them behind gqlgen handlers.
3) The server uses go-router’s Fiber adapter on `:9091`; restart wipes the DB and re-seeds.

## Run it
```bash
GO_BIN=/path/to/go ./taskfile clean   # optional: remove generated files
GO_BIN=/path/to/go ./taskfile build   # regen schema + gqlgen output
GO_BIN=/path/to/go ./taskfile serve   # start server on :9091
```
Endpoints: GraphQL `http://localhost:9091/graphql` • Playground `http://localhost:9091/playground`

## Seeded data (grab IDs from queries below)
- Publishers: Aurora Press, Nimbus Editorial (each with a headquarters)
- Authors: Lina Ortiz, Miles Dorsey, Esha Kapur (+ profiles for Lina, Miles)
- Books: Contact Shadows, The Exo-Archive Accord, Microburst Protocol (+ chapters)
- Tags: Science Fiction, Space Opera, Tech Thriller (linked to authors/books)

## Playground snippets
Run the list query first to copy IDs, then use them in the mutations.

List publishers with nested authors/books/tags (also gives you IDs):
```graphql
query ListPublishers {
  listPublishingHouse {
    id
    name
    imprintPrefix
    headquarters { city country openedAt }
    authors { id fullName email tags { name category } }
    books { id title isbn status chapters { chapterIndex title } }
  }
}
```

Read one author with relations:
```graphql
query GetAuthor($id: UUID!) {
  getAuthor(id: $id) {
    id
    fullName
    penName
    email
    tags { name category }
    publisher { id name }
    profile { biography favoriteGenre writingStyle }
    books { id title status chapters { chapterIndex title } }
  }
}
```
Vars: `{"id": "AUTHOR-ID-HERE"}`

Filter/paginate books:
```graphql
query ListBooks {
  listBook(
    pagination: { limit: 5, offset: 0 }
    orderBy: [{ field: "title", direction: ASC }]
    filter: [{ field: "status", operator: EQ, value: "in_print" }]
  ) {
    id
    title
    status
    author { id fullName }
    tags { name }
  }
}
```

Create a tag:
```graphql
mutation CreateTag($input: CreateTagInput!) {
  createTag(input: $input) { id name category description }
}
```
Vars:
```json
{"input":{"name":"Cyber Noir","category":"genre","description":"Dark, techy vibes"}}
```

Create a book (use IDs from the list queries):
```graphql
mutation CreateBook($input: CreateBookInput!) {
  createBook(input: $input) { id title isbn status authorId publisherId }
}
```
Vars:
```json
{
  "input": {
    "title": "Glass City",
    "isbn": "978-1-4028-9999-9",
    "status": "draft",
    "authorId": "AUTHOR-ID-HERE",
    "publisherId": "PUBLISHER-ID-HERE"
  }
}
```

Update an author’s pen name:
```graphql
mutation UpdateAuthor($id: UUID!, $input: UpdateAuthorInput!) {
  updateAuthor(id: $id, input: $input) { id fullName penName updatedAt }
}
```
Vars:
```json
{"id":"AUTHOR-ID-HERE","input":{"penName":"New Pen"}}
```

Delete a chapter:
```graphql
mutation DeleteChapter($id: UUID!) {
  deleteChapter(id: $id)
}
```
Vars: `{"id": "CHAPTER-ID-HERE"}`
