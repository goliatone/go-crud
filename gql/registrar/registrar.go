package registrar

import "github.com/goliatone/go-crud"

// Controller describes any type that can register its routes into a crud.Router.
type Controller interface {
	RegisterRoutes(crud.Router)
}

// RegisterControllers attaches all provided controllers to a no-op router so their
// schemas are pushed into the global crud registry without binding real routes.
func RegisterControllers(controllers ...Controller) {
	router := NoOpRouter{}
	for _, controller := range controllers {
		if controller == nil {
			continue
		}
		controller.RegisterRoutes(router)
	}
}

// NoOpRouter satisfies crud.Router but drops all route registrations.
type NoOpRouter struct{}

func (NoOpRouter) Get(string, func(crud.Context) error) crud.RouterRouteInfo    { return noOpRoute{} }
func (NoOpRouter) Post(string, func(crud.Context) error) crud.RouterRouteInfo   { return noOpRoute{} }
func (NoOpRouter) Put(string, func(crud.Context) error) crud.RouterRouteInfo    { return noOpRoute{} }
func (NoOpRouter) Patch(string, func(crud.Context) error) crud.RouterRouteInfo  { return noOpRoute{} }
func (NoOpRouter) Delete(string, func(crud.Context) error) crud.RouterRouteInfo { return noOpRoute{} }

type noOpRoute struct{}

func (noOpRoute) Name(string) crud.RouterRouteInfo { return noOpRoute{} }
func (noOpRoute) Description(string) crud.MetadataRouterRouteInfo {
	return noOpRoute{}
}
func (noOpRoute) Summary(string) crud.MetadataRouterRouteInfo { return noOpRoute{} }
func (noOpRoute) Tags(...string) crud.MetadataRouterRouteInfo { return noOpRoute{} }
func (noOpRoute) Parameter(string, string, bool, map[string]any) crud.MetadataRouterRouteInfo {
	return noOpRoute{}
}
func (noOpRoute) RequestBody(string, bool, map[string]any) crud.MetadataRouterRouteInfo {
	return noOpRoute{}
}
func (noOpRoute) Response(int, string, map[string]any) crud.MetadataRouterRouteInfo {
	return noOpRoute{}
}
