package goenax

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func docsGet(t *testing.T, h http.Handler, path string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func TestDocs_ServesSpecWithETag(t *testing.T) {
	reg := New()
	reg.Add(Define("GET", "/api/ping", Summary("ping")))
	h := Docs(reg, Info{Title: "T", Version: "1.2.3"})

	rec := docsGet(t, h, "/docs/openapi.json", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("spec = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	var doc struct {
		Info  map[string]string `json:"info"`
		Paths map[string]any    `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}

	if doc.Info["version"] != "1.2.3" || doc.Paths["/api/ping"] == nil {
		t.Errorf("spec = %s", rec.Body.String())
	}

	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}

	if rec := docsGet(t, h, "/docs/openapi.json", map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
		t.Errorf("revalidation = %d, want 304", rec.Code)
	}
}

// The spec is cached, but a route registered after the first request must
// still show up — the live registry is the source of truth.
func TestDocs_RebuildsWhenRegistryChanges(t *testing.T) {
	reg := New()
	reg.Add(Define("GET", "/a"))
	h := Docs(reg, Info{Title: "T", Version: "1"})

	first := docsGet(t, h, "/docs/openapi.json", nil)

	reg.Add(Define("GET", "/b"))

	second := docsGet(t, h, "/docs/openapi.json", nil)
	if !strings.Contains(second.Body.String(), `"/b"`) {
		t.Error("route added after the first request is missing from the spec")
	}

	if first.Header().Get("ETag") == second.Header().Get("ETag") {
		t.Error("ETag unchanged after the registry changed")
	}
}

func TestDocs_PagePerUI(t *testing.T) {
	cases := map[UI]string{
		SwaggerUI: "swagger-ui-dist@5",
		Scalar:    "@scalar/api-reference@1",
		Redoc:     "redoc@2",
	}

	for ui, asset := range cases {
		h := Docs(New(), Info{Title: `<b>"API"</b>`}, WithUI(ui))

		for _, path := range []string{"/docs", "/docs/"} {
			rec := docsGet(t, h, path, nil)
			body := rec.Body.String()

			if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("%s %s = %d %q", ui, path, rec.Code, rec.Header().Get("Content-Type"))
			}

			if !strings.Contains(body, asset) || !strings.Contains(body, `+ "openapi.json"`) {
				t.Errorf("%s page lacks its asset or spec URL:\n%s", ui, body)
			}

			if strings.Contains(body, "<b>") {
				t.Errorf("%s page does not escape the title", ui)
			}
		}
	}
}

func TestDocs_RejectsNonGet(t *testing.T) {
	rec := httptest.NewRecorder()
	Docs(New(), Info{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/docs/openapi.json", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", rec.Code)
	}
}
