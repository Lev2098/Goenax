package goenax

import (
	"reflect"
	"strings"
)

const (
	inPath   = "path"
	inQuery  = "query"
	inHeader = "header"
)

// paramTags maps a struct tag to the parameter location it declares. It covers
// the binding tags of every supported router, so the struct a handler binds can
// be documented as-is:
//
//	Echo      param:"id"  query:"q"  header:"X-Key"
//	Gin       uri:"id"    form:"q"   header:"X-Key"
//	net/http  path:"id"   query:"q"  header:"X-Key"  (read with r.PathValue / r.URL.Query)
//
// Order matters only when a field carries two tags: the first match wins.
var paramTags = []struct{ tag, in string }{
	{"param", inPath}, {"uri", inPath}, {"path", inPath},
	{"query", inQuery}, {"form", inQuery},
	{"header", inHeader},
}

// Params declares an endpoint's path, query and header parameters from the
// struct the handler binds them into, written as goenax.Params[listQuery]().
//
//	type listQuery struct {
//	    Limit int    `query:"limit" validate:"max=100" doc:"page size"`
//	    Role  string `query:"role"  validate:"oneof=admin member"`
//	}
//
// Each field's type and validate rules become the parameter schema (limit is an
// integer with maximum 100, role an enum), and its `doc` tag the description.
// Path parameters are always required; query and header parameters only with
// validate:"required". Fields without a binding tag are ignored, embedded
// structs are flattened.
//
// Because the names come from the binding tags, renaming a parameter in the
// struct renames it in the spec — the drift that hand-written Query(...) calls
// allow is gone.
func Params[T any]() Option {
	params := paramsFor(reflect.TypeFor[T]())

	return func(e *Endpoint) { e.Params = append(e.Params, params...) }
}

func paramsFor(t reflect.Type) []Param {
	t = deref(t)
	if t.Kind() != reflect.Struct {
		return nil
	}

	b := &builder{seen: map[reflect.Type]bool{}}

	var out []Param

	for f := range t.Fields() {
		if embeddedStruct(f) {
			out = append(out, paramsFor(f.Type)...)

			continue
		}

		if !f.IsExported() {
			continue
		}

		name, in, ok := paramLocation(f)
		if !ok {
			continue
		}

		rules, elemRules := parseValidateTag(f)

		s := b.of(f.Type)
		applyRules(&s, rules)
		setExample(&s, f.Tag.Get("example"))
		applyElemRules(&s, elemRules)

		_, required := rules["required"]

		out = append(out, Param{
			Name:     name,
			In:       in,
			Required: required || in == inPath,
			Desc:     f.Tag.Get("doc"),
			Schema:   &s,
		})
	}

	return out
}

// paramLocation finds the first binding tag on a field and returns the
// parameter name (the tag value up to any comma, e.g. Gin's form:"q,default=1")
// and its location.
func paramLocation(f reflect.StructField) (name, in string, ok bool) {
	for _, pt := range paramTags {
		v, found := f.Tag.Lookup(pt.tag)
		if !found {
			continue
		}

		name, _, _ = strings.Cut(v, ",")
		if name == "" || name == "-" {
			return "", "", false
		}

		return name, pt.in, true
	}

	return "", "", false
}
