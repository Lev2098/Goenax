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
