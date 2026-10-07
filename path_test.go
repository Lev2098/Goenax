package goenax

import "testing"

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/api/users/{id}":            "/api/users/{id}",
		"/api/users/:id":             "/api/users/{id}",
		"/api/users/:id/posts/:post": "/api/users/{id}/posts/{post}",
		"/static/*filepath":          "/static/{filepath}",
		"/files/{path...}":           "/files/{path}",
		"/posts/{$}":                 "/posts/",
		"/echo/*":                    "/echo/*",
		"/plain":                     "/plain",
		"/a:b":                       "/a:b", // a colon mid-segment is not a param
	}

	for in, want := range cases {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegistry_LookupIsMethodCaseInsensitiveAndNormalised(t *testing.T) {
	r := New()
	r.Add(Define("post", "/users/:id"))

	if _, ok := r.Lookup("POST", "/users/{id}"); !ok {
		t.Error("Lookup(POST, /users/{id}) missed")
	}

	if _, ok := r.Lookup("post", "/users/:id"); !ok {
		t.Error("Lookup(post, /users/:id) missed")
	}
}

func TestCoverage_IgnoredRoutesAreNotUndocumented(t *testing.T) {
	r := New()
	r.Ignore("GET", "/healthz")

	und, _ := r.Coverage([]Route{{Method: "GET", Path: "/healthz"}})
	if len(und) != 0 {
		t.Errorf("undocumented = %v, want none", und)
	}
}
