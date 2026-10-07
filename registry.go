package goenax

import "sync"

// Registry holds the declared endpoints. It is safe for concurrent use, so the
// spec can be served live (see Docs) while routes are still being registered.
type Registry struct {
	mu        sync.RWMutex
	endpoints []Endpoint
	ignored   map[Route]bool
	version   uint64 // bumped on every change; lets Docs cache the spec
}

func New() *Registry {
	return &Registry{}
}

// Add records an endpoint. Its method is upper-cased and its path normalised to
// OpenAPI form (see NormalizePath).
func (r *Registry) Add(e Endpoint) {
	k := key(e.Method, e.Path)
	e.Method, e.Path = k.Method, k.Path

	r.mu.Lock()
	defer r.mu.Unlock()

	r.endpoints = append(r.endpoints, e)
	r.version++
}

// Ignore marks a route as intentionally undocumented — health checks, metrics,
// the docs routes themselves — so Coverage does not report it.
func (r *Registry) Ignore(method, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ignored == nil {
		r.ignored = map[Route]bool{}
	}

	r.ignored[key(method, path)] = true
}

func (r *Registry) Endpoints() []Endpoint {
	return r.snapshot()
}

// Lookup finds an endpoint by method and path template. The method is matched
// case-insensitively and the path may be in router-native form (":id").
func (r *Registry) Lookup(method, path string) (Endpoint, bool) {
	k := key(method, path)

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, e := range r.endpoints {
		if e.Method == k.Method && e.Path == k.Path {
			return e, true
		}
	}

	return Endpoint{}, false
}

func (r *Registry) currentVersion() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.version
}

func (r *Registry) snapshot() []Endpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	eps := make([]Endpoint, len(r.endpoints))
	copy(eps, r.endpoints)

	return eps
}
