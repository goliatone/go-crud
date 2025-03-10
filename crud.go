package crud

import (
	"context"
)

type Request interface {
	UserContext() context.Context
	Params(key string, defaultValue ...string) string
	BodyParser(out any) error
	Query(key string, defaultValue ...string) string
	QueryInt(key string, defaultValue ...int) int
	Queries() map[string]string
	Body() []byte
}

type Response interface {
	Status(status int) Response
	JSON(data any, ctype ...string) error
	SendStatus(status int) error
}

type Context interface {
	Request
	Response
}

type ResourceHandler interface {
	// Index fetches all records
	Index(Context) error
	// Show fetches a single record, usually by ID
	Show(Context) error
	Create(Context) error
	CreateBatch(Context) error
	Update(Context) error
	UpdateBatch(Context) error
	Delete(Context) error
	DeleteBatch(Context) error
}

// ResourceController defines an interface for registering CRUD routes
type ResourceController[T any] interface {
	RegisterRoutes(r Router)
}

// Router is a simplified interface from the crud package perspective, referencing the generic router
type Router interface {
	Get(path string, handler func(Context) error) RouterRouteInfo
	Post(path string, handler func(Context) error) RouterRouteInfo
	Put(path string, handler func(Context) error) RouterRouteInfo
	Delete(path string, handler func(Context) error) RouterRouteInfo

	// GET(path string, handler func(Context) error) RouterRouteInfo
	// POST(path string, handler func(Context) error) RouterRouteInfo
	// PUT(path string, handler func(Context) error) RouterRouteInfo
	// DELETE(path string, handler func(Context) error) RouterRouteInfo
}

// RouterRouteInfo is a simplified interface for route info
type RouterRouteInfo interface {
	Name(string) RouterRouteInfo
}
