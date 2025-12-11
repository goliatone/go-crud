package generator

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/goliatone/go-router"

	"github.com/goliatone/go-crud/gql/internal/formatter"
	"github.com/goliatone/go-crud/gql/internal/gqlgen"
	"github.com/goliatone/go-crud/gql/internal/hooks"
	"github.com/goliatone/go-crud/gql/internal/metadata"
	"github.com/goliatone/go-crud/gql/internal/overlay"
	"github.com/goliatone/go-crud/gql/internal/templates"
	"github.com/goliatone/go-crud/gql/internal/writer"
)

// Options configure the generation flow.
type Options struct {
	MetadataFile       string
	SchemaPackage      string
	OutDir             string
	ConfigPath         string
	Include            []string
	Exclude            []string
	TypeMappings       map[formatter.TypeRef]string
	TemplateDir        string
	Force              bool
	DryRun             bool
	RunGQLGen          bool
	SkipGQLGen         bool
	RunGoImports       bool
	EmitDataloader     bool
	EmitSubscriptions  bool
	SubscriptionEvents []string
	PolicyHook         string
	OmitMutationFields []string
	OverlayFile        string
	HooksFile          string
	AuthPackage        string
	AuthGuard          string
	AuthFail           string
}

// FileResult captures the status of a single write.
type FileResult struct {
	Path   string
	Status writer.Status
}

// Result summarizes generation.
type Result struct {
	Files     []FileResult
	RanGQLGen bool
}

// Generate executes the end-to-end generation flow.
func Generate(ctx context.Context, opts Options) (Result, error) {
	var result Result

	outDir := opts.OutDir
	if outDir == "" {
		outDir = "graph"
	}
	outDir = filepath.Clean(outDir)

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = "gqlgen.yml"
	}
	configPath = filepath.Clean(configPath)

	schemas, err := loadSchemas(opts.MetadataFile, opts.SchemaPackage)
	if err != nil {
		return result, err
	}

	ol, err := overlay.Load(opts.OverlayFile)
	if err != nil {
		return result, fmt.Errorf("load overlay: %w", err)
	}

	if opts.HooksFile != "" {
		hookOverlay, err := overlay.Load(opts.HooksFile)
		if err != nil {
			return result, fmt.Errorf("load hooks overlay: %w", err)
		}
		ol = overlay.Merge(ol, hookOverlay)
	}

	schemas = filterSchemas(schemas, opts.Include, opts.Exclude)
	if len(schemas) == 0 {
		return result, fmt.Errorf("no schemas matched include/exclude filters")
	}

	formatterOpts := []formatter.Option{}
	if len(opts.TypeMappings) > 0 {
		formatterOpts = append(formatterOpts, formatter.WithTypeMappings(opts.TypeMappings))
	}

	doc, err := formatter.Format(schemas, formatterOpts...)
	if err != nil {
		return result, fmt.Errorf("format metadata: %w", err)
	}

	if len(opts.OmitMutationFields) > 0 {
		doc = applyMutationOmissions(doc, opts.OmitMutationFields)
	}

	renderer, err := templates.NewRendererWithBaseDir(opts.TemplateDir)
	if err != nil {
		return result, fmt.Errorf("init renderer: %w", err)
	}

	writerOpts := []writer.Option{
		writer.WithForce(opts.Force),
		writer.WithDryRun(opts.DryRun),
	}
	if !opts.RunGoImports {
		writerOpts = append(writerOpts, writer.WithGoImports(false))
	}
	w := writer.New(writerOpts...)

	ctxData := templates.BuildContext(doc, templates.ContextOptions{
		ConfigPath:         configPath,
		OutDir:             outDir,
		PolicyHook:         opts.PolicyHook,
		EmitDataloader:     opts.EmitDataloader,
		EmitSubscriptions:  opts.EmitSubscriptions,
		SubscriptionEvents: opts.SubscriptionEvents,
		Overlay:            ol,
		AuthPackage:        opts.AuthPackage,
		AuthGuard:          opts.AuthGuard,
		HookOptions: hooks.Options{
			Overlay:     ol.Hooks,
			AuthPackage: opts.AuthPackage,
			AuthGuard:   opts.AuthGuard,
			AuthFail:    opts.AuthFail,
		},
	})

	writeSteps := []struct {
		template string
		path     string
		write    func(string, []byte) (writer.Status, error)
	}{
		{templates.SchemaTemplate, filepath.Join(outDir, "schema.graphql"), w.WriteGenerated},
		{templates.GQLGenConfigTemplate, configPath, w.WriteGenerated},
		{templates.ModelsTemplate, filepath.Join(outDir, "model", "models_gen.go"), w.WriteGenerated},
		{templates.ModelsCustomTemplate, filepath.Join(outDir, "model", "models_custom.go"), w.WriteCustomOnce},
		{templates.ResolverGenTemplate, filepath.Join(outDir, "resolvers", "resolver_gen.go"), w.WriteGenerated},
		{templates.ResolverCustomTemplate, filepath.Join(outDir, "resolvers", "resolver_custom.go"), w.WriteCustomOnce},
	}

	if opts.EmitDataloader {
		writeSteps = append(writeSteps, struct {
			template string
			path     string
			write    func(string, []byte) (writer.Status, error)
		}{
			template: templates.DataloaderTemplate,
			path:     filepath.Join(outDir, "dataloader", "dataloader_gen.go"),
			write:    w.WriteGenerated,
		})
	}

	for _, step := range writeSteps {
		content, err := renderer.Render(step.template, ctxData)
		if err != nil {
			return result, fmt.Errorf("render %s: %w", step.template, err)
		}
		status, err := step.write(step.path, []byte(content))
		if err != nil {
			return result, fmt.Errorf("write %s: %w", step.path, err)
		}
		result.Files = append(result.Files, FileResult{Path: step.path, Status: status})
	}

	if shouldRunGQLGen(opts) {
		if err := gqlgen.Run(ctx, configPath); err != nil {
			return result, err
		}
		result.RanGQLGen = true
	}

	return result, nil
}

