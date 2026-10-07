// Package goenax is a code-as-source-of-truth registry for HTTP endpoints.
//
// An endpoint is declared once — typed request/response plus metadata — and the
// registry projects that declaration into OpenAPI (and, downstream, Postman and
// TypeScript). Because the request/response schemas are taken from real Go
// types, the contract cannot lie about the shapes the handler binds and returns.
//
// The core package uses only the standard library; framework integration lives
// in the adapter packages (adapter/echo, adapter/gin, adapter/nethttp).
package goenax

import "reflect"

// ErrorSpec is one documented non-2xx outcome of an endpoint.
type ErrorSpec struct {
	Status  int
	Code    string // short machine-readable slug, e.g. "validation"
	Message string // human-readable example message
}

// Param is a query (or, later, path) parameter.
type Param struct {
	Name     string
	In       string // "query"
	Required bool
	Desc     string
}

// ExtraResponse is an additional documented response (e.g. a 202 alternative to
// the primary success). Type is optional — nil means the response has no body.
type ExtraResponse struct {
	Status int
	Desc   string
	Type   reflect.Type
}

// ResponseHeader documents a header returned on the success response.
type ResponseHeader struct {
	Name string
	Desc string
}

// Endpoint is the full contract of a single route: how it is called plus the
// metadata Validate requires. Request and Response hold the Go types of the
// bodies (nil when there is none); their JSON schemas are derived from those
// types when the spec is built.
type Endpoint struct {
	Method string
	Path   string

	Summary string
	Desc    string
	Owner   string
	Tags    []string

	Request  reflect.Type // nil = no request body
	Response reflect.Type // nil = empty response (e.g. 204)
	Status   int          // success status code (200/201/204)

	Secured     bool // true = requires a bearer session
	SuccessHTML bool // success renders text/html rather than JSON
	Params      []Param
	RespHeaders []ResponseHeader // headers on the success response
	Also        []ExtraResponse
	Errors      []ErrorSpec
	DependsOn   []string // IDs of endpoints that must run first (QA scenarios)
}

// Option mutates an Endpoint during Define.
//
// This is the functional-options pattern: each helper below returns a small
// function that sets one field, and Define applies them in order. It keeps
// declarations readable and lets us add new metadata later without changing
// every call site.
type Option func(*Endpoint)

// Summary sets the one-line summary.
func Summary(s string) Option { return func(e *Endpoint) { e.Summary = s } }

// Description sets the longer description.
func Description(s string) Option { return func(e *Endpoint) { e.Desc = s } }

// Owner sets the owning team/channel (e.g. "#team").
func Owner(s string) Option { return func(e *Endpoint) { e.Owner = s } }

// Tags appends grouping tags (folders in Postman, tags in OpenAPI).
func Tags(tags ...string) Option {
	return func(e *Endpoint) { e.Tags = append(e.Tags, tags...) }
}

// DependsOn records endpoint IDs that must be called before this one.
func DependsOn(ids ...string) Option {
	return func(e *Endpoint) { e.DependsOn = append(e.DependsOn, ids...) }
}

// Error documents one non-2xx outcome.
func Error(status int, code, message string) Option {
	return func(e *Endpoint) {
		e.Errors = append(e.Errors, ErrorSpec{Status: status, Code: code, Message: message})
	}
}

// Request declares the request body type, written as goenax.Request[fooRequest]().
//
// It is generic so the caller names the type instead of passing a value.
// reflect.TypeFor[T]() (Go 1.22+) captures that type at compile time, so the
// schema is bound to the exact struct the handler binds — no annotations, and
// the compiler guarantees the type exists.
func Request[T any]() Option {
	return func(e *Endpoint) { e.Request = reflect.TypeFor[T]() }
}

// Response declares the success body type and status,
// e.g. goenax.Response[sessionResponse](200).
func Response[T any](status int) Option {
	return func(e *Endpoint) {
		e.Response = reflect.TypeFor[T]()
		e.Status = status
	}
}

// NoContent declares a success with an empty body, e.g. goenax.NoContent(204).
func NoContent(status int) Option {
	return func(e *Endpoint) {
		e.Response = nil
		e.Status = status
	}
}

// Secured marks the endpoint as requiring a bearer session. Unmarked endpoints
// are public (they emit an empty security requirement).
func Secured() Option {
	return func(e *Endpoint) { e.Secured = true }
}

// Query documents a query parameter.
func Query(name string, required bool, desc string) Option {
	return func(e *Endpoint) {
		e.Params = append(e.Params, Param{Name: name, In: "query", Required: required, Desc: desc})
	}
}

// Header documents a request header (e.g. X-Recaptcha-Token).
func Header(name string, required bool, desc string) Option {
	return func(e *Endpoint) {
		e.Params = append(e.Params, Param{Name: name, In: "header", Required: required, Desc: desc})
	}
}

// PathParam documents a path parameter (always required, must appear as {name}
// in the route path).
func PathParam(name, desc string) Option {
	return func(e *Endpoint) {
		e.Params = append(e.Params, Param{Name: name, In: "path", Required: true, Desc: desc})
	}
}

// HTML marks the success response as text/html (e.g. email-link landing pages).
func HTML() Option {
	return func(e *Endpoint) { e.SuccessHTML = true }
}

// Also documents an additional response status alongside the primary success,
// without a body.
func Also(status int, desc string) Option {
	return func(e *Endpoint) {
		e.Also = append(e.Also, ExtraResponse{Status: status, Desc: desc})
	}
}

// AlsoJSON documents an additional response status that carries a JSON body of
// type T (e.g. a 202 with a { "status": … } payload).
func AlsoJSON[T any](status int, desc string) Option {
	return func(e *Endpoint) {
		e.Also = append(e.Also, ExtraResponse{Status: status, Desc: desc, Type: reflect.TypeFor[T]()})
	}
}

// RespHeader documents a header returned on the success response.
func RespHeader(name, desc string) Option {
	return func(e *Endpoint) {
		e.RespHeaders = append(e.RespHeaders, ResponseHeader{Name: name, Desc: desc})
	}
}

// Compose bundles several options into one. It lets a middleware ship its own
// documentation — e.g. a rate limiter that contributes a 429 response, or a
// captcha that contributes a header and its rejection — so routes opt into the
// behaviour and its docs together.
func Compose(opts ...Option) Option {
	return func(e *Endpoint) {
		for _, opt := range opts {
			opt(e)
		}
	}
}

// Define assembles an Endpoint from a method, path, and options. Status defaults
// to 200 unless an option overrides it. Define does no validation — that is
// Validate's job; here we only build the declaration.
func Define(method, path string, opts ...Option) Endpoint {
	e := Endpoint{Method: method, Path: path, Status: 200}
	for _, opt := range opts {
		opt(&e)
	}

	return e
}
