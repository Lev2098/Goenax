// Package nethttpadapter mounts routes on a net/http ServeMux while recording
// each route's contract in an goenax.Registry — one declaration, two outputs.
//
// It relies on the Go 1.22+ ServeMux, whose patterns carry the method
// ("POST /path"). Like the Echo adapter, goenax's core stays framework-neutral;
// this file is the only part that knows about net/http.
package nethttpadapter

import (
	"net/http"
	"slices"

	"github.com/Lev2098/Goenax"
)

// Middleware is the standard net/http middleware shape.
type Middleware func(http.Handler) http.Handler

// Router wraps a ServeMux and a registry, prepending prefix to recorded paths.
type Router struct {
	mux    *http.ServeMux
	reg    *goenax.Registry
	prefix string
}

// New wraps a ServeMux together with the registry. prefix is prepended to every
// recorded path (e.g. "/api") so the registry holds absolute paths.
func New(mux *http.ServeMux, reg *goenax.Registry, prefix string) *Router {
	return &Router{mux: mux, reg: reg, prefix: prefix}
}

// Handle mounts a route on the mux (as "METHOD /prefix/path") and records its
// contract. Middleware is applied outermost-first.
func (r *Router) Handle(
	method, path string,
	h http.HandlerFunc,
	opts []goenax.Option,
	mw ...Middleware,
) {
	full := r.prefix + path

	r.mux.Handle(method+" "+full, wrap(h, mw))
	r.reg.Add(goenax.Define(method, full, opts...))
}

// GET mounts and records a GET route.
func (r *Router) GET(path string, h http.HandlerFunc, opts []goenax.Option, mw ...Middleware) {
	r.Handle(http.MethodGet, path, h, opts, mw...)
}

// POST mounts and records a POST route.
func (r *Router) POST(path string, h http.HandlerFunc, opts []goenax.Option, mw ...Middleware) {
	r.Handle(http.MethodPost, path, h, opts, mw...)
}

// PUT mounts and records a PUT route.
func (r *Router) PUT(path string, h http.HandlerFunc, opts []goenax.Option, mw ...Middleware) {
	r.Handle(http.MethodPut, path, h, opts, mw...)
}

// PATCH mounts and records a PATCH route.
func (r *Router) PATCH(path string, h http.HandlerFunc, opts []goenax.Option, mw ...Middleware) {
	r.Handle(http.MethodPatch, path, h, opts, mw...)
}

// DELETE mounts and records a DELETE route.
func (r *Router) DELETE(path string, h http.HandlerFunc, opts []goenax.Option, mw ...Middleware) {
	r.Handle(http.MethodDelete, path, h, opts, mw...)
}

// Docs mounts a goenax.Docs handler at path and path+"/" (the UI) and
// path+"/openapi.json" (the spec). The docs routes are not recorded in the spec and are ignored by
// Coverage. Pass middleware to guard them, e.g. auth in production:
//
//	api.Docs("/docs", goenax.Docs(reg, info), requireStaff)
func (r *Router) Docs(path string, docs http.Handler, mw ...Middleware) {
	h := wrap(docs, mw)

	for _, p := range []string{path, path + "/{$}", path + "/" + goenax.SpecFile} {
		r.mux.Handle(http.MethodGet+" "+r.prefix+p, h)
		r.reg.Ignore(http.MethodGet, r.prefix+p)
	}
}

// wrap applies middleware outermost-first.
func wrap(h http.Handler, mw []Middleware) http.Handler {
	for _, m := range slices.Backward(mw) {
		h = m(h)
	}

	return h
}
