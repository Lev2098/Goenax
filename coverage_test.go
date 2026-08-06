package goenax

import "testing"

func TestCoverage_DetectsUndocumentedAndUnmounted(t *testing.T) {
	reg := New()
	reg.Add(Define("GET", "/a"))
	reg.Add(Define("POST", "/b"))
	reg.Add(Define("GET", "/ghost")) // documented but never mounted

	actual := []Route{
		{Method: "GET", Path: "/a"},
		{Method: "post", Path: "/b"},    // lower-case method must still match
		{Method: "GET", Path: "/rogue"}, // on the router, not documented
	}

	undocumented, unmounted := reg.Coverage(actual)

	if len(undocumented) != 1 || undocumented[0].Path != "/rogue" {
		t.Errorf("undocumented = %v, want [/rogue]", undocumented)
	}

	if len(unmounted) != 1 || unmounted[0].Path != "/ghost" {
		t.Errorf("unmounted = %v, want [/ghost]", unmounted)
	}
}

func TestCoverage_CleanWhenAligned(t *testing.T) {
	reg := New()
	reg.Add(Define("GET", "/a"))
	reg.Add(Define("POST", "/b"))

	undocumented, unmounted := reg.Coverage([]Route{{"GET", "/a"}, {"POST", "/b"}})

	if len(undocumented) != 0 || len(unmounted) != 0 {
		t.Errorf("expected clean coverage, got undocumented=%v unmounted=%v", undocumented, unmounted)
	}
}
