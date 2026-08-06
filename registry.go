package goenax

type Registry struct {
	endpoints []Endpoint
}

func New() *Registry {
	return &Registry{}
}

func (r *Registry) Add(e Endpoint) {
	r.endpoints = append(r.endpoints, e)
}

func (r *Registry) Endpoints() []Endpoint {
	out := make([]Endpoint, len(r.endpoints))
	copy(out, r.endpoints)

	return out
}

func (r *Registry) Lookup(method, path string) (Endpoint, bool) {
	for _, e := range r.endpoints {
		if e.Method == method && e.Path == path {
			return e, true
		}
	}

	return Endpoint{}, false
}
