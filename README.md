# Goenax

**Code-as-source-of-truth API registry for Go.**

Declare each HTTP endpoint once — in the same place you mount it on your router —
and Goenax builds an OpenAPI 3.1 document from your Go types. From that one
source you generate the Postman collection and the TypeScript types with the
standard tools, and Goenax gives you the checks that keep them honest.

Framework-neutral: adapters for **Echo**, **net/http** and **Gin** ship in the
box; the core imports none of them.

```
go get github.com/Lev2098/Goenax
```


---

## Why

Your API contract already exists in the code. Writing it down a second time — a
hand-kept OpenAPI file, a Postman collection, doc comments — creates a copy that
drifts the first week a handler changes and the copy doesn't. Annotation tools
move the drift into comments the compiler never checks. Goenax takes the contract
from the Go **types** you already have, so the schema can't describe a shape the
compiler doesn't know.

---

## Quickstart

```go
import (
    "net/http"

    "github.com/labstack/echo/v4"
    "github.com/Lev2098/Goenax"
    echoadapter "github.com/Lev2098/Goenax/adapter/echo"
)

type LoginRequest struct {
    Email    string `json:"email"    validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
}
type SessionResponse struct {
    Token string `json:"token"`
}

e := echo.New()
reg := goenax.New()
api := echoadapter.New(e.Group("/api"), reg, "/api")

api.POST("/login", loginHandler, []goenax.Option{
    goenax.Summary("Exchange credentials for a session"),
    goenax.Owner("#team"),
    goenax.Tags("Auth"),
    goenax.Request[LoginRequest](),
    goenax.Response[SessionResponse](http.StatusOK),
    goenax.Error(http.StatusUnauthorized, "invalid", "wrong email or password"),
}, authMiddleware)

// one call did two things: mounted the live route on Echo,
// and recorded its contract in reg.

spec, _ := reg.OpenAPI(goenax.Info{
    Title: "My API", Version: "1.0.0",
    Servers: []goenax.Server{{URL: "http://localhost:8080"}},
})
```

To emit the spec in CI **without booting the server**, keep the declarations in a
table that both the live mount and a handler-free `Spec()` consume (the option
sets don't need handlers) — then a tiny `cmd` prints `Spec().OpenAPI(...)`.

---

## The declaration API

| Option | Documents |
| ------ | --------- |
| `Summary(s)` · `Description(s)` | human text |
| `Owner(s)` | owning team/channel — required by `Validate` |
| `Tags(t...)` | grouping (OpenAPI tags, Postman folders) |
| `Request[T]()` | request body type |
| `Response[T](status)` | success body type + status |
| `NoContent(status)` | success with no body |
| `HTML()` | success renders `text/html` |
| `Also(status, desc)` · `AlsoJSON[T](status, desc)` | an extra status, without / with a body |
| `RespHeader(name, desc)` | a header on the success response |
| `Error(status, code, msg)` | a documented failure |
| `Params[T]()` | path / query / header parameters, typed, from the struct the handler binds |
| `Query` / `Header` / `PathParam` | a single parameter, as a plain string |
| `Secured()` | requires a bearer session |
| `DependsOn(ids...)` | endpoints a QA run must call first |
| `Compose(opts...)` | bundle options (e.g. a middleware's own docs) |

`Request[T]` / `Response[T]` are generic: `reflect.TypeFor[T]()` binds the schema
to the exact struct at compile time — no annotations.

### Parameters from the bound struct

`Params[T]` does for parameters what `Request[T]` does for the body: document
the struct the handler already binds, so a renamed parameter can't leave the
spec behind.

```go
type ListUsersParams struct {
    Org   string `param:"org"   doc:"organisation slug"`
    Limit int    `query:"limit" validate:"min=1,max=100" doc:"page size"`
    Role  string `query:"role"  validate:"omitempty,oneof=admin member"`
}

