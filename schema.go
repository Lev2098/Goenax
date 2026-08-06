package goenax

import (
	"encoding/json"
	"maps"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	typeObject  = "object"
	typeString  = "string"
	typeInteger = "integer"
	typeNumber  = "number"
	typeBoolean = "boolean"
	typeArray   = "array"
)

const (
	formatEmail    = "email"
	formatUUID     = "uuid"
	formatURI      = "uri"
	formatDateTime = "date-time"
)

// Schema is a JSON Schema node — the shared shape behind an endpoint's request
// and response bodies. It maps directly onto the schema object OpenAPI 3.1 uses.
// When Ref is set the node is only a reference into components/schemas.
type Schema struct {
	Ref                  string             `json:"$ref,omitempty"`
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Enum                 []string           `json:"enum,omitempty"`
	Example              any                `json:"example,omitempty"`
	MinLength            *int               `json:"minLength,omitempty"`
	MaxLength            *int               `json:"maxLength,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Required             []string           `json:"required,omitempty"`
}

// SchemaFor builds an inline Schema from a Go type (no $ref). It is cycle-safe:
// a self-referential type stops at a bare object rather than recursing forever.
func SchemaFor(t reflect.Type) Schema {
	b := &builder{seen: map[reflect.Type]bool{}}

	return b.of(t)
}

// builder turns Go types into schemas. When schemas is non-nil, named struct
// types are emitted once into it and referenced by $ref (deduped, cycle-proof);
// when nil, everything is inlined and the seen set guards cycles.
type builder struct {
	schemas map[string]*Schema
	names   map[reflect.Type]string // ref mode: type -> component name (collision-safe)
	seen    map[reflect.Type]bool
}

func (b *builder) of(t reflect.Type) Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t {
	case reflect.TypeFor[time.Time]():
		return Schema{Type: typeString, Format: formatDateTime}
	case reflect.TypeFor[json.RawMessage]():
		return Schema{} // arbitrary JSON
	}

	switch t.Kind() { //nolint:exhaustive // the default arm handles the remaining kinds
	case reflect.String:
		return Schema{Type: typeString}
	case reflect.Bool:
		return Schema{Type: typeBoolean}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Schema{Type: typeInteger}
	case reflect.Float32, reflect.Float64:
		return Schema{Type: typeNumber}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 { // []byte
			return Schema{Type: typeString, Format: "byte"}
		}

		item := b.of(t.Elem())

		return Schema{Type: typeArray, Items: &item}
	case reflect.Map:
		value := b.of(t.Elem())

		return Schema{Type: typeObject, AdditionalProperties: &value}
	case reflect.Struct:
		if b.schemas != nil && t.Name() != "" {
			return Schema{Ref: "#/components/schemas/" + b.ensure(t)}
		}

		return b.structSchema(t)
	default:
		// Interfaces, channels, funcs — an empty schema means "any".
		return Schema{}
	}
}

// ensure builds the component for a named struct once and returns its component
// name. It placeholders first so a self-reference resolves to the $ref instead
// of recursing, and disambiguates two distinct types that share a Go name.
func (b *builder) ensure(t reflect.Type) string {
	if name, ok := b.names[t]; ok {
		return name
	}

	name := t.Name()
	if _, taken := b.schemas[name]; taken {
		name = pkgPrefix(t) + name // a different type already owns this name
	}

	b.names[t] = name
	b.schemas[name] = &Schema{}
	*b.schemas[name] = b.structSchema(t)

	return name
}

// pkgPrefix disambiguates a component name with the type's package, e.g. a
// `model.User` and an `http.User` become "User" and "http_User".
func pkgPrefix(t reflect.Type) string {
	p := t.PkgPath()
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}

	if p == "" {
		return ""
	}

	return p + "_"
}

func (b *builder) structSchema(t reflect.Type) Schema {
	// In inline mode, guard cycles by path; in ref mode, ensure() already did.
	if b.schemas == nil {
		if b.seen[t] {
			return Schema{Type: typeObject}
		}

		b.seen[t] = true
		defer delete(b.seen, t)
	}

	s := Schema{Type: typeObject, Properties: map[string]*Schema{}}

	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}

		name, omitempty, skip := parseJSONTag(f)
		if skip {
			continue
		}

		// Embedded struct fields without their own json tag are flattened, so we
		// build them inline (as properties) rather than as a $ref.
		if f.Anonymous && deref(f.Type).Kind() == reflect.Struct && f.Tag.Get("json") == "" {
			embedded := b.structSchema(deref(f.Type))
			maps.Copy(s.Properties, embedded.Properties)
			s.Required = append(s.Required, embedded.Required...)

			continue
		}

		fs := b.of(f.Type)
		rules := parseValidateTag(f)

		// A $ref node must stand alone; only enrich inline (leaf) schemas.
		if fs.Ref == "" {
			applyRules(&fs, rules)
			setExample(&fs, f.Tag.Get("example"))
		}

		s.Properties[name] = &fs

		if isRequired(f, omitempty, rules) {
			s.Required = append(s.Required, name)
		}
	}

	sort.Strings(s.Required)

	if len(s.Required) == 0 {
		s.Required = nil
	}

	return s
}

// isRequired combines the json and validate tags: an explicit validate rule wins,
// otherwise a field is required unless it is omitempty or a pointer.
func isRequired(f reflect.StructField, omitempty bool, rules map[string]string) bool {
	if _, ok := rules["required"]; ok {
		return true
	}

	if _, ok := rules["omitempty"]; ok {
		return false
	}

	return !omitempty && f.Type.Kind() != reflect.Pointer
}

// parseValidateTag reads a go-playground/validator tag into a rule map, e.g.
// `validate:"required,email,min=8"` -> {required:"", email:"", min:"8"}.
func parseValidateTag(f reflect.StructField) map[string]string {
	rules := map[string]string{}

	for part := range strings.SplitSeq(f.Tag.Get("validate"), ",") {
		if part == "" {
			continue
		}

		key, val, _ := strings.Cut(part, "=")
		rules[key] = val
	}

	return rules
}

func applyRules(s *Schema, rules map[string]string) {
	for key, val := range rules {
		switch key {
		case formatEmail:
			s.Format = formatEmail
		case formatUUID, "uuid4":
			s.Format = formatUUID
		case "url", formatURI:
			s.Format = formatURI
		case "oneof":
			s.Enum = strings.Fields(val)
		case "min":
			applyBound(s, val, true)
		case "max":
			applyBound(s, val, false)
		case "len":
			applyBound(s, val, true)
			applyBound(s, val, false)
		}
	}
}

// applyBound maps a min/max to minLength/maxLength for strings and
// minimum/maximum for numbers.
func applyBound(s *Schema, val string, lower bool) {
	n, err := strconv.Atoi(val)
	if err != nil {
		return
	}

	switch s.Type {
	case typeString:
		if lower {
			s.MinLength = &n
		} else {
			s.MaxLength = &n
		}
	case typeInteger, typeNumber:
		f := float64(n)
		if lower {
			s.Minimum = &f
		} else {
			s.Maximum = &f
		}
	}
}

// setExample gives a leaf schema a deterministic example — an explicit `example`
// tag wins, then an enum's first value, then a type/format default. Deterministic
// examples keep the generated Postman collection stable (its generator otherwise
// fakes random values for typed fields).
func setExample(s *Schema, tagVal string) {
	switch {
	case tagVal != "":
		s.Example = tagVal
	case len(s.Enum) > 0:
		s.Example = s.Enum[0]
	case s.Type == typeString:
		ex := stringExample(s.Format)
		if s.MinLength != nil && len(ex) < *s.MinLength {
			ex += strings.Repeat("x", *s.MinLength-len(ex)) // keep the example valid + deterministic
		}

		s.Example = ex
	case s.Type == typeInteger || s.Type == typeNumber:
		s.Example = 0
	case s.Type == typeBoolean:
		s.Example = false
	}
}

func stringExample(format string) string {
	switch format {
	case formatEmail:
		return "user@example.com"
	case formatUUID:
		return "00000000-0000-0000-0000-000000000000"
	case formatURI:
		return "https://example.com"
	case formatDateTime:
		return "2026-01-01T00:00:00Z"
	case "byte":
		return "ZXhhbXBsZQ=="
	default:
		return "example"
	}
}

// parseJSONTag reads a field's json tag: the property name, whether it is
// omitempty, and whether it is skipped ("-").
func parseJSONTag(f reflect.StructField) (name string, omitempty, skip bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}

	parts := strings.Split(tag, ",")

	name = parts[0]
	if name == "" {
		name = f.Name
	}

	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitempty = true
		}
	}

	return name, omitempty, false
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}
