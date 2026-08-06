// Package echoadapter mounts routes on an Echo router while recording each
// route's contract in an goenax.Registry — one declaration, two outputs: a live
// route and a spec entry.
//
// Only this adapter knows about Echo; goenax's core stays framework-neutral, so
// a Gin or net/http adapter is an isolated add-on that never touches the core.
package echoadapter

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/Lev2098/Goenax"
)

// mounter is the slice of *echo.Echo / *echo.Group behaviour the adapter needs.
// Both satisfy it, so a Router can wrap either.
type mounter interface {
	Add(method, path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	Group(prefix string, m ...echo.MiddlewareFunc) *echo.Group
}

// Router wraps an Echo mounter and a registry. prefix is the path already
// applied by the wrapped Echo group, so recorded paths are absolute.
type Router struct {
	echo   mounter
	reg    *goenax.Registry
	prefix string
}

// New wraps an Echo router (e.g. e.Group("/api")) together with the
// registry. prefix must match the Echo group's prefix so the registry records
// full paths (e.g. "/api").
func New(m mounter, reg *goenax.Registry, prefix string) *Router {
	return &Router{echo: m, reg: reg, prefix: prefix}
}

// Group mirrors Echo group nesting while extending the recorded prefix.
func (r *Router) Group(prefix string, mw ...echo.MiddlewareFunc) *Router {
	return &Router{
		echo:   r.echo.Group(prefix, mw...),
		reg:    r.reg,
		prefix: r.prefix + prefix,
	}
}

// Handle mounts a route on Echo and records its contract in the registry. path
// is relative to this Router's prefix (as with Echo groups).
func (r *Router) Handle(
	method, path string,
	h echo.HandlerFunc,
	opts []goenax.Option,
	mw ...echo.MiddlewareFunc,
) {
	r.echo.Add(method, path, h, mw...)
	r.reg.Add(goenax.Define(method, r.prefix+path, opts...))
}

// GET mounts and records a GET route.
func (r *Router) GET(path string, h echo.HandlerFunc, opts []goenax.Option, mw ...echo.MiddlewareFunc) {
	r.Handle(http.MethodGet, path, h, opts, mw...)
}

// POST mounts and records a POST route.
func (r *Router) POST(path string, h echo.HandlerFunc, opts []goenax.Option, mw ...echo.MiddlewareFunc) {
	r.Handle(http.MethodPost, path, h, opts, mw...)
}

// DELETE mounts and records a DELETE route.
func (r *Router) DELETE(path string, h echo.HandlerFunc, opts []goenax.Option, mw ...echo.MiddlewareFunc) {
	r.Handle(http.MethodDelete, path, h, opts, mw...)
}

// Routes lists the routes registered on an Echo instance as goenax.Route pairs,
// for use with (*goenax.Registry).Coverage.
func Routes(e *echo.Echo) []goenax.Route {
	registered := e.Routes()
	out := make([]goenax.Route, 0, len(registered))

	for _, rt := range registered {
		out = append(out, goenax.Route{Method: rt.Method, Path: rt.Path})
	}

	return out
}
