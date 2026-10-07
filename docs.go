package goenax

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"net/http"
	"strings"
	"sync"
)

// SpecFile is the name the spec is served under, relative to the docs page:
// a Docs handler mounted at /docs serves the spec at /docs/openapi.json.
const SpecFile = "openapi.json"

// UI selects the interactive viewer the docs page renders.
type UI string

const (
	SwaggerUI UI = "swagger" // Swagger UI — "Try it out" on every operation (default)
	Scalar    UI = "scalar"  // Scalar — modern reference with a built-in client
	Redoc     UI = "redoc"   // Redoc — read-only, three-panel reference
)

// DocsOption customises a Docs handler.
type DocsOption func(*docsConfig)

type docsConfig struct {
	ui UI
}

// WithUI picks the viewer the docs page renders (default SwaggerUI).
func WithUI(ui UI) DocsOption { return func(c *docsConfig) { c.ui = ui } }

// Docs serves the registry as live documentation — the FastAPI /docs idea: the
// running service is the source of truth for its own contract.
//
// The handler answers two requests:
//
//	GET …/openapi.json  the OpenAPI document, built from the registry
//	GET …               an HTML page rendering that document in the chosen UI
//
// Mount it with an adapter's Docs method (which also keeps the docs routes out
// of the spec and of Coverage), or by hand on any router.
//
// The spec is built lazily and cached until the registry changes, and is served
// with an ETag so clients revalidate cheaply. Leave info.Servers empty to make
// "Try it out" call whichever host served the page — the same binary then works
// on every environment without configuration.
//
// The UI assets load from the jsDelivr CDN (as FastAPI does). The page needs no
// server-side knowledge of its own URL: it resolves openapi.json relative to the
// address the browser is on, so a path-rewriting reverse proxy keeps working.
//
// Exposing the spec reveals every endpoint; guard it in production (auth
// middleware, internal network only, or simply don't mount it).
func Docs(reg *Registry, info Info, opts ...DocsOption) http.Handler {
	cfg := docsConfig{ui: SwaggerUI}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &docsHandler{reg: reg, info: info, ui: cfg.ui}
}

type docsHandler struct {
	reg  *Registry
	info Info
	ui   UI

	mu      sync.Mutex
	version uint64
	built   bool
	spec    []byte
	etag    string
}

func (h *docsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)

		return
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")

	if strings.HasSuffix(r.URL.Path, "/"+SpecFile) {
		h.serveSpec(w, r)

		return
	}

	h.servePage(w)
}

func (h *docsHandler) serveSpec(w http.ResponseWriter, r *http.Request) {
	spec, etag, err := h.current()
	if err != nil {
		http.Error(w, "goenax: building the OpenAPI document failed", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", mimeJSON)
	w.Header().Set("Cache-Control", "no-cache") // always revalidate: a deploy changes the contract
	w.Header().Set("ETag", etag)

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)

		return
	}

	_, _ = w.Write(spec)
}

// current returns the cached spec, rebuilding it when the registry has changed
// since the last build.
func (h *docsHandler) current() (spec []byte, etag string, err error) {
	version := h.reg.currentVersion()

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.built && h.version == version {
		return h.spec, h.etag, nil
	}

	spec, err = h.reg.OpenAPI(h.info)
	if err != nil {
		return nil, "", err
	}

	sum := sha256.Sum256(spec)
	h.spec, h.etag, h.version, h.built = spec, `"`+hex.EncodeToString(sum[:8])+`"`, version, true

	return h.spec, h.etag, nil
}

func (h *docsHandler) servePage(w http.ResponseWriter) {
	tmpl, ok := pages[h.ui]
	if !ok {
		tmpl = pages[SwaggerUI]
	}

	var buf bytes.Buffer

	err := tmpl.Execute(&buf, struct{ Title, Spec string }{Title: h.info.Title, Spec: SpecFile})
	if err != nil {
		http.Error(w, "goenax: rendering the docs page failed", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = buf.WriteTo(w)
}

// specURLScript resolves openapi.json against the page's own path, whether it
// was opened as /docs or /docs/ (a plain relative URL would break on the former).
const specURLScript = `const specURL = location.pathname.replace(/\/?$/, "/") + {{.Spec}};`

const pageHead = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
`

var pages = map[UI]*template.Template{
	SwaggerUI: template.Must(template.New("swagger").Parse(pageHead + `<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
` + specURLScript + `
SwaggerUIBundle({ url: specURL, dom_id: "#swagger-ui", deepLinking: true, persistAuthorization: true });
</script>
</body>
</html>
`)),
	Scalar: template.Must(template.New("scalar").Parse(pageHead + `</head>
<body>
<div id="app"></div>
<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1"></script>
<script>
` + specURLScript + `
Scalar.createApiReference("#app", { url: specURL, showDeveloperTools: "never", agent: { disabled: true }, mcp: { disabled: true } });
</script>
</body>
</html>
`)),
	Redoc: template.Must(template.New("redoc").Parse(pageHead + `</head>
<body>
<div id="redoc"></div>
<script src="https://cdn.jsdelivr.net/npm/redoc@2/bundles/redoc.standalone.js"></script>
<script>
` + specURLScript + `
Redoc.init(specURL, {}, document.getElementById("redoc"));
</script>
</body>
</html>
`)),
}
