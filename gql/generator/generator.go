package generator

import (
	"context"

	internal "github.com/goliatone/go-crud/gql/internal/generator"
)

// Options configure the generation flow.
type Options = internal.Options

// FileResult captures the status of a single write.
type FileResult = internal.FileResult

// Result summarizes generation.
type Result = internal.Result

// Generate executes the end-to-end generation flow.
func Generate(ctx context.Context, opts Options) (Result, error) {
	return internal.Generate(ctx, opts)
}
