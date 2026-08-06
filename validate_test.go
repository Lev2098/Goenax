package goenax

import (
	"strings"
	"testing"
)

func TestValidate_AllGood(t *testing.T) {
	r := New()
	r.Add(Define("POST", "/api/contact",
		Summary("Send a message"),
		Description("Email-only contact form."),
		Owner("#team"),
		Tags("Contact"),
		Request[demoContact](),
		NoContent(204),
		Error(400, "validation", "invalid"),
	))

	if errs := r.Validate(); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_ReportsEverythingMissing(t *testing.T) {
	r := New()
	r.Add(Define("POST", "/x")) // no options at all

	errs := r.Validate()
	if len(errs) != 1 {
		t.Fatalf("errs = %d, want 1", len(errs))
	}

	if errs[0].Endpoint != "POST /x" {
		t.Errorf("endpoint = %q, want %q", errs[0].Endpoint, "POST /x")
	}

	want := "summary,description,owner,tags,error responses"
	if got := strings.Join(errs[0].Problems, ","); got != want {
		t.Fatalf("missing = %q, want %q", got, want)
	}
}

func TestValidate_GetDoesNotRequireErrorsOrBody(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/api/health",
		Summary("Health"),
		Description("Liveness probe."),
		Owner("#team"),
		Tags("Ops"),
	))

	if errs := r.Validate(); len(errs) != 0 {
		t.Fatalf("GET should not require errors/body, got %v", errs)
	}
}

func TestValidate_WhitespaceCountsAsBlank(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/x",
		Summary("   "),
		Description("d"),
		Owner("o"),
		Tags("t"),
	))

	errs := r.Validate()
	if len(errs) != 1 || errs[0].Problems[0] != "summary" {
		t.Fatalf("whitespace summary should be flagged, got %v", errs)
	}
}

func TestValidationError_Message(t *testing.T) {
	e := ValidationError{Endpoint: "POST /x", Problems: []string{"summary", "owner"}}

	if got := e.Error(); got != "POST /x: summary; owner" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestValidate_DuplicateRoute(t *testing.T) {
	r := New()
	base := []Option{Summary("s"), Description("d"), Owner("#x"), Tags("X"), Error(400, "c", "m")}
	r.Add(Define("POST", "/dup", base...))
	r.Add(Define("POST", "/dup", base...))

	errs := r.Validate()
	if len(errs) != 1 || len(errs[0].Problems) != 1 || errs[0].Problems[0] != "duplicate route" {
		t.Fatalf("expected one duplicate-route problem, got %v", errs)
	}
}

func TestValidate_PathParamMismatch(t *testing.T) {
	r := New()
	// path has {id}, but no PathParam declared; and a declared "extra" not in path.
	r.Add(Define("GET", "/items/{id}",
		Summary("s"), Description("d"), Owner("#x"), Tags("X"),
		PathParam("extra", "unused"),
	))

	errs := r.Validate()
	if len(errs) != 1 {
		t.Fatalf("errs = %v", errs)
	}

	joined := strings.Join(errs[0].Problems, " | ")
	if !strings.Contains(joined, "{id}") || !strings.Contains(joined, "extra") {
		t.Fatalf("expected {id} undeclared + extra-not-in-path, got %q", joined)
	}
}
