//go:build tools
// +build tools

// This file is ignored in normal builds and only exists to host the go:generate
// directive for regenerating the GraphQL assets from the relationships metadata.
package main

// Regenerate the GraphQL schema/config/resolvers for the relationships example.
// Uses the checked-in metadata fixture rather than a live registrar to keep the
// example deterministic.
//go:generate go run ../gql/cmd/graphqlgen --metadata-file ../gql/examples/relationships/metadata.json --out ./graph --config ./gqlgen.yml
