package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/generator"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "graphqlgen: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("graphqlgen", flag.ContinueOnError)
	fs.SetOutput(out)

	opts := generator.Options{
		OutDir:       "graph",
		ConfigPath:   "gqlgen.yml",
		RunGoImports: true,
	}
	var include, exclude, omitMutationFields stringList
	typeMappings := newTypeMappingFlag()

	fs.StringVar(&opts.MetadataFile, "metadata-file", opts.MetadataFile, "path to a JSON file containing router.SchemaMetadata")
	fs.StringVar(&opts.SchemaPackage, "schema-package", opts.SchemaPackage, "package to import before reading schemas from the registry (optional)")
	fs.StringVar(&opts.OutDir, "out", opts.OutDir, "output base directory for GraphQL assets")
	fs.StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "path to write gqlgen.yml")
	fs.StringVar(&opts.TemplateDir, "template-dir", opts.TemplateDir, "directory containing template overrides")
	fs.BoolVar(&opts.Force, "force", opts.Force, "force overwrite custom stubs (resolver_custom.go)")
	fs.BoolVar(&opts.DryRun, "dry-run", opts.DryRun, "dry run; do not write files")
	fs.BoolVar(&opts.RunGQLGen, "run-gqlgen", opts.RunGQLGen, "run gqlgen after generation (off by default)")
	fs.BoolVar(&opts.SkipGQLGen, "skip-gqlgen", opts.SkipGQLGen, "skip running gqlgen even if requested")
	fs.BoolVar(&opts.RunGoImports, "goimports", opts.RunGoImports, "apply goimports to generated Go files")
	fs.BoolVar(&opts.EmitDataloader, "emit-dataloader", opts.EmitDataloader, "generate dataloader scaffold (off by default)")
	fs.StringVar(&opts.PolicyHook, "policy-hook", opts.PolicyHook, "optional scope guard function to invoke inside generated resolvers")
	fs.StringVar(&opts.OverlayFile, "overlay-file", opts.OverlayFile, "JSON/YAML overlay to enrich metadata with scalars/enums/inputs/operations")
	fs.StringVar(&opts.HooksFile, "hooks-file", opts.HooksFile, "JSON/YAML overlay containing resolver hook snippets/imports")
	fs.StringVar(&opts.AuthPackage, "auth-package", opts.AuthPackage, "import path to add when using auth hooks (e.g., github.com/goliatone/go-auth)")
	fs.StringVar(&opts.AuthGuard, "auth-guard", opts.AuthGuard, "auth guard expression used in resolver hooks (e.g., auth.FromContext(ctx))")
	fs.StringVar(&opts.AuthFail, "auth-fail", opts.AuthFail, "failure expression when auth guard fails; defaults to errors.New(\"unauthorized\")")
	fs.Var(&include, "include", "schema include glob (repeatable)")
	fs.Var(&exclude, "exclude", "schema exclude glob (repeatable)")
	fs.Var(&omitMutationFields, "omit-mutation-field", "field name or entity.field to omit from mutation helpers (repeatable)")
	fs.Var(typeMappings, "type-mapping", "scalar override mapping (e.g., string:uuid=UUID or time.Time=Time)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	opts.Include = include
	opts.Exclude = exclude
	opts.OmitMutationFields = omitMutationFields
	opts.TypeMappings = typeMappings.values

	result, err := generator.Generate(ctx, opts)
	if err != nil {
		return err
	}

	for _, file := range result.Files {
		fmt.Fprintf(out, "%-14s %s\n", file.Status, file.Path)
	}
	switch {
	case result.RanGQLGen:
		fmt.Fprintln(out, "gqlgen: ran")
	case opts.RunGQLGen && opts.DryRun:
		fmt.Fprintln(out, "gqlgen: skipped (dry-run)")
	case opts.RunGQLGen && opts.SkipGQLGen:
		fmt.Fprintln(out, "gqlgen: skipped (skip-gqlgen)")
	}

	return nil
}

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

type typeMappingFlag struct {
	values map[formatter.TypeRef]string
}

func newTypeMappingFlag() *typeMappingFlag {
	return &typeMappingFlag{values: make(map[formatter.TypeRef]string)}
}

func (t *typeMappingFlag) String() string {
	if len(t.values) == 0 {
		return ""
	}

	var items []string
	for ref, scalar := range t.values {
		items = append(items, fmt.Sprintf("%s:%s:%s=%s", ref.Type, ref.Format, ref.GoType, scalar))
	}
	return strings.Join(items, ",")
}

func (t *typeMappingFlag) Set(value string) error {
	ref, scalar, err := parseTypeMapping(value)
	if err != nil {
		return err
	}
	t.values[ref] = scalar
	return nil
}

func parseTypeMapping(value string) (formatter.TypeRef, string, error) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return formatter.TypeRef{}, "", fmt.Errorf("invalid type-mapping %q: expected key=value", value)
	}

	key := strings.TrimSpace(parts[0])
	scalar := strings.TrimSpace(parts[1])
	if key == "" || scalar == "" {
		return formatter.TypeRef{}, "", fmt.Errorf("invalid type-mapping %q: empty key or value", value)
	}

	keyParts := strings.Split(key, ":")
	var ref formatter.TypeRef
	switch len(keyParts) {
	case 1:
		if strings.ContainsAny(keyParts[0], "./") || strings.Contains(keyParts[0], ".") {
			ref.GoType = keyParts[0]
		} else {
			ref.Type = keyParts[0]
		}
	case 2:
		ref.Type = keyParts[0]
		ref.Format = keyParts[1]
	case 3:
		ref.Type = keyParts[0]
		ref.Format = keyParts[1]
		ref.GoType = keyParts[2]
	default:
		return formatter.TypeRef{}, "", fmt.Errorf("invalid type-mapping key %q: use type[:format[:goType]]", key)
	}

	return ref, scalar, nil
}
