package goenax

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSchemaFor_Primitives(t *testing.T) {
	cases := []struct {
		val      any
		wantType string
	}{
		{"", "string"},
		{true, "boolean"},
		{42, "integer"},
		{3.14, "number"},
	}

	for _, c := range cases {
		got := SchemaFor(reflect.TypeOf(c.val))
		if got.Type != c.wantType {
			t.Errorf("SchemaFor(%T).Type = %q, want %q", c.val, got.Type, c.wantType)
		}
	}
}

func TestSchemaFor_TimeIsDateTimeString(t *testing.T) {
	got := SchemaFor(reflect.TypeFor[time.Time]())
	if got.Type != "string" || got.Format != "date-time" {
		t.Fatalf("time.Time -> %q/%q, want string/date-time", got.Type, got.Format)
	}
}

func TestSchemaFor_Slice(t *testing.T) {
	got := SchemaFor(reflect.TypeFor[[]string]())
	if got.Type != "array" {
		t.Fatalf("type = %q, want array", got.Type)
	}

	if got.Items == nil || got.Items.Type != "string" {
		t.Fatalf("items = %+v, want string", got.Items)
	}
}

type sampleReq struct {
	Email  string   `json:"email"`
	Name   string   `json:"name,omitempty"`
	Age    *int     `json:"age"`
	Tags   []string `json:"tags"`
	Secret string   `json:"-"`
	hidden string   //nolint:unused
}

func TestSchemaFor_StructTagsAndRequired(t *testing.T) {
	s := SchemaFor(reflect.TypeFor[sampleReq]())

	if s.Type != "object" {
		t.Fatalf("type = %q, want object", s.Type)
	}

	// json:"-" and the unexported field must not appear.
	if _, ok := s.Properties["Secret"]; ok {
		t.Error(`json:"-" field leaked into schema`)
	}

	if _, ok := s.Properties["hidden"]; ok {
		t.Error("unexported field leaked into schema")
	}

	if len(s.Properties) != 4 {
		t.Fatalf("properties = %d, want 4 (%v)", len(s.Properties), keys(s.Properties))
	}

	if s.Properties["age"].Type != "integer" {
		t.Errorf("age type = %q, want integer (pointer should deref)", s.Properties["age"].Type)
	}

	if s.Properties["tags"].Type != "array" {
		t.Errorf("tags type = %q, want array", s.Properties["tags"].Type)
	}

	// Required: email + tags. name is omitempty, age is a pointer -> optional.
	want := []string{"email", "tags"}
	if !reflect.DeepEqual(s.Required, want) {
		t.Fatalf("required = %v, want %v", s.Required, want)
	}
}

type nested struct {
	Inner sampleReq `json:"inner"`
}

func TestSchemaFor_Nested(t *testing.T) {
	s := SchemaFor(reflect.TypeFor[nested]())

	inner, ok := s.Properties["inner"]
	if !ok {
		t.Fatal("inner property missing")
	}

	if inner.Type != "object" || inner.Properties["email"] == nil {
		t.Fatalf("nested object not expanded: %+v", inner)
	}
}

func keys(m map[string]*Schema) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

type validated struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Source   string `json:"source"   validate:"omitempty,oneof=footer blog quiz"`
}

func TestSchemaFor_ValidateTags(t *testing.T) {
	s := SchemaFor(reflect.TypeFor[validated]())

	if s.Properties["email"].Format != "email" {
		t.Errorf("email format = %q, want email", s.Properties["email"].Format)
	}

	if ml := s.Properties["password"].MinLength; ml == nil || *ml != 8 {
		t.Errorf("password minLength = %v, want 8", ml)
	}

	if got := fmt.Sprint(s.Properties["source"].Enum); got != "[footer blog quiz]" {
		t.Errorf("source enum = %q", got)
	}

	// email + password are required; source is omitempty -> optional.
	if got := strings.Join(s.Required, ","); got != "email,password" {
		t.Errorf("required = %q", got)
	}
}

func TestSchemaFor_Map(t *testing.T) {
	s := SchemaFor(reflect.TypeFor[map[string]int]())
	if s.Type != "object" || s.AdditionalProperties == nil || s.AdditionalProperties.Type != "integer" {
		t.Fatalf("map schema = %+v", s)
	}
}

type node struct {
	Name     string  `json:"name"`
	Children []*node `json:"children,omitempty"`
	Parent   *node   `json:"parent,omitempty"`
}

func TestSchemaFor_CycleSafe(t *testing.T) {
	s := SchemaFor(reflect.TypeFor[node]()) // must not hang or overflow

	if s.Type != "object" || s.Properties["name"] == nil {
		t.Fatalf("node schema = %+v", s)
	}

	if s.Properties["children"].Type != "array" {
		t.Errorf("children should be an array")
	}
}

func TestSchemaFor_DiveAppliesToElements(t *testing.T) {
	type in struct {
		Emails []string          `json:"emails" validate:"min=1,dive,email"`
		Scores map[string]int    `json:"scores" validate:"dive,oneof=1 2"`
		Raw    []string          `json:"raw"    validate:"dive"`
		Keys   map[string]string `json:"keys"   validate:"dive,keys,min=2,endkeys,email"`
	}

	s := SchemaFor(reflect.TypeFor[in]())

	emails := s.Properties["emails"]
	if emails.Format != "" || emails.Items.Format != "email" {
		t.Errorf("emails: array format %q, items format %q; want \"\" and email", emails.Format, emails.Items.Format)
	}

	if got := fmt.Sprint(s.Properties["scores"].AdditionalProperties.Enum); got != "[1 2]" {
		t.Errorf("scores values enum = %s, want [1 2]", got)
	}

	if s.Properties["keys"].AdditionalProperties.Format != "" {
		t.Error("map-key rules must not be applied to values")
	}
}

func TestSchemaFor_ExampleIsTyped(t *testing.T) {
	type in struct {
		N   int     `json:"n"   example:"42"`
		F   float64 `json:"f"   example:"1.5"`
		B   bool    `json:"b"   example:"true"`
		S   string  `json:"s"   example:"hi"`
		Bad int     `json:"bad" example:"many"`
	}

	s := SchemaFor(reflect.TypeFor[in]())
	want := map[string]any{"n": int64(42), "f": 1.5, "b": true, "s": "hi", "bad": "many"}

	for name, ex := range want {
		if got := s.Properties[name].Example; got != ex {
			t.Errorf("%s example = %#v, want %#v", name, got, ex)
		}
	}
}