func shouldRunGQLGen(opts Options) bool {
	return opts.RunGQLGen && !opts.SkipGQLGen && !opts.DryRun
}

func loadSchemas(metadataFile, schemaPackage string) ([]router.SchemaMetadata, error) {
	if metadataFile != "" {
		schemas, err := metadata.FromFile(metadataFile)
		if err != nil {
			return nil, fmt.Errorf("load metadata from file %s: %w", metadataFile, err)
		}
		return schemas, nil
	}

	if schemaPackage != "" {
		schemas, err := metadata.FromSchemaPackage(schemaPackage)
		if err != nil {
			return nil, fmt.Errorf("load metadata from schema-package %s: %w", schemaPackage, err)
		}
		return schemas, nil
	}

	schemas, err := metadata.FromRegistry()
	if err != nil {
		return nil, fmt.Errorf("load metadata from registry: %w", err)
	}
	return schemas, nil
}

func filterSchemas(schemas []router.SchemaMetadata, include, exclude []string) []router.SchemaMetadata {
	if len(include) == 0 && len(exclude) == 0 {
		return schemas
	}

	var filtered []router.SchemaMetadata
	for _, schema := range schemas {
		name := schema.Name
		if matches(name, exclude) {
			continue
		}
		if len(include) > 0 && !matches(name, include) {
			continue
		}
		filtered = append(filtered, schema)
	}
	return filtered
}

func matches(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}

	target := strings.ToLower(name)
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if ok, err := path.Match(pattern, target); err == nil && ok {
			return true
		}
		if target == pattern {
			return true
		}
	}
	return false
}

func applyMutationOmissions(doc formatter.Document, patterns []string) formatter.Document {
	if len(patterns) == 0 {
		return doc
	}

	global := make(map[string]struct{})
	perEntity := make(map[string]map[string]struct{})

	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		if pattern == "" {
			continue
		}
		lower := strings.ToLower(pattern)
		if strings.Contains(lower, ".") {
			parts := strings.SplitN(lower, ".", 2)
			if len(parts) == 2 {
				entity := parts[0]
				field := parts[1]
				if entity == "" || field == "" {
					continue
				}
				if perEntity[entity] == nil {
					perEntity[entity] = make(map[string]struct{})
				}
				perEntity[entity][field] = struct{}{}
				continue
			}
		}
		global[lower] = struct{}{}
	}

	for i := range doc.Entities {
		entity := &doc.Entities[i]
		entityKey := strings.ToLower(entity.RawName)
		for j := range entity.Fields {
			field := &entity.Fields[j]
			fieldKey := strings.ToLower(field.OriginalName)

			_, matchGlobal := global[fieldKey]
			_, matchEntity := perEntity[entityKey][fieldKey]
			if matchGlobal || matchEntity {
				field.OmitFromMutations = true
			}
		}
	}

	return doc
}
