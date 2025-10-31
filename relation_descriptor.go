package crud

import (
	"reflect"
	"strings"
	"sync"

	"github.com/goliatone/go-router"
)

type relationConfig struct {
	provider router.RelationMetadataProvider
}

var (
	relationProviderRegistry sync.Map // map[reflect.Type]relationConfig
	relationDescriptorCache  sync.Map // map[reflect.Type]*router.RelationDescriptor
)

func registerRelationProvider(typ reflect.Type, provider router.RelationMetadataProvider) {
	base := indirectType(typ)
	if base == nil {
		return
	}
	if provider == nil {
		provider = router.NewDefaultRelationProvider()
	}

	relationProviderRegistry.Store(base, relationConfig{
		provider: provider,
	})

	relationDescriptorCache.Delete(base)
	queryConfigRegistry.Delete(base)
}

func getRelationDescriptorForType(typ reflect.Type) *router.RelationDescriptor {
	base := indirectType(typ)
	if base == nil {
		return nil
	}

	if cached, ok := relationDescriptorCache.Load(base); ok {
		if descriptor, ok := cached.(*router.RelationDescriptor); ok {
			return descriptor
		}
	}

	provider := router.NewDefaultRelationProvider()
	if cfg, ok := relationProviderRegistry.Load(base); ok {
		if rc, ok := cfg.(relationConfig); ok && rc.provider != nil {
			provider = rc.provider
		}
	}

	descriptor, err := provider.BuildRelationDescriptor(base)
	if err != nil || descriptor == nil {
		return nil
	}

	descriptor = router.ApplyRelationFilters(base, descriptor)
	relationDescriptorCache.Store(base, descriptor)
	return descriptor
}

func descriptorIncludes(descriptor *router.RelationDescriptor, path string) bool {
	if descriptor == nil {
		return false
	}
	target := strings.ToLower(path)
	for _, include := range descriptor.Includes {
		if strings.ToLower(include) == target {
			return true
		}
	}
	return false
}

func invalidateRelationDescriptorCache() {
	relationDescriptorCache.Range(func(key, _ any) bool {
		relationDescriptorCache.Delete(key)
		return true
	})
}
