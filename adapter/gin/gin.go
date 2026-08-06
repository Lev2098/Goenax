// Package ginadapter mounts routes on a Gin router while recording each route's
// contract in an goenax.Registry — one declaration, two outputs.
//
// Only this file knows about Gin; goenax's core stays framework-neutral, so this
// adapter sits beside the Echo and net/http ones without touching it.
package ginadapter

import (
	"github.com/gin-gonic/gin"

	"github.com/Lev2098/Goenax"
)

// Router wraps a Gin router (engine or group) and a registry, prepending prefix
// to recorded paths.
type Router struct {
	group  gin.IRouter
	reg    *goenax.Registry
	prefix string
}

// New wraps a Gin router together with the registry. prefix must match the Gin
// group's prefix so the registry records absolute paths (e.g. "/api").
func New(group gin.IRouter, reg *goenax.Registry, prefix string) *Router {
	return &Router{group: group, reg: reg, prefix: prefix}
}

// Group mirrors Gin group nesting while extending the recorded prefix.
func (r *Router) Group(prefix string, mw ...gin.HandlerFunc) *Router {
	return &Router{
		group:  r.group.Group(prefix, mw...),
		reg:    r.reg,
		prefix: r.prefix + prefix,
	}
}

// Handle mounts a route on Gin (middleware then handler) and records its
// contract. path is relative to this Router's prefix.
func (r *Router) Handle(method, path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	handlers := make([]gin.HandlerFunc, 0, len(mw)+1)
	handlers = append(handlers, mw...)
	handlers = append(handlers, h)

	r.group.Handle(method, r.prefix+path, handlers...)
	r.reg.Add(goenax.Define(method, r.prefix+path, opts...))
}

// GET mounts and records a GET route.
func (r *Router) GET(path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	r.Handle("GET", path, h, opts, mw...)
}

// POST mounts and records a POST route.
func (r *Router) POST(path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	r.Handle("POST", path, h, opts, mw...)
}

// DELETE mounts and records a DELETE route.
func (r *Router) DELETE(path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	r.Handle("DELETE", path, h, opts, mw...)
}

// Routes lists the routes registered on a Gin engine as goenax.Route pairs, for
// use with (*goenax.Registry).Coverage.
func Routes(e *gin.Engine) []goenax.Route {
	registered := e.Routes()
	out := make([]goenax.Route, 0, len(registered))

	for _, rt := range registered {
		out = append(out, goenax.Route{Method: rt.Method, Path: rt.Path})
	}

	return out
}
