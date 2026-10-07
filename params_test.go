package goenax

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type pagination struct {
	Limit  int `query:"limit"  validate:"min=1,max=100" doc:"page size"`
	Offset int `query:"offset"`
}

type listUsersParams struct {
	pagination

	OrgID   string   `param:"org"     validate:"uuid"`
	Role    string   `query:"role"    validate:"required,oneof=admin member"`
	Tags    []string `query:"tag"     validate:"dive,max=20"`
	APIKey  string   `header:"X-Api-Key"`
	Ignored string   // no binding tag
	Skipped string   `query:"-"`
	hidden  string   `query:"hidden"` //nolint:unused // proves unexported fields are skipped
}

func paramByName(t *testing.T, ps []Param, name string) Param {
	t.Helper()

	for _, p := range ps {
		if p.Name == name {
			return p
		}
	}

	t.Fatalf("param %q not declared; got %+v", name, ps)

	return Param{}
}

func TestParams_EchoTags(t *testing.T) {
	ps := Define("GET", "/orgs/{org}/users", Params[listUsersParams]()).Params

	if len(ps) != 6 {
		t.Fatalf("got %d params, want 6 (limit offset org role tag X-Api-Key): %+v", len(ps), ps)
	}

	limit := paramByName(t, ps, "limit")
	if limit.In != "query" || limit.Required || limit.Desc != "page size" ||
		limit.Schema.Type != "integer" || *limit.Schema.Minimum != 1 || *limit.Schema.Maximum != 100 {
		t.Errorf("limit = %+v schema %+v", limit, limit.Schema)
	}

	org := paramByName(t, ps, "org")
	if org.In != "path" || !org.Required || org.Schema.Format != "uuid" {
		t.Errorf("org = %+v (path params are always required)", org)
	}

	role := paramByName(t, ps, "role")
	if !role.Required || fmt.Sprint(role.Schema.Enum) != "[admin member]" {
		t.Errorf("role = %+v enum %v", role, role.Schema.Enum)
	}

	tag := paramByName(t, ps, "tag")
	if tag.Schema.Type != "array" || *tag.Schema.Items.MaxLength != 20 {
		t.Errorf("tag schema = %+v", tag.Schema)
	}

	if key := paramByName(t, ps, "X-Api-Key"); key.In != "header" || key.Required {
		t.Errorf("X-Api-Key = %+v", key)
	}
}

func TestParams_GinAndNetHTTPTags(t *testing.T) {
	type ginParams struct {
		ID   int    `uri:"id"`
		Q    string `form:"q,default=x"`
		Slug string `path:"slug"`
	}

	ps := Define("GET", "/x", Params[ginParams]()).Params

	if p := paramByName(t, ps, "id"); p.In != "path" || p.Schema.Type != "integer" {
		t.Errorf("id = %+v", p)
	}

	if p := paramByName(t, ps, "q"); p.In != "query" {
		t.Errorf("q = %+v (form:\"q,default=x\" names q)", p)
	}

	if p := paramByName(t, ps, "slug"); p.In != "path" {
		t.Errorf("slug = %+v", p)
	}
}

func TestParams_NonStructDeclaresNothing(t *testing.T) {
	if ps := Define("GET", "/x", Params[int]()).Params; len(ps) != 0 {
		t.Errorf("Params[int] = %+v, want none", ps)
	}
}

func TestParams_InOpenAPIAndValidate(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/orgs/:org/users",
		Summary("s"), Description("d"), Owner("#o"), Tags("T"),
		Params[listUsersParams]()))

	if errs := r.Validate(); len(errs) != 0 {
		t.Errorf("Validate() = %v — the path param from Params must satisfy the {org} token", errs)
	}

	out, err := r.OpenAPI(Info{Title: "t", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name   string         `json:"name"`
				Schema map[string]any `json:"schema"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}

	found := false

	for _, p := range doc.Paths["/orgs/{org}/users"]["get"].Parameters {
		if p.Name == "limit" {
			found = true

			if p.Schema["type"] != "integer" || p.Schema["maximum"] != 100.0 {
				t.Errorf("limit schema in spec = %v", p.Schema)
			}
		}
	}

	if !found {
		t.Error("limit (from the embedded pagination struct) is missing from the spec")
	}
}

func TestValidate_DuplicateParam(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/x",
		Summary("s"), Description("d"), Owner("#o"), Tags("T"),
		Params[pagination](), Query("limit", false, "again"),
		Header("X-Key", false, ""), Header("x-key", false, "")))

	errs := r.Validate()
	if len(errs) != 1 || strings.Join(errs[0].Problems, "; ") !=
		"duplicate query parameter limit; duplicate header parameter x-key" {
		t.Errorf("Validate() = %v", errs)
	}
}
