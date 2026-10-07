package goenax_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"

	goenax "github.com/Lev2098/Goenax"
)

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type Session struct {
	Token string `json:"token"`
	Role  string `json:"role" validate:"oneof=admin member"`
}

type ListUsersParams struct {
	Org   string `param:"org"   doc:"organisation slug"`
	Limit int    `query:"limit" validate:"min=1,max=100" doc:"page size"`
	Role  string `query:"role"  validate:"omitempty,oneof=admin member"`
}

// Declare an endpoint once; the registry turns it into an OpenAPI document.
// In an application the adapters (adapter/echo, adapter/gin, adapter/nethttp)
// call Define and Add for you while they mount the route.
func Example() {
	reg := goenax.New()
	reg.Add(goenax.Define(http.MethodPost, "/api/login",
		goenax.Summary("Exchange credentials for a session"),
		goenax.Description("Returns a bearer token."),
		goenax.Owner("#auth"),
		goenax.Tags("Auth"),
		goenax.Request[LoginRequest](),
		goenax.Response[Session](http.StatusOK),
		goenax.Error(http.StatusUnauthorized, "invalid", "wrong email or password"),
	))

	spec, err := reg.OpenAPI(goenax.Info{Title: "My API", Version: "1.0.0"})
	if err != nil {
		panic(err)
	}

	var doc struct {
		OpenAPI    string                    `json:"openapi"`
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		panic(err)
	}

	schemas := slices.Sorted(func(yield func(string) bool) {
		for name := range doc.Components.Schemas {
			if !yield(name) {
				return
			}
		}
	})

	fmt.Println("openapi:", doc.OpenAPI)
	fmt.Println("operations:", len(doc.Paths["/api/login"]))
	fmt.Println("schemas:", schemas)
	// Output:
	// openapi: 3.1.0
	// operations: 1
	// schemas: [LoginRequest Session]
}

// Params documents path, query and header parameters from the struct the
// handler binds — here with Echo's tags (Gin's uri/form work too).
func ExampleParams() {
	ep := goenax.Define(http.MethodGet, "/orgs/{org}/users", goenax.Params[ListUsersParams]())

	for _, p := range ep.Params {
		fmt.Printf("%-5s %-5s required=%-5v type=%s\n", p.In, p.Name, p.Required, p.Schema.Type)
	}
	// Output:
	// path  org   required=true  type=string
	// query limit required=false type=integer
	// query role  required=false type=string
}

// Validate is the policy gate: run it in a test and fail CI on any problem.
func ExampleRegistry_Validate() {
	reg := goenax.New()
	reg.Add(goenax.Define(http.MethodDelete, "/api/users/{id}",
		goenax.Summary("Delete a user"),
	))

	for _, problem := range reg.Validate() {
		fmt.Println(problem)
	}
	// Output:
	// DELETE /api/users/{id}: description; owner; tags; error responses; undeclared path parameter {id}
}

// ValidateResponse checks a real handler response against the declared type —
// call it from a handler test to catch a response that drifted from its contract.
func ExampleRegistry_ValidateResponse() {
	reg := goenax.New()
	reg.Add(goenax.Define(http.MethodPost, "/api/login", goenax.Response[Session](http.StatusOK)))

	fmt.Println(reg.ValidateResponse(http.MethodPost, "/api/login", []byte(`{"token":"t","role":"admin"}`)))
	fmt.Println(reg.ValidateResponse(http.MethodPost, "/api/login", []byte(`{"token":"t","role":"root"}`)))
	// Output:
	// <nil>
	// $.role: root is not one of [admin member]: value not in enum
}

// Coverage diffs the router's real routes against the declared ones. Router
// syntax (":id") and OpenAPI syntax ("{id}") compare equal.
func ExampleRegistry_Coverage() {
	reg := goenax.New()
	reg.Add(goenax.Define(http.MethodGet, "/api/users/{id}"))
	reg.Add(goenax.Define(http.MethodGet, "/api/orders"))
	reg.Ignore(http.MethodGet, "/healthz")

	undocumented, unmounted := reg.Coverage([]goenax.Route{
		{Method: http.MethodGet, Path: "/api/users/:id"},
		{Method: http.MethodPost, Path: "/api/users"},
		{Method: http.MethodGet, Path: "/healthz"},
	})

	fmt.Println("undocumented:", undocumented)
	fmt.Println("unmounted:", unmounted)
	// Output:
	// undocumented: [{POST /api/users}]
	// unmounted: [{GET /api/orders}]
}

// Docs serves the live spec and an interactive UI. With an adapter, mount it
// with Router.Docs instead — that also keeps the docs routes out of the spec.
func ExampleDocs() {
	reg := goenax.New()
	reg.Add(goenax.Define(http.MethodGet, "/api/ping", goenax.Summary("Health check")))

	docs := goenax.Docs(reg, goenax.Info{Title: "My API", Version: "1.0.0"}, goenax.WithUI(goenax.Scalar))

	mux := http.NewServeMux()
	mux.Handle("GET /docs", docs)
	mux.Handle("GET /docs/openapi.json", docs)

	for _, path := range []string{"/docs", "/docs/openapi.json"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		fmt.Println(path, rec.Code, rec.Header().Get("Content-Type"))
	}
	// Output:
	// /docs 200 text/html; charset=utf-8
	// /docs/openapi.json 200 application/json
}

func ExampleNormalizePath() {
	fmt.Println(goenax.NormalizePath("/users/:id/files/*path"))
	fmt.Println(goenax.NormalizePath("/static/{rest...}"))
	// Output:
	// /users/{id}/files/{path}
	// /static/{rest}
}
