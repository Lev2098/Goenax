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
| `Query` / `Header` / `PathParam` | parameters |
| `Secured()` | requires a bearer session |
| `DependsOn(ids...)` | endpoints a QA run must call first |
| `Compose(opts...)` | bundle options (e.g. a middleware's own docs) |

`Request[T]` / `Response[T]` are generic: `reflect.TypeFor[T]()` binds the schema
to the exact struct at compile time — no annotations.

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
| `validate:"oneof=a b c"` | `enum` |
| `example:"…"` | `example` (else a deterministic default) |
| `time.Time` / `[]T` / `map[string]T` | `date-time` / `array` / object with `additionalProperties` |
| named `struct` | a `$ref` into `components/schemas` (defined once, shared) |

It is cycle-safe and gives every leaf a deterministic example, so a generated
Postman collection is stable across runs.

---

## Adapters

Same shape for every framework — `New`, `Group`, `Handle`/`GET`/`POST`/`DELETE`.
Each mounts the route **and** records the contract. Only the adapter imports the
framework.

```go
echoadapter.New(e.Group("/api"), reg, "/api")   // *echo.Echo / *echo.Group
nethttpadapter.New(mux, reg, "/api")             // *http.ServeMux (Go 1.22+ patterns)
ginadapter.New(engine, reg, "/api")              // gin.IRouter
```

Middleware uses each framework's native type. `echoadapter.Routes(e)` /
`ginadapter.Routes(engine)` list the registered routes for `Coverage` (below).

---

## Keeping it honest

Goenax gives you the checks; wire them into your tests and CI.

- **`reg.Validate()`** — returns problems per endpoint: missing summary/owner/tag,
  a mutating method with no error response, a duplicate method+path, or a path
  `{token}` that doesn't match a declared `PathParam`. Fail CI on a non-empty
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

## License

MIT — see [LICENSE](./LICENSE).
