package ginadapter_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Lev2098/Goenax"
	ginadapter "github.com/Lev2098/Goenax/adapter/gin"
)

type contactReq struct {
	Email   string `json:"email"`
	Message string `json:"message"`
}

func TestRouter_MountsRouteAndRecordsContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	reg := goenax.New()
	py := ginadapter.New(engine, reg, "/api")

	var mwRan, called bool

	py.POST("/contact", func(c *gin.Context) {
		called = true

		c.Status(http.StatusNoContent)
	}, []goenax.Option{
		goenax.Summary("Send a contact message"),
		goenax.Owner("#team"),
		goenax.Tags("Contact"),
		goenax.Request[contactReq](),
		goenax.NoContent(http.StatusNoContent),
	}, func(c *gin.Context) {
		mwRan = true

		c.Next()
	})

	req := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if !called || !mwRan {
		t.Errorf("called=%v mwRan=%v, want both true", called, mwRan)
	}

	ep, ok := reg.Lookup("POST", "/api/contact")
	if !ok {
		t.Fatal("endpoint not recorded in registry")
	}

	if ep.Summary != "Send a contact message" {
		t.Errorf("summary = %q, want %q", ep.Summary, "Send a contact message")
	}
}

// Regression: a nested Group used to mount at /v1/api/v1/... (prefix applied
// twice) while recording /api/v1/..., so the spec described a route that did
// not exist.
func TestRouter_NestedGroupMountsWhereItRecords(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	reg := goenax.New()
	v1 := ginadapter.New(engine, reg, "/api").Group("/v1")

	v1.GET("/users/:id", func(c *gin.Context) { c.String(http.StatusOK, c.Param("id")) },
		[]goenax.Option{goenax.PathParam("id", "user id")})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/7", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "7" {
		t.Fatalf("GET /api/v1/users/7 = %d %q, want 200 \"7\"", rec.Code, rec.Body.String())
	}

	if _, ok := reg.Lookup("GET", "/api/v1/users/{id}"); !ok {
		t.Fatalf("registry = %+v, want GET /api/v1/users/{id}", reg.Endpoints())
	}

	und, unm := reg.Coverage(ginadapter.Routes(engine))
	if len(und) != 0 || len(unm) != 0 {
		t.Errorf("coverage undocumented=%v unmounted=%v, want both empty", und, unm)
	}
}

func TestRouter_DocsServesSpecAndStaysOutOfIt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	reg := goenax.New()
	api := ginadapter.New(engine, reg, "/api")

	var guarded bool

	api.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) }, nil)
	api.Docs("/docs", goenax.Docs(reg, goenax.Info{Title: "T", Version: "1"}), func(c *gin.Context) {
		guarded = true

		c.Next()
	})

	for _, p := range []string{"/api/docs", "/api/docs/openapi.json"} {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", p, rec.Code)
		}
	}

	if !guarded {
		t.Error("docs middleware did not run")
	}

	und, unm := reg.Coverage(ginadapter.Routes(engine))
	if len(und) != 0 || len(unm) != 0 {
		t.Errorf("coverage undocumented=%v unmounted=%v, want both empty", und, unm)
	}
}
