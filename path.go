package goenax

import "strings"

// NormalizePath rewrites a router-native path template into the OpenAPI form the
// registry stores, so a route declared in its framework's own syntax documents
// (and validates) correctly:
//
//	/users/:id           -> /users/{id}       (Echo, Gin)
//	/files/*filepath     -> /files/{filepath} (Gin catch-all)
//	/files/{path...}     -> /files/{path}     (net/http wildcard)
//	/posts/{$}           -> /posts/           (net/http exact-match marker)
//
// A bare `*` (Echo's unnamed wildcard) has no OpenAPI equivalent and is kept
// as-is. Paths already in OpenAPI form pass through unchanged.
func NormalizePath(path string) string {
	segs := strings.Split(path, "/")

	for i, seg := range segs {
		switch {
		case seg == "{$}":
			segs[i] = ""
		case strings.HasPrefix(seg, ":") && len(seg) > 1:
			segs[i] = "{" + seg[1:] + "}"
		case strings.HasPrefix(seg, "*") && len(seg) > 1:
			segs[i] = "{" + seg[1:] + "}"
		case strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "...}"):
			segs[i] = strings.TrimSuffix(seg, "...}") + "}"
		}
	}

	return strings.Join(segs, "/")
}
