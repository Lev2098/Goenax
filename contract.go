package goenax

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	errNoEndpoint     = errors.New("endpoint not in the registry")
	errUnexpectedBody = errors.New("endpoint declares no response body")
	errType           = errors.New("type mismatch")
	errRequired       = errors.New("missing required property")
	errEnum           = errors.New("value not in enum")
)

// ValidateResponse checks that a JSON response body conforms to the schema of an
// endpoint's declared Response type. It closes the gap between what an endpoint
// *promises* (Response[T]) and what the handler actually returns: call it from a
// handler test on the real response, and a shape that has drifted from the
// contract fails the test.
//
//	rec := httptest.NewRecorder()
//	router.ServeHTTP(rec, req)
//	if err := reg.ValidateResponse("POST", "/api/login", rec.Body.Bytes()); err != nil {
//	    t.Fatalf("response breaks its contract: %v", err)
//	}
//
// It checks the declared shape (types, required properties, enums) and allows
// unknown extra properties, so a forward-compatible addition doesn't fail a test.
func (r *Registry) ValidateResponse(method, path string, body []byte) error {
	e, ok := r.Lookup(method, path)
	if !ok {
		return fmt.Errorf("goenax: %s %s: %w", method, path, errNoEndpoint)
	}

	if e.Response == nil {
		if len(bytes.TrimSpace(body)) == 0 {
			return nil
		}

		return fmt.Errorf("goenax: %s %s (%d bytes): %w", method, path, len(body), errUnexpectedBody)
	}

	var v any

	err := json.Unmarshal(body, &v)
	if err != nil {
		return fmt.Errorf("goenax: response is not valid JSON: %w", err)
	}

	return checkValue(SchemaFor(e.Response), v, "$")
}

func checkValue(s Schema, v any, path string) error {
	if v == nil { // JSON null — treat as absent rather than a type error
		return nil
	}

	if len(s.Enum) > 0 {
		if str, ok := v.(string); !ok || !slices.Contains(s.Enum, str) {
			return fmt.Errorf("%s: %v is not one of %s: %w", path, v, strings.Join(s.Enum, ", "), errEnum)
		}

		return nil
	}

	switch s.Type {
	case typeObject:
		return checkObject(s, v, path)
	case typeArray:
		return checkArray(s, v, path)
	case typeString:
		return wantJSON[string](v, path, "string")
	case typeBoolean:
		return wantJSON[bool](v, path, "boolean")
	case typeInteger, typeNumber:
		return wantJSON[float64](v, path, "number") // JSON numbers decode to float64
	default:
		return nil // empty schema means "any"
	}
}

func checkObject(s Schema, v any, path string) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: want object, got %T: %w", path, v, errType)
	}

	for _, req := range s.Required {
		if _, present := m[req]; !present {
			return fmt.Errorf("%s: property %q: %w", path, req, errRequired)
		}
	}

	for name, ps := range s.Properties {
		val, present := m[name]
		if !present {
			continue
		}

		err := property(*ps, val, path, name)
		if err != nil {
			return err
		}
	}

	return checkAdditional(s, m, path)
}

func checkAdditional(s Schema, m map[string]any, path string) error {
	if s.AdditionalProperties == nil {
		return nil
	}

	for name, val := range m {
		if _, declared := s.Properties[name]; declared {
			continue
		}

		err := property(*s.AdditionalProperties, val, path, name)
		if err != nil {
			return err
		}
	}

	return nil
}

func property(s Schema, val any, path, name string) error {
	return checkValue(s, val, path+"."+name)
}

func checkArray(s Schema, v any, path string) error {
	arr, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%s: want array, got %T: %w", path, v, errType)
	}

	if s.Items == nil {
		return nil
	}

	for i, item := range arr {
		err := checkValue(*s.Items, item, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return err
		}
	}

	return nil
}

func wantJSON[T any](v any, path, name string) error {
	if _, ok := v.(T); !ok {
		return fmt.Errorf("%s: want %s, got %T: %w", path, name, v, errType)
	}

	return nil
}
