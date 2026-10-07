package goenax

import (
	"strings"
	"testing"
)

type cUser struct {
	ID    string `json:"id"`
	Email string `json:"email" validate:"email"`
	Role  string `json:"role"  validate:"omitempty,oneof=admin user"`
}

type cSession struct {
	Token string `json:"token"`
	User  cUser  `json:"user"`
}

func sessionRegistry() *Registry {
	r := New()
	r.Add(Define("GET", "/me",
		Summary("me"), Description("d"), Owner("#x"), Tags("X"),
		Response[cSession](200), Error(401, "unauthorized", "no session"),
	))

	return r
}

func TestValidateResponse_GoodBodyPasses(t *testing.T) {
	body := []byte(`{"token":"t","user":{"id":"1","email":"a@b.co","role":"admin","extra":true}}`)
	if err := sessionRegistry().ValidateResponse("GET", "/me", body); err != nil {
		t.Fatalf("valid body rejected (extra fields must be allowed): %v", err)
	}
}

// The headline for hole #1: the handler forgot a declared field. The spec still
// promises it; this catches the drift.
func TestValidateResponse_CatchesMissingField(t *testing.T) {
	body := []byte(`{"token":"t"}`) // no "user"

	err := sessionRegistry().ValidateResponse("GET", "/me", body)
	if err == nil || !strings.Contains(err.Error(), "user") {
		t.Fatalf("expected a missing-user error, got %v", err)
	}
}

func TestValidateResponse_CatchesWrongType(t *testing.T) {
	body := []byte(`{"token":123,"user":{"id":"1","email":"a@b.co"}}`)

	err := sessionRegistry().ValidateResponse("GET", "/me", body)
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected a token type error, got %v", err)
	}
}

func TestValidateResponse_CatchesEnumViolation(t *testing.T) {
	body := []byte(`{"token":"t","user":{"id":"1","email":"a@b.co","role":"superadmin"}}`)

	err := sessionRegistry().ValidateResponse("GET", "/me", body)
	if err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("expected a role enum error, got %v", err)
	}
}

func TestValidateResponse_ArrayItemsChecked(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/users",
		Summary("users"), Description("d"), Owner("#x"), Tags("X"),
		Response[[]cUser](200), Error(401, "unauthorized", "no session"),
	))

	if err := r.ValidateResponse("GET", "/users", []byte(`[{"id":"1","email":"a@b.co"}]`)); err != nil {
		t.Fatalf("valid array rejected: %v", err)
	}

	err := r.ValidateResponse("GET", "/users", []byte(`[{"id":"1"}]`)) // missing email
	if err == nil || !strings.Contains(err.Error(), "[0]") {
		t.Fatalf("expected a per-item error, got %v", err)
	}
}

func TestValidateResponse_NoBodyEndpoint(t *testing.T) {
	r := New()
	r.Add(Define("POST", "/logout",
		Summary("logout"), Description("d"), Owner("#x"), Tags("X"),
		NoContent(204), Error(401, "unauthorized", "no session"),
	))

	if err := r.ValidateResponse("POST", "/logout", []byte("   ")); err != nil {
		t.Errorf("empty body should pass for a no-content endpoint: %v", err)
	}

	if err := r.ValidateResponse("POST", "/logout", []byte(`{"x":1}`)); err == nil {
		t.Error("a body on a no-content endpoint should fail")
	}
}

type cLevel struct {
	Level int     `json:"level" validate:"oneof=1 2 3"`
	Ratio float64 `json:"ratio" validate:"oneof=0.5 1.5"`
}

// Regression: a numeric oneof used to become a string enum, so every valid
// response was rejected ("2 is not one of 1, 2, 3").
func TestValidateResponse_NumericEnum(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/lvl", Response[cLevel](200)))

	if err := r.ValidateResponse("GET", "/lvl", []byte(`{"level":2,"ratio":1.5}`)); err != nil {
		t.Errorf("valid numeric enum rejected: %v", err)
	}

	if err := r.ValidateResponse("GET", "/lvl", []byte(`{"level":4,"ratio":1.5}`)); err == nil {
		t.Error("level 4 accepted, want enum error")
	}

	if err := r.ValidateResponse("GET", "/lvl", []byte(`{"level":"2","ratio":1.5}`)); err == nil {
		t.Error(`level "2" (string) accepted, want enum error`)
	}
}

func TestValidateResponse_IntegerRejectsFraction(t *testing.T) {
	type n struct {
		Count int `json:"count"`
	}

	r := New()
	r.Add(Define("GET", "/n", Response[n](200)))

	if err := r.ValidateResponse("GET", "/n", []byte(`{"count":1.5}`)); err == nil {
		t.Error("count 1.5 accepted for an integer field")
	}

	if err := r.ValidateResponse("GET", "/n", []byte(`{"count":2}`)); err != nil {
		t.Errorf("count 2 rejected: %v", err)
	}
}