api.GET("/orgs/:org/users", func(c echo.Context) error {
    var p ListUsersParams
    if err := c.Bind(&p); err != nil { … }   // the same struct…
    …
}, []goenax.Option{
    goenax.Params[ListUsersParams](),         // …documents the parameters
    …
})
```

| Tag | Location | Router |
| --- | -------- | ------ |
| `param:"…"` / `uri:"…"` / `path:"…"` | path (always required) | Echo / Gin / net/http |
| `query:"…"` / `form:"…"` | query | Echo, net/http / Gin |
| `header:"…"` | header | all |

Types and `validate` rules become the schema (`limit` above is an integer,
1–100; `role` an enum). Query and header parameters are optional unless
`validate:"required"`. `doc:"…"` is the description; untagged fields are ignored
and embedded structs are flattened, so shared pagination can be one embedded
struct.

---

## Schema derivation

Goenax reflects over the request/response types and reads the `json` and
go-playground `validate` tags:

| Go | OpenAPI schema |
| -- | -------------- |
| primitives | `string` / `integer` / `boolean` / `number` |
| `*T`, or `json:",omitempty"` | optional |
| `validate:"required"` | required |
| `validate:"email"` / `"uuid"` / `"url"` | `format` |
| `validate:"min/max/len"` | `minLength`/`maxLength` or `minimum`/`maximum` |
| `validate:"oneof=a b c"` | `enum` (typed: `oneof=1 2` on an `int` is `[1, 2]`) |
| `validate:"dive,…"` | rules after `dive` apply to array items / map values |
| `example:"…"` | `example`, typed to the field (`example:"42"` on an `int` is `42`); else a deterministic default |
| `time.Time` / `[]T` / `map[string]T` | `date-time` / `array` / object with `additionalProperties` |
| named `struct` | a `$ref` into `components/schemas` (defined once, shared) |

It is cycle-safe and gives every leaf a deterministic example, so a generated
Postman collection is stable across runs.

A field without `omitempty` that isn't a pointer is `required` — in requests as
well as responses. Make an optional request field a pointer or `omitempty`.

Every `Error` documents the same JSON body — `{ "message": "…" }` by default.
If your API returns something else, give its type once:

```go
reg.OpenAPI(goenax.Info{Title: "My API", Version: "1.0.0",
    ErrorType: reflect.TypeFor[APIError]()})
