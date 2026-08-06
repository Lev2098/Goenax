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
