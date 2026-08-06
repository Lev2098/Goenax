package goenax

import (
	"net/http"
	"sort"
	"strings"
)

// ValidationError lists everything wrong with one endpoint — missing metadata,
// a duplicate route, or a path/parameter mismatch.
type ValidationError struct {
	Endpoint string   // "POST /api/contact"
	Problems []string // ["description", "owner", "duplicate route"]
}

func (e ValidationError) Error() string {
	return e.Endpoint + ": " + strings.Join(e.Problems, "; ")
}

// Validate checks every endpoint against the policy and structural rules, and
// returns one ValidationError per offending endpoint (nil = all good).
//
// Policy (kept small, not bureaucratic): every endpoint needs a summary,
// description, owner and tag; mutating methods need at least one error response.
// Structural: no two endpoints share a method+path, and path {tokens} match the
// declared PathParam names. Request bodies and the success status are not
// checked (action POSTs carry no body; the status is auto-defaulted).
func (r *Registry) Validate() []ValidationError {
	var errs []ValidationError

	seen := map[string]bool{}

	for _, e := range r.endpoints {
		problems := missingMetadata(e)
		problems = append(problems, pathParamProblems(e)...)

		route := strings.ToUpper(e.Method) + " " + e.Path
		if seen[route] {
			problems = append(problems, "duplicate route")
		}

		seen[route] = true

		if len(problems) > 0 {
			errs = append(errs, ValidationError{Endpoint: e.Method + " " + e.Path, Problems: problems})
		}
	}

	return errs
}

func missingMetadata(e Endpoint) []string {
	var missing []string

	if blank(e.Summary) {
		missing = append(missing, "summary")
	}

	if blank(e.Desc) {
		missing = append(missing, "description")
	}

	if blank(e.Owner) {
		missing = append(missing, "owner")
	}

	if len(e.Tags) == 0 {
		missing = append(missing, "tags")
	}

	if mutating(e.Method) && len(e.Errors) == 0 {
		missing = append(missing, "error responses")
	}

	return missing
}

// pathParamProblems flags path {tokens} without a declared PathParam and declared
// path params that never appear in the path.
func pathParamProblems(e Endpoint) []string {
	declared := map[string]bool{}

	for _, p := range e.Params {
		if p.In == "path" {
			declared[p.Name] = true
		}
	}

	var problems []string

	for _, tok := range pathTokens(e.Path) {
		if declared[tok] {
			delete(declared, tok)

			continue
		}

		problems = append(problems, "undeclared path parameter {"+tok+"}")
	}

	leftover := make([]string, 0, len(declared))
	for name := range declared {
		leftover = append(leftover, name)
	}

	sort.Strings(leftover)

	for _, name := range leftover {
		problems = append(problems, "path parameter "+name+" not in the path")
	}

	return problems
}

func pathTokens(path string) []string {
	var out []string

	for {
		_, after, found := strings.Cut(path, "{")
		if !found {
			break
		}

		tok, rest, closed := strings.Cut(after, "}")
		if !closed {
			break
		}

		out = append(out, tok)
		path = rest
	}

	return out
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