```

---

## Examples

- **Runnable demo** — `go run ./examples/echo`, then open
  <http://localhost:8080/api/docs> (also `/api/scalar`, `/api/redoc`). It
  shows `Request`/`Response`, `Params[T]`, `Info.ErrorType`, `Validate` and
  `Coverage` on boot, and the live docs.
- **API examples** — [`example_test.go`](./example_test.go), also rendered on
  [pkg.go.dev](https://pkg.go.dev/github.com/Lev2098/Goenax). They run as tests,
  so they can't go stale.

---

## Adapters

Same shape for every framework — `New`, `Group`, `Handle`/`GET`/`POST`/`PUT`/
`PATCH`/`DELETE`, `Docs`. Each mounts the route **and** records the contract.
Only the adapter imports the framework.

```go
echoadapter.New(e.Group("/api"), reg, "/api")   // pass the already-prefixed Echo group
nethttpadapter.New(mux, reg, "/api")             // *http.ServeMux (Go 1.22+ patterns)
ginadapter.New(engine, reg, "/api")              // pass the unprefixed router; New creates the group
```

Write paths in the framework's own syntax — `/users/:id` on Echo/Gin,
`/users/{id}` on net/http. The registry stores them in OpenAPI form
(`/users/{id}`; see `goenax.NormalizePath`), so `PathParam("id", …)`, the spec
and `Coverage` all line up.

Middleware uses each framework's native type. `echoadapter.Routes(e)` /
`ginadapter.Routes(engine)` list the registered routes for `Coverage` (below).

---

## Live docs (`/docs`)

Like FastAPI, the running service can serve its own contract — the spec comes
from the same registry the routes were mounted through, so it always describes
exactly what this build serves.

```go
api.Docs("/docs", goenax.Docs(reg, goenax.Info{
    Title:   "My API",
    Version: version, // e.g. set via -ldflags, so /docs shows which build answers
}), requireStaff)
```

- `GET /api/docs` — interactive UI; `GET /api/docs/openapi.json` — the spec.
- UI: `goenax.WithUI(goenax.SwaggerUI)` (default), `goenax.Scalar` or
  `goenax.Redoc`. Assets load from the jsDelivr CDN.
- The spec is built lazily, cached until the registry changes, and served with an
  `ETag`.
- Leave `Info.Servers` empty: "Try it out" then calls whichever host served the
  page, so one binary works on every environment.
- The docs routes are not part of the spec and are ignored by `Coverage`. Use
  `reg.Ignore(method, path)` for other intentionally undocumented routes
  (health checks, metrics).
- **The spec reveals every endpoint.** In production, guard it with middleware
  or don't mount it (`if cfg.DocsEnabled { … }`).

Without an adapter, `goenax.Docs` is a plain `http.Handler`: mount it on
`path` and `path + "/openapi.json"`.

For one portal across many services, let the portal *pull* each service's
`/docs/openapi.json` (Swagger UI `urls`, Scalar, Backstage) — the service stays
the source of truth.

---

## Keeping it honest

Goenax gives you the checks; wire them into your tests and CI.

- **`reg.Validate()`** — returns problems per endpoint: missing
  summary/description/owner/tag, a mutating method with no error response, a
  duplicate method+path, a path `{token}` that doesn't match a declared
  `PathParam`, a status code claimed by two responses (e.g. `Also(200)` next
  to a 200 success), or two routes producing the same `operationId`
  (`/a-b` and `/a_b`). Several `Error`s on one status are fine — they merge. Fail CI on a non-empty
  result and documentation becomes a build requirement, not a reviewer's ask.
- **`reg.ValidateResponse(method, path, body)`** — validates a real response
  against its declared `Response[T]` schema. Call it from a handler test and a
  handler that returns the wrong shape fails — closing the gap between what an
  endpoint *promises* and what it *returns*.
- **`reg.Coverage(actual)`** — diffs the routes registered on the router against
  the declared endpoints, flagging routes that are undocumented or documented but
  never mounted.
- **Regenerate & diff.** Commit the generated spec (and Postman / TS) and, in CI,
  regenerate them and `diff` against the committed files — a change that isn't
  reflected fails the build.
- **Breaking changes.** Diff the spec against your baseline with
  [`oasdiff`](https://github.com/oasdiff/oasdiff) and fail on a breaking change.

Postman and TypeScript come from the emitted OpenAPI via `openapi-to-postman` and
`openapi-typescript` — Goenax doesn't reinvent them.

---

## Changelog

### Unreleased

**New**

- `Params[T]()` — typed path / query / header parameters from the struct the
  handler binds (Echo, Gin and net/http binding tags), with `doc:"…"`
  descriptions. `Validate` flags a parameter declared twice.
- Runnable demo in `examples/echo`; godoc examples in `example_test.go`.

**Fixed**

- An unexported embedded struct was dropped from body schemas, though
  `encoding/json` promotes its fields.
- Generated examples ignored the schema's bounds (`0` for `min=1`, `"example"`
  for `max=3`), so Swagger "Try it out" sent invalid requests by default.

### v0.2.0 — 2026-10-07

**New**

- **Live docs** — `goenax.Docs(reg, info)` serves the spec at `…/openapi.json`
  plus a Swagger UI / Scalar / Redoc page; mount it with the adapters' `Docs`
  method. See [Live docs](#live-docs-docs).
- `reg.Ignore(method, path)` keeps health checks, metrics and the docs routes
  out of `Coverage`.
- `goenax.NormalizePath` — router syntax (`:id`, `*file`, `{p...}`) to OpenAPI
  (`{id}`).
- `Info.ErrorType` — document your own error body instead of `{ "message" }`.
- `validate:"dive,…"` rules are applied to array items / map values.
- `PUT` and `PATCH` helpers on every adapter.
- `Registry` is safe for concurrent use.

**Fixed**

- Gin: a nested `Group` mounted routes with the prefix applied twice
  (`/v1/api/v1/…`) while the spec said `/api/v1/…`.
- Echo/Gin: `:id` path params were recorded verbatim — an invalid OpenAPI path,
  a false `Validate` error, and a `Coverage` mismatch. They are now `{id}`.
- `oneof` on a number field produced a string enum, so `ValidateResponse`
  rejected every valid value.
- Several `Error`s with one status overwrote each other; their descriptions now
  merge, and `Validate` flags a status claimed by two different responses.
- `ValidateResponse` accepted `1.5` for an integer field.
- `Lookup` was case-sensitive on the method.
- `operationId` contained `{` `}`; `Validate` now also flags `operationId`
  collisions.
- `example` tags on number/bool fields were emitted as strings.

**Breaking**

- `Schema.Enum` is `[]any` (was `[]string`) so enums carry the field's type.
- `Registry.Add` stores the method upper-cased and the path in OpenAPI form.
- `ginadapter.New` creates the prefixed group itself — pass the *unprefixed*
  router, `New(engine, reg, "/api")` (as the README always showed).

---

## License

MIT — see [LICENSE](./LICENSE).
