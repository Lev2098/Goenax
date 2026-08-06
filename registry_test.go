package goenax

import "testing"

func TestRegistry_AddAndLookup(t *testing.T) {
	r := New()
	r.Add(Define("POST", "/contact", Summary("contact")))
	r.Add(Define("GET", "/health", Summary("health")))

	got, ok := r.Lookup("GET", "/health")
	if !ok {
		t.Fatal("Lookup(GET, /health) not found")
	}

	if got.Summary != "health" {
		t.Fatalf("summary = %q, want health", got.Summary)
	}
}

func TestRegistry_LookupMiss(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/a"))

	if _, ok := r.Lookup("POST", "/a"); ok {
		t.Fatal("method mismatch should miss")
	}

	if _, ok := r.Lookup("GET", "/b"); ok {
		t.Fatal("path mismatch should miss")
	}
}

// Endpoints must hand back a copy — mutating or appending to the result must not
// touch the registry's internal state.
func TestRegistry_EndpointsReturnsCopy(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/a", Owner("#team")))

	eps := r.Endpoints()
	eps[0].Owner = "#hacked"
	eps = append(eps, Define("GET", "/b"))
	_ = eps

	got, _ := r.Lookup("GET", "/a")
	if got.Owner != "#team" {
		t.Fatalf("internal state mutated via Endpoints(): owner = %q", got.Owner)
	}

	if n := len(r.Endpoints()); n != 1 {
		t.Fatalf("append to returned slice leaked into registry: len = %d, want 1", n)
	}
}

func TestNew_IsEmpty(t *testing.T) {
	if n := len(New().Endpoints()); n != 0 {
		t.Fatalf("new registry len = %d, want 0", n)
	}
}
