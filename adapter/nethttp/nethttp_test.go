package nethttpadapter_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Lev2098/Goenax"
	nethttpadapter "github.com/Lev2098/Goenax/adapter/nethttp"
)

type contactReq struct {
	Email   string `json:"email"`
	Message string `json:"message"`
}

func TestRouter_MountsRouteAndRecordsContract(t *testing.T) {
	mux := http.NewServeMux()
	reg := goenax.New()
	py := nethttpadapter.New(mux, reg, "/api")

	var mwRan, called bool

	py.POST("/contact", func(w http.ResponseWriter, _ *http.Request) {
		called = true

		w.WriteHeader(http.StatusNoContent)
	}, []goenax.Option{
		goenax.Summary("Send a contact message"),
		goenax.Owner("#team"),
		goenax.Tags("Contact"),
		goenax.Request[contactReq](),
		goenax.NoContent(http.StatusNoContent),
	}, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mwRan = true

			next.ServeHTTP(w, r)
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

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
