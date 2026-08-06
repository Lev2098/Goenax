package goenax

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

type demoContact struct {
	Email   string `json:"email"`
	Message string `json:"message"`
}

type demoCreds struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type demoSession struct {
	Token string `json:"token"`
}

func demoRegistry() *Registry {
	r := New()
	r.Add(Define("POST", "/api/contact",
		Summary("Send a contact message"),
		Description("Email-only contact form."),
		Owner("#team"),
		Tags("Contact"),
		Request[demoContact](),
		NoContent(204),
		Error(400, "validation", "invalid body"),
		Error(500, "delivery", "could not deliver your message"),
	))
	r.Add(Define("POST", "/api/auth/login",
		Summary("Exchange credentials for a session"),
		Description("Returns a session token."),
		Owner("#team"),
		Tags("Auth"),
		Request[demoCreds](),
		Response[demoSession](200),
		Error(401, "invalid", "invalid email or password"),
	))
	r.Add(Define("GET", "/api/auth/me",
		Summary("Return the authenticated user"),
		Description("Requires a bearer session."),
		Owner("#team"),
		Tags("Auth"),
		Secured(),
		Response[demoSession](200),
		Error(401, "unauthorized", "unauthorized"),
	))

	return r
}

func demoInfo() Info {
	return Info{
		Title:   "PriceYou API",
		Version: "1.0.0",
		Servers: []Server{{URL: "http://localhost:8080", Description: "Local dev"}},
	}
}

func TestOpenAPI_Structure(t *testing.T) {
	b, err := demoRegistry().OpenAPI(demoInfo())
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v, want 3.1.0", doc["openapi"])
	}

	paths, _ := doc["paths"].(map[string]any)

	contact, _ := paths["/api/contact"].(map[string]any)
	post, _ := contact["post"].(map[string]any)
	if post["requestBody"] == nil {
		t.Error("contact POST missing requestBody")
	}

	resp, _ := post["responses"].(map[string]any)
	for _, code := range []string{"204", "400", "500"} {
		if _, ok := resp[code]; !ok {
			t.Errorf("contact POST missing response %s", code)
		}
	}

	login, _ := paths["/api/auth/login"].(map[string]any)
	loginPost, _ := login["post"].(map[string]any)
	loginResp, _ := loginPost["responses"].(map[string]any)

	ok200, _ := loginResp["200"].(map[string]any)
	if ok200["content"] == nil {
		t.Error("login 200 should carry a response body schema")
	}

	if loginPost["operationId"] != "post_api_auth_login" {
		t.Errorf("operationId = %v", loginPost["operationId"])
	}
}

func TestOpenAPI_HTMLAndExtraResponse(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/api/newsletter/confirm",
		Summary("Confirm"), Description("d"), Owner("#team"), Tags("Newsletter"),
		HTML(), Query("token", true, "token"),
		Error(400, "invalid", "bad token"),
	))
	r.Add(Define("POST", "/api/auth/signup",
		Summary("Signup"), Description("d"), Owner("#team"), Tags("Auth"),
		Request[demoCreds](), Response[demoSession](201),
		Also(202, "set-password link sent"),
		Error(400, "validation", "bad"),
	))

	var doc map[string]any
	if b, err := r.OpenAPI(demoInfo()); err != nil {
		t.Fatal(err)
	} else if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}

	paths, _ := doc["paths"].(map[string]any)

	confirm, _ := paths["/api/newsletter/confirm"].(map[string]any)
	get, _ := confirm["get"].(map[string]any)
	resp200, _ := get["responses"].(map[string]any)["200"].(map[string]any)
	content, _ := resp200["content"].(map[string]any)
	if _, ok := content["text/html"]; !ok {
		t.Errorf("confirm 200 should be text/html, got %v", content)
	}

	params, _ := get["parameters"].([]any)
	if len(params) != 1 {
		t.Errorf("confirm should have 1 query param, got %d", len(params))
	}

	signup, _ := paths["/api/auth/signup"].(map[string]any)
	signupResp, _ := signup["post"].(map[string]any)["responses"].(map[string]any)
	if _, ok := signupResp["202"]; !ok {
		t.Errorf("signup should document 202, got %v", signupResp)
	}
}

