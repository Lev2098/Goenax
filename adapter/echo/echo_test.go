package echoadapter_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/Lev2098/Goenax"
	echoadapter "github.com/Lev2098/Goenax/adapter/echo"
)

type contactReq struct {
	Email   string `json:"email"`
	Message string `json:"message"`
}

func TestRouter_MountsRouteAndRecordsContract(t *testing.T) {
	e := echo.New()
	reg := goenax.New()
	py := echoadapter.New(e.Group("/api"), reg, "/api")

	called := false
	py.POST("/contact", func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusNoContent)
	}, []goenax.Option{
		goenax.Summary("Send a contact message"),
		goenax.Owner("#team"),
		goenax.Tags("Contact"),
		goenax.Request[contactReq](),
		goenax.NoContent(204),
	})

	// 1) ЖИВИЙ РОУТ: запит доходить до хендлера і повертає 204
	req := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
	if !called {
		t.Error("expected called to be true")
	}

	ep, ok := reg.Lookup("POST", "/api/contact")
	if !ok {
		t.Error("expected ep to be found")
	}
	if ep.Summary != "Send a contact message" {
		t.Errorf("expected Summary to be %s, got %s", "Send a contact message", ep.Summary)
	}
}

func TestRoutes_CoverageCatchesAdHocRoute(t *testing.T) {
	e := echo.New()
	reg := goenax.New()
	py := echoadapter.New(e.Group("/api"), reg, "/api")

	noop := func(echo.Context) error { return nil }
	py.GET("/ok", noop, []goenax.Option{goenax.Summary("ok"), goenax.Owner("#x"), goenax.Tags("X")})

	// a route mounted directly on Echo, bypassing goenax — should be caught.
	e.GET("/api/rogue", noop)

	undocumented, unmounted := reg.Coverage(echoadapter.Routes(e))

	if len(unmounted) != 0 {
		t.Errorf("unmounted = %v, want none", unmounted)
	}

	if len(undocumented) != 1 || undocumented[0].Path != "/api/rogue" {
		t.Fatalf("undocumented = %v, want [/api/rogue]", undocumented)
	}
}

// Regression: Echo's ":id" used to be recorded verbatim, so the spec had an
// invalid path and Validate reported the declared PathParam as missing.
func TestRouter_ColonParamRecordedInOpenAPIForm(t *testing.T) {
	e := echo.New()
	reg := goenax.New()
	api := echoadapter.New(e.Group("/api"), reg, "/api")

	api.GET("/users/:id", func(c echo.Context) error { return c.String(http.StatusOK, c.Param("id")) },
		[]goenax.Option{
			goenax.Summary("s"), goenax.Description("d"), goenax.Owner("#o"), goenax.Tags("T"),
			goenax.PathParam("id", "user id"),
		})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users/7", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/users/7 = %d, want 200", rec.Code)
	}

	if errs := reg.Validate(); len(errs) != 0 {
		t.Errorf("Validate() = %v, want none", errs)
	}

	und, unm := reg.Coverage(echoadapter.Routes(e))
	if len(und) != 0 || len(unm) != 0 {
		t.Errorf("coverage undocumented=%v unmounted=%v, want both empty", und, unm)
	}
}

func TestRouter_DocsServesSpecAndStaysOutOfIt(t *testing.T) {
	e := echo.New()
	reg := goenax.New()
	api := echoadapter.New(e.Group("/api"), reg, "/api")

	api.GET("/ping", func(c echo.Context) error { return c.NoContent(http.StatusOK) }, nil)
	api.Docs("/docs", goenax.Docs(reg, goenax.Info{Title: "T", Version: "1"}))

	for _, p := range []string{"/api/docs", "/api/docs/", "/api/docs/openapi.json"} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", p, rec.Code)
		}
	}

	if n := len(reg.Endpoints()); n != 1 {
		t.Errorf("registry has %d endpoints, want 1 (docs must not be recorded)", n)
	}

	und, unm := reg.Coverage(echoadapter.Routes(e))
	if len(und) != 0 || len(unm) != 0 {
		t.Errorf("coverage undocumented=%v unmounted=%v, want both empty", und, unm)
	}
}
