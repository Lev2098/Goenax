// Package ginadapter mounts routes on a Gin router while recording each route's
// contract in an goenax.Registry — one declaration, two outputs.
//
// Only this file knows about Gin; goenax's core stays framework-neutral, so this
// adapter sits beside the Echo and net/http ones without touching it.
package ginadapter

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Lev2098/Goenax"
)

// Router wraps a Gin group and a registry. group already carries prefix, so
// routes are mounted relative to it and recorded with prefix prepended.
type Router struct {
	group  gin.IRouter
	reg    *goenax.Registry
	prefix string
}

// New wraps a Gin router (typically the engine) together with the registry and
// mounts everything under prefix (e.g. "/api"): New(engine, reg, "/api").
//
// Unlike the Echo adapter, pass the *unprefixed* router — New creates the Gin
// group itself, so the mounted and the recorded paths cannot disagree.
func New(router gin.IRouter, reg *goenax.Registry, prefix string) *Router {
	return &Router{group: router.Group(prefix), reg: reg, prefix: prefix}
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

	r.group.Handle(method, path, handlers...)
	r.reg.Add(goenax.Define(method, r.prefix+path, opts...))
}

// GET mounts and records a GET route.
func (r *Router) GET(path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	r.Handle(http.MethodGet, path, h, opts, mw...)
}

// POST mounts and records a POST route.
func (r *Router) POST(path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	r.Handle(http.MethodPost, path, h, opts, mw...)
}

// PUT mounts and records a PUT route.
func (r *Router) PUT(path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	r.Handle(http.MethodPut, path, h, opts, mw...)
}

// PATCH mounts and records a PATCH route.
func (r *Router) PATCH(path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	r.Handle(http.MethodPatch, path, h, opts, mw...)
}

// DELETE mounts and records a DELETE route.
func (r *Router) DELETE(path string, h gin.HandlerFunc, opts []goenax.Option, mw ...gin.HandlerFunc) {
	r.Handle(http.MethodDelete, path, h, opts, mw...)
}

// Docs mounts a goenax.Docs handler at path (the UI) and path+"/openapi.json"
// (the spec). Gin's default RedirectTrailingSlash sends path+"/" to path. The docs routes are not recorded in the spec and are ignored by
// Coverage. Pass middleware to guard them, e.g. auth in production:
//
//	api.Docs("/docs", goenax.Docs(reg, info), requireStaff)
func (r *Router) Docs(path string, docs http.Handler, mw ...gin.HandlerFunc) {
	handlers := make([]gin.HandlerFunc, 0, len(mw)+1)
	handlers = append(handlers, mw...)
	handlers = append(handlers, gin.WrapH(docs))

	for _, p := range []string{path, path + "/" + goenax.SpecFile} {
		r.group.GET(p, handlers...)
		r.reg.Ignore(http.MethodGet, r.prefix+p)
	}
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
