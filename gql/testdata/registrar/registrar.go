package registrar

import (
	"github.com/goliatone/go-crud"
	repository "github.com/goliatone/go-repository-bun"
)

type RegistryDemo struct {
	ID   string `json:"id" crud:"resource=registry-demo"`
	Name string `json:"name"`
}

func init() {
	var repo repository.Repository[RegistryDemo]
	ctrl := crud.NewController[RegistryDemo](repo)
	ctrl.RegisterRoutes(noOpRouter{})
}

type noOpRouter struct{}

func (noOpRouter) Get(string, func(crud.Context) error) crud.RouterRouteInfo    { return noOpRoute{} }
func (noOpRouter) Post(string, func(crud.Context) error) crud.RouterRouteInfo   { return noOpRoute{} }
func (noOpRouter) Put(string, func(crud.Context) error) crud.RouterRouteInfo    { return noOpRoute{} }
func (noOpRouter) Patch(string, func(crud.Context) error) crud.RouterRouteInfo  { return noOpRoute{} }
func (noOpRouter) Delete(string, func(crud.Context) error) crud.RouterRouteInfo { return noOpRoute{} }

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
