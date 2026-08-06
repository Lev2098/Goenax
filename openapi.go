package goenax

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const (
	mimeJSON     = "application/json"
	mimeHTML     = "text/html"
	bearerScheme = "bearerAuth"
)

// Info is the OpenAPI `info` block plus the servers the API is reachable at.
type Info struct {
	Title   string
	Version string
	Servers []Server
}

// Server is one OpenAPI server entry.
type Server struct {
	URL         string
	Description string
}

// OpenAPI projects the registry into an OpenAPI 3.1 document and returns it as
// indented JSON. Map keys (paths, methods, status codes) serialise in a stable
// order, so the output is deterministic and safe to diff / drift-check.
func (r *Registry) OpenAPI(info Info) ([]byte, error) {
	doc := openAPIDoc{
		OpenAPI: "3.1.0",
		Info:    docInfo{Title: info.Title, Version: info.Version},
		Servers: serversOf(info.Servers),
		Paths:   map[string]map[string]operation{},
	}

	b := &builder{
		schemas: map[string]*Schema{},
		names:   map[reflect.Type]string{},
		seen:    map[reflect.Type]bool{},
	}
	tagSet := map[string]struct{}{}
	secured := false

	for _, e := range r.endpoints {
		if doc.Paths[e.Path] == nil {
			doc.Paths[e.Path] = map[string]operation{}
		}

		doc.Paths[e.Path][strings.ToLower(e.Method)] = b.operationFor(e)

		if e.Secured {
			secured = true
		}

		for _, tag := range e.Tags {
			tagSet[tag] = struct{}{}
		}
	}

	doc.Tags = sortedTags(tagSet)
	doc.Components = componentsOf(b.schemas, secured)

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("goenax: marshal openapi: %w", err)
	}

	return out, nil
}

func (b *builder) operationFor(e Endpoint) operation {
	op := operation{
		Tags:        e.Tags,
		Summary:     e.Summary,
		Description: e.Desc,
		OperationID: operationID(e.Method, e.Path),
		Security:    securityFor(e.Secured),
		Parameters:  paramsOf(e.Params),
		XDependsOn:  e.DependsOn,
		Responses:   map[string]response{},
	}

	if e.Request != nil {
		schema := b.of(e.Request)
		op.RequestBody = &requestBody{
			Required: true,
			Content:  jsonContent(&schema),
		}
	}

	success := response{Description: "Successful response"}

	switch {
	case e.SuccessHTML:
		success.Content = htmlContent()
	case e.Response != nil:
		schema := b.of(e.Response)
		success.Content = jsonContent(&schema)
	}

	if len(e.RespHeaders) > 0 {
		success.Headers = headersOf(e.RespHeaders)
	}

	op.Responses[strconv.Itoa(e.Status)] = success

	for _, ex := range e.Also {
		resp := response{Description: ex.Desc}

		if ex.Type != nil {
			schema := b.of(ex.Type)
			resp.Content = jsonContent(&schema)
		}

		op.Responses[strconv.Itoa(ex.Status)] = resp
	}

	for _, er := range e.Errors {
		op.Responses[strconv.Itoa(er.Status)] = response{
			Description: errorDescription(er),
			Content:     jsonContent(errorSchema()),
		}
	}

	return op
}

func errorDescription(er ErrorSpec) string {
	if er.Message != "" {
		return er.Message
	}

	return er.Code
}

// errorSchema is the shape of our JSON error bodies: { "message": "..." }.
func errorSchema() *Schema {
	return &Schema{
		Type:       typeObject,
		Properties: map[string]*Schema{"message": {Type: typeString}},
		Required:   []string{"message"},
	}
}

func jsonContent(schema *Schema) map[string]mediaType {
	return map[string]mediaType{mimeJSON: {Schema: schema}}
}

func htmlContent() map[string]mediaType {
	return map[string]mediaType{mimeHTML: {Schema: &Schema{Type: typeString}}}
}

func paramsOf(params []Param) []parameter {
	out := make([]parameter, 0, len(params))
	for _, p := range params {
		out = append(out, parameter{
			Name:        p.Name,
			In:          p.In,
			Required:    p.Required,
			Description: p.Desc,
			Schema:      &Schema{Type: typeString},
		})
	}

	return out
}

// operationID is a stable, unique id per method+path, e.g.
// post_api_auth_change_password.
func operationID(method, path string) string {
	id := strings.Trim(path, "/")
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "-", "_")

	return strings.ToLower(method) + "_" + id
}

func componentsOf(schemas map[string]*Schema, secured bool) *components {
	c := &components{}

	if len(schemas) > 0 {
		c.Schemas = schemas
	}

	if secured {
		c.SecuritySchemes = map[string]securityScheme{
			bearerScheme: {Type: "http", Scheme: "bearer"},
		}
	}

	if c.Schemas == nil && c.SecuritySchemes == nil {
		return nil
	}

	return c
}

func sortedTags(set map[string]struct{}) []docTag {
	tags := make([]docTag, 0, len(set))
	for name := range set {
		tags = append(tags, docTag{Name: name})
	}

	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })

	return tags
}

func serversOf(list []Server) []docServer {
	out := make([]docServer, 0, len(list))
	for _, s := range list {
		out = append(out, docServer(s))
	}

	return out
}

// securityFor returns the per-operation security requirement: the bearer scheme
// for a secured endpoint, or an explicit empty list (no auth) for a public one.
func securityFor(secured bool) []securityReq {
	if secured {
		return []securityReq{{bearerScheme: {}}}
	}

	return []securityReq{}
}

type openAPIDoc struct {
	OpenAPI    string                          `json:"openapi"`
	Info       docInfo                         `json:"info"`
	Servers    []docServer                     `json:"servers,omitempty"`
	Tags       []docTag                        `json:"tags,omitempty"`
	Paths      map[string]map[string]operation `json:"paths"`
	Components *components                     `json:"components,omitempty"`
}

type docInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type docServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type docTag struct {
	Name string `json:"name"`
}

type components struct {
	Schemas         map[string]*Schema        `json:"schemas,omitempty"`
	SecuritySchemes map[string]securityScheme `json:"securitySchemes,omitempty"`
}

type securityScheme struct {
	Type   string `json:"type"`
	Scheme string `json:"scheme,omitempty"`
}

// securityReq is one OpenAPI security requirement: scheme name -> scopes.
type securityReq map[string][]string

type operation struct {
	Tags        []string            `json:"tags,omitempty"`
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	OperationID string              `json:"operationId,omitempty"`
	Security    []securityReq       `json:"security"`
	Parameters  []parameter         `json:"parameters,omitempty"`
	XDependsOn  []string            `json:"x-dependsOn,omitempty"`
	RequestBody *requestBody        `json:"requestBody,omitempty"`
	Responses   map[string]response `json:"responses"`
}

type parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Required    bool    `json:"required"`
	Description string  `json:"description,omitempty"`
	Schema      *Schema `json:"schema"`
}

type requestBody struct {
	Required bool                 `json:"required"`
	Content  map[string]mediaType `json:"content"`
}

type mediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type response struct {
	Description string               `json:"description"`
	Headers     map[string]headerObj `json:"headers,omitempty"`
	Content     map[string]mediaType `json:"content,omitempty"`
}

type headerObj struct {
	Description string  `json:"description,omitempty"`
	Schema      *Schema `json:"schema"`
}

func headersOf(hs []ResponseHeader) map[string]headerObj {
	m := make(map[string]headerObj, len(hs))
	for _, h := range hs {
		m[h.Name] = headerObj{Description: h.Desc, Schema: &Schema{Type: typeString}}
	}

	return m
}
