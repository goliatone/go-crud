package crud

import (
	"reflect"
	"sort"
	"strings"

	"github.com/ettle/strcase"
	"github.com/goliatone/go-router"
)

// The router's default descriptor understands rel: tags but not Bun m2m: tags.
// Complete only our default provider before application relation filters run.
// Explicit custom providers remain authoritative.
type defaultRelationProvider struct {
	base *router.DefaultRelationProvider
}

func newDefaultRelationProvider() router.RelationMetadataProvider {
	return &defaultRelationProvider{base: router.NewDefaultRelationProvider()}
}

func (p *defaultRelationProvider) BuildRelationDescriptor(typ reflect.Type) (*router.RelationDescriptor, error) {
	descriptor, err := p.base.BuildRelationDescriptor(typ)
	if err != nil {
		return nil, err
	}
	p.complete(descriptor.Tree, indirectType(typ), make(map[reflect.Type]bool))
	paths := make(map[string]bool)
	collectCRUDRelationPaths(descriptor.Tree, "", paths)
	descriptor.Includes = make([]string, 0, len(paths))
	for path := range paths {
		descriptor.Includes = append(descriptor.Includes, path)
	}
	sort.Strings(descriptor.Includes)
	return descriptor, nil
}

func (p *defaultRelationProvider) complete(node *router.RelationNode, typ reflect.Type, ancestors map[reflect.Type]bool) {
	if node == nil || typ.Kind() != reflect.Struct {
		return
	}
	if ancestors[typ] {
		node.Children = nil
		return
	}
	ancestors[typ] = true
	defer delete(ancestors, typ)
	for field := range typ.Fields() {
		if !field.IsExported() || field.Tag.Get(TAG_CRUD) == "-" {
			continue
		}
		tag := field.Tag.Get(TAG_BUN)
		if !strings.Contains(tag, "rel:") && !strings.Contains(tag, "m2m:") {
			continue
		}
		childType := field.Type
		for childType.Kind() == reflect.Pointer || childType.Kind() == reflect.Slice || childType.Kind() == reflect.Array {
			childType = childType.Elem()
		}
		if childType.Kind() != reflect.Struct {
			continue
		}
		name := strings.TrimSpace(strings.Split(field.Tag.Get(TAG_JSON), ",")[0])
		if name == "" || name == "-" {
			name = strcase.ToSnake(field.Name)
		}
		child := node.Children[strings.ToLower(name)]
		if child == nil && strings.Contains(tag, "m2m:") {
			descriptor, err := p.base.BuildRelationDescriptor(childType)
			if err != nil || descriptor.Tree == nil {
				continue
			}
			child = descriptor.Tree
			child.Name = name
			child.Display = strcase.ToCase(name, strcase.TitleCase, ' ')
			if !strings.EqualFold(name, field.Name) {
				child.Aliases = []string{field.Name}
			}
			if node.Children == nil {
				node.Children = make(map[string]*router.RelationNode)
			}
			node.Children[strings.ToLower(name)] = child
			node.Children[strings.ToLower(field.Name)] = child
		}
		p.complete(child, childType, ancestors)
	}
}

func collectCRUDRelationPaths(node *router.RelationNode, parent string, paths map[string]bool) {
	if node == nil {
		return
	}
	seen := make(map[*router.RelationNode]bool)
	for _, child := range node.Children {
		if child == nil || seen[child] {
			continue
		}
		seen[child] = true
		path := child.Name
		if parent != "" {
			path = parent + "." + path
		}
		paths[path] = true
		collectCRUDRelationPaths(child, path, paths)
	}
}
