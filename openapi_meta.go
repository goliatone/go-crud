package crud

import "github.com/goliatone/go-router"

var _ router.MetadataProvider = (*Controller[any])(nil)

// GetMetadata implements router.MetadataProvider, we use it
// to generate the required info that will be used to create a
// OpenAPI spec or something similar
func (c *Controller[T]) GetMetadata() router.ResourceMetadata {
	metadata := router.GetResourceMetadata(c.resourceType)
	if metadata == nil {
		return router.ResourceMetadata{}
	}
	return *metadata
}
