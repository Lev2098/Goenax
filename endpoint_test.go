package goenax

import (
	"reflect"
	"testing"
)

type fooRequest struct {
	Email string `json:"email"`
}

type fooResponse struct {
	Token string `json:"token"`
}

func TestDefine_DefaultStatusIs200(t *testing.T) {
	e := Define("GET", "/x")

	if e.Method != "GET" || e.Path != "/x" {
		t.Fatalf("method/path = %q %q", e.Method, e.Path)
	}

	if e.Status != 200 {
		t.Fatalf("default status = %d, want 200", e.Status)
	}
}

func TestDefine_AppliesAllOptions(t *testing.T) {
	e := Define("POST", "/contact",
		Summary("Send a message"),
		Description("Email-only contact form"),
		Owner("#team"),
		Tags("Contact"),
		Request[fooRequest](),
		NoContent(204),
		Error(400, "validation", "invalid body"),
		Error(500, "delivery", "could not send"),
		DependsOn("users.create"),
	)

	if e.Summary != "Send a message" || e.Desc != "Email-only contact form" || e.Owner != "#team" {
		t.Fatalf("metadata not applied: %+v", e)
	}

	if len(e.Tags) != 1 || e.Tags[0] != "Contact" {
		t.Fatalf("tags = %v", e.Tags)
	}

	if e.Request != reflect.TypeFor[fooRequest]() {
		t.Fatalf("request type = %v, want fooRequest", e.Request)
	}

	if e.Response != nil {
		t.Fatalf("NoContent should leave response nil, got %v", e.Response)
	}

	if e.Status != 204 {
		t.Fatalf("status = %d, want 204", e.Status)
	}

	if len(e.Errors) != 2 || e.Errors[0].Status != 400 || e.Errors[1].Code != "delivery" {
		t.Fatalf("errors = %+v", e.Errors)
	}

	if len(e.DependsOn) != 1 || e.DependsOn[0] != "users.create" {
		t.Fatalf("dependsOn = %v", e.DependsOn)
	}
}

func TestResponse_SetsTypeAndStatus(t *testing.T) {
	e := Define("POST", "/login", Response[fooResponse](200))

	if e.Response != reflect.TypeFor[fooResponse]() {
		t.Fatalf("response type = %v, want fooResponse", e.Response)
	}

	if e.Status != 200 {
		t.Fatalf("status = %d, want 200", e.Status)
	}
}

func TestTagsAndDependsOn_Append(t *testing.T) {
	e := Define("GET", "/x",
		Tags("a", "b"),
		Tags("c"),
		DependsOn("x"),
		DependsOn("y", "z"),
	)

	if got := e.Tags; len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("tags = %v", got)
	}

	if got := e.DependsOn; len(got) != 3 || got[2] != "z" {
		t.Fatalf("dependsOn = %v", got)
	}
}

func TestCompose_AppliesAllBundledOptions(t *testing.T) {
	bundle := Compose(
		Tags("Common"),
		Error(429, "rate_limited", "rate limit exceeded"),
	)

	e := Define("GET", "/x", Summary("x"), bundle)

	if len(e.Tags) != 1 || e.Tags[0] != "Common" {
		t.Errorf("tags = %v, want [Common]", e.Tags)
	}

	if len(e.Errors) != 1 || e.Errors[0].Status != 429 {
		t.Errorf("errors = %v, want one 429", e.Errors)
	}
}
