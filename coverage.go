package goenax

import (
	"slices"
	"strings"
)

// Route is a method + path pair — the unit of comparison for Coverage.
type Route struct {
	Method string
	Path   string
}

// Coverage compares the routes actually registered on a router with the
// endpoints declared in the registry. A shared route table makes the two match
// by construction; Coverage catches the case a table can't — a route mounted
// (or removed) directly, out of band. It returns:
//
//   - undocumented: on the router but not in the registry (missing from the spec)
//   - unmounted:    in the registry but never registered (a dead spec entry)
//
// Pass the router's routes via an adapter helper, e.g. echoadapter.Routes(e) or
// ginadapter.Routes(engine). (net/http's ServeMux exposes no route list, so it
// has no helper — feed it a Route slice you build yourself.)
func (r *Registry) Coverage(actual []Route) (undocumented, unmounted []Route) {
	declared := make(map[Route]bool, len(r.endpoints))
	for _, e := range r.endpoints {
		declared[key(e.Method, e.Path)] = true
	}

	seen := make(map[Route]bool, len(actual))
	for _, rt := range actual {
		k := key(rt.Method, rt.Path)
		seen[k] = true

		if !declared[k] {
			undocumented = append(undocumented, k)
		}
	}

	for _, e := range r.endpoints {
		if !seen[key(e.Method, e.Path)] {
			unmounted = append(unmounted, key(e.Method, e.Path))
		}
	}

	slices.SortFunc(undocumented, routeLess)
	slices.SortFunc(unmounted, routeLess)

	return undocumented, unmounted
}

func key(method, path string) Route {
	return Route{Method: strings.ToUpper(method), Path: path}
}

func routeLess(a, b Route) int {
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}

	return strings.Compare(a.Method, b.Method)
}
