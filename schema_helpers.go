package crud

import (
	"reflect"
	"strings"

	"github.com/goliatone/go-router"
)

type staticMetadataProvider struct {
	metadata router.ResourceMetadata
}

func (s staticMetadataProvider) GetMetadata() router.ResourceMetadata {
	return s.metadata
}

func collectRelationResourceTypes(root reflect.Type) []reflect.Type {
	acc := make(map[reflect.Type]struct{})
	visited := make(map[reflect.Type]struct{})
	collectRelationResourceTypesRecursive(root, visited, acc)

	if len(acc) == 0 {
		return nil
	}

	result := make([]reflect.Type, 0, len(acc))
	for typ := range acc {
		result = append(result, typ)
	}
	return result
}

func collectRelationResourceTypesRecursive(current reflect.Type, visited map[reflect.Type]struct{}, acc map[reflect.Type]struct{}) {
	base := indirectType(current)
	if base == nil || base.Kind() != reflect.Struct {
		return
	}

	if _, seen := visited[base]; seen {
		return
	}
	visited[base] = struct{}{}

	for i := 0; i < base.NumField(); i++ {
		field := base.Field(i)
		if !field.IsExported() {
			continue
		}
		if field.Tag.Get(TAG_CRUD) == "-" {
			continue
		}

		bunTag := field.Tag.Get(TAG_BUN)
		if bunTag == "" {
			continue
		}

		if !strings.Contains(bunTag, "rel:") && !strings.Contains(bunTag, "m2m:") {
			continue
		}

		childType := field.Type
		for childType.Kind() == reflect.Ptr || childType.Kind() == reflect.Slice || childType.Kind() == reflect.Array {
			childType = childType.Elem()
		}

		childBase := indirectType(childType)
		if childBase == nil || childBase.Kind() != reflect.Struct {
			continue
		}

		if childBase != base {
			acc[childBase] = struct{}{}
		}

		collectRelationResourceTypesRecursive(childBase, visited, acc)
	}
}
