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
	Enum                 []any              `json:"enum,omitempty"` // typed to match Type (string / int64 / float64)
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
		if !f.IsExported() && !embeddedStruct(f) {
			continue
		}

		name, omitempty, skip := parseJSONTag(f)
		if skip {
			continue
		}

		// Embedded struct fields without their own json tag are flattened, so we
		// build them inline (as properties) rather than as a $ref.
		if embeddedStruct(f) && (f.Tag.Get("json") == "" || !f.IsExported()) {
			embedded := b.structSchema(deref(f.Type))
			maps.Copy(s.Properties, embedded.Properties)
			s.Required = append(s.Required, embedded.Required...)

			continue
		}

		fs := b.of(f.Type)
		rules, elemRules := parseValidateTag(f)

		// A $ref node must stand alone; only enrich inline (leaf) schemas.
		if fs.Ref == "" {
			applyRules(&fs, rules)
			setExample(&fs, f.Tag.Get("example"))
			applyElemRules(&fs, elemRules)
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

// parseValidateTag reads a go-playground/validator tag into rule maps, e.g.
// `validate:"required,email,min=8"` -> {required:"", email:"", min:"8"}.
// Rules after `dive` apply to the elements of a slice/map, not the field
// itself: `validate:"min=1,dive,email"` -> rules {min:"1"}, elem {email:""}.
func parseValidateTag(f reflect.StructField) (rules, elem map[string]string) {
	rules, elem = map[string]string{}, map[string]string{}
	target, dived := rules, false

	for part := range strings.SplitSeq(f.Tag.Get("validate"), ",") {
		if part == "" {
			continue
		}

		if part == "dive" {
			if dived {
				break // nested dive (slice of slices) — not mapped
			}

			target, dived = elem, true

			continue
		}

		key, val, _ := strings.Cut(part, "=")
		target[key] = val
	}

	return rules, elem
}

// applyElemRules applies the rules after `dive` to an array's items or a map's
// values. Map-key rules (`keys … endkeys`) are not mapped.
func applyElemRules(s *Schema, elem map[string]string) {
	if len(elem) == 0 {
		return
	}

	if _, ok := elem["keys"]; ok {
		return
	}

	target := s.Items
	if target == nil {
		target = s.AdditionalProperties
	}

	if target == nil || target.Ref != "" {
		return
	}

	applyRules(target, elem)
	setExample(target, "")
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
			s.Enum = enumValues(s.Type, val)
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

// enumValues parses a `oneof` list into values of the schema's type, so an
// integer field gets `enum: [1, 2, 3]` rather than strings. A value that does not
// parse as the field's type is kept as a string (the schema is then honest about
// the mismatch instead of silently dropping it).
func enumValues(typ, list string) []any {
	words := strings.Fields(list)
	out := make([]any, 0, len(words))

	for _, w := range words {
		out = append(out, scalarValue(typ, w))
	}

	return out
}

// scalarValue parses a tag word (enum entry, example) as the schema's type, so
// an integer field gets 42 rather than "42". Unparsable words stay strings.
func scalarValue(typ, w string) any {
	switch typ {
	case typeInteger:
		if n, err := strconv.ParseInt(w, 10, 64); err == nil {
			return n
		}
	case typeNumber:
		if f, err := strconv.ParseFloat(w, 64); err == nil {
			return f
		}
	case typeBoolean:
		if b, err := strconv.ParseBool(w); err == nil {
			return b
		}
	}

	return w
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
		s.Example = scalarValue(s.Type, tagVal)
	case len(s.Enum) > 0:
		s.Example = s.Enum[0]
	case s.Type == typeString:
		ex := stringExample(s.Format)
		if s.MinLength != nil && len(ex) < *s.MinLength {
			ex += strings.Repeat("x", *s.MinLength-len(ex)) // keep the example valid + deterministic
		}

		if s.MaxLength != nil && len(ex) > *s.MaxLength {
			ex = ex[:*s.MaxLength]
		}

		s.Example = ex
	case s.Type == typeInteger:
		s.Example = int64(clampExample(s))
	case s.Type == typeNumber:
		s.Example = clampExample(s)
	case s.Type == typeBoolean:
		s.Example = false
	}
}

// clampExample is 0 moved inside the schema's minimum/maximum, so a generated
// request (Swagger "Try it out", Postman) is valid out of the box.
func clampExample(s *Schema) float64 {
	ex := 0.0
	if s.Minimum != nil && ex < *s.Minimum {
		ex = *s.Minimum
	}

	if s.Maximum != nil && ex > *s.Maximum {
		ex = *s.Maximum
	}

	return ex
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

// embeddedStruct reports whether f is an embedded struct (or *struct). Like
// encoding/json, its exported fields are promoted into the parent even when the
// embedded type itself is unexported (`type page struct{…}` embedded as page).
func embeddedStruct(f reflect.StructField) bool {
	return f.Anonymous && deref(f.Type).Kind() == reflect.Struct
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}