func TestOpenAPI_HeaderPathAndDeps(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/api/items/{id}",
		Summary("Get item"), Description("d"), Owner("#team"), Tags("Items"),
		PathParam("id", "item id"),
		Header("X-Recaptcha-Token", false, "captcha"),
		DependsOn("POST /api/auth/login"),
		Response[demoSession](200),
		Error(404, "not_found", "no such item"),
	))

	var doc map[string]any
	if b, err := r.OpenAPI(demoInfo()); err != nil {
		t.Fatal(err)
	} else if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}

	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths["/api/items/{id}"].(map[string]any)
	op, _ := item["get"].(map[string]any)

	ins := map[string]bool{}
	for _, p := range op["parameters"].([]any) {
		ins[p.(map[string]any)["in"].(string)] = true
	}

	if !ins["path"] || !ins["header"] {
		t.Errorf("parameter locations = %v, want path+header", ins)
	}

	if op["x-dependsOn"] == nil {
		t.Error("x-dependsOn missing")
	}
}

type treeNode struct {
	Name     string      `json:"name"`
	Children []*treeNode `json:"children,omitempty"`
}

func mustDoc(t *testing.T, r *Registry) map[string]any {
	t.Helper()

	b, err := r.OpenAPI(demoInfo())
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}

	return doc
}

func TestOpenAPI_RecursiveTypeSelfRefs(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/tree",
		Summary("Tree"), Description("d"), Owner("#x"), Tags("X"),
		Response[treeNode](200), Error(404, "nf", "not found"),
	))

	doc := mustDoc(t, r) // must not hang

	schemas, _ := doc["components"].(map[string]any)["schemas"].(map[string]any)
	node, ok := schemas["treeNode"].(map[string]any)
	if !ok {
		t.Fatal("treeNode component missing")
	}

	children, _ := node["properties"].(map[string]any)["children"].(map[string]any)
	items, _ := children["items"].(map[string]any)
	if items["$ref"] != "#/components/schemas/treeNode" {
		t.Errorf("children.items should self-ref, got %v", items)
	}
}

func TestOpenAPI_SliceResponseRefsItems(t *testing.T) {
	r := New()
	r.Add(Define("GET", "/list",
		Summary("List"), Description("d"), Owner("#x"), Tags("X"),
		Response[[]demoSession](200), Error(404, "nf", "not found"),
	))

	doc := mustDoc(t, r)
	op, _ := doc["paths"].(map[string]any)["/list"].(map[string]any)["get"].(map[string]any)
	schema := op["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)

	if schema["type"] != "array" {
		t.Fatalf("want array, got %v", schema["type"])
	}

	if items, _ := schema["items"].(map[string]any); items["$ref"] != "#/components/schemas/demoSession" {
		t.Errorf("items should $ref demoSession, got %v", items)
	}
}

func TestOpenAPI_Deterministic(t *testing.T) {
	r := demoRegistry()

	a, err := r.OpenAPI(demoInfo())
	if err != nil {
		t.Fatal(err)
	}

	b, err := r.OpenAPI(demoInfo())
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(a, b) {
		t.Fatal("OpenAPI output is not deterministic across runs")
	}
}

// TestDumpDemoSpec writes the demo spec to the path in APIREG_DUMP so it can be
// validated externally (e.g. `redocly lint`). Skipped in normal test runs.
func TestDumpDemoSpec(t *testing.T) {
	out := os.Getenv("APIREG_DUMP")
	if out == "" {
		t.Skip("set APIREG_DUMP=<path> to write the demo spec")
	}

	b, err := demoRegistry().OpenAPI(demoInfo())
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(out, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

type pendingBody struct {
	Status string `json:"status"`
}

func TestOpenAPI_AlsoJSONBodyAndRespHeader(t *testing.T) {
	r := New()
	r.Add(Define("POST", "/signup",
		Summary("s"), Description("d"), Owner("#x"), Tags("X"),
		Request[demoCreds](), Response[demoSession](201),
		AlsoJSON[pendingBody](202, "email already registered"),
		RespHeader("X-Request-Id", "trace id"),
		Error(400, "v", "bad"),
	))

	resp := mustDoc(t, r)["paths"].(map[string]any)["/signup"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)

	if r202, _ := resp["202"].(map[string]any); r202["content"] == nil {
		t.Error("202 (AlsoJSON) should carry a body schema")
	}

	if r201, _ := resp["201"].(map[string]any); r201["headers"] == nil {
		t.Error("201 success should carry the declared response header")
	}
}
