package crud

import "github.com/goliatone/go-router"

var _ router.MetadataProvider = (*Controller[any])(nil)

// GetMetadata implements router.MetadataProvider, we use it
// to generate the required info that will be used to create a
// OpenAPI spec or something similar
func (c *Controller[T]) GetMetadata() router.ResourceMetadata {
	metadata := router.GetResourceMetadata(c.resourceType)
	if metadata == nil {
		if len(c.actionRouteDefs) == 0 {
			return router.ResourceMetadata{}
		}
		return router.ResourceMetadata{
			Routes: append([]router.RouteDefinition{}, c.actionRouteDefs...),
		}
	}
	copyMeta := *metadata
	if len(copyMeta.Routes) > 0 {
		copyMeta.Routes = append([]router.RouteDefinition{}, copyMeta.Routes...)
	}
	if len(c.actionRouteDefs) > 0 {
		copyMeta.Routes = append(copyMeta.Routes, c.actionRouteDefs...)
	}
	return copyMeta
}
