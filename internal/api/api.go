// Package api implements Tollan's spec-first REST API. The router mounted here
// is the single surface the web UI consumes. The canonical contract lives in
// api/openapi.yaml (embedded) and is served, with a docs page, at /api/docs.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/tollan/internal/input"
	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/version"
)

// InputLister exposes the running inputs to the API.
type InputLister interface {
	List() []input.Status
}

// Deps are the API handler dependencies.
type Deps struct {
	Spec   []byte
	Store  logstore.Store
	Inputs InputLister
}

// API holds the handler dependencies.
type API struct {
	deps Deps
}

// New constructs an API from its dependencies.
func New(deps Deps) *API {
	return &API{deps: deps}
}

// Routes returns the chi router for the API surface.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/openapi.yaml", a.handleSpec)
	r.Get("/docs", a.handleDocs)

	r.Route("/v1", func(r chi.Router) {
		r.Get("/version", a.handleVersion)
		r.Get("/search", a.handleSearch)
		r.Get("/inputs", a.handleInputs)
		r.Get("/streams", a.handleStreams)
	})
	return r
}

func (a *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Get())
}

func (a *API) handleInputs(w http.ResponseWriter, r *http.Request) {
	if a.deps.Inputs == nil {
		writeJSON(w, http.StatusOK, []input.Status{})
		return
	}
	writeJSON(w, http.StatusOK, a.deps.Inputs.List())
}

func (a *API) handleSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(a.deps.Spec)
}

func (a *API) handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsHTML))
}

// writeJSON encodes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error envelope.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// docsHTML is a self-contained landing page for the API docs. It links to the
// machine-readable spec served by this binary and loads no external scripts, so
// the scratch image stays self-contained. Interactive Swagger UI is bundled via
// the embedded web build in a later phase.
const docsHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Tollan API</title>
    <style>
      body { font: 16px/1.5 system-ui, sans-serif; margin: 2rem auto; max-width: 42rem; padding: 0 1rem; }
      code { background: #0001; padding: .1em .3em; border-radius: .2em; }
      a { color: #2563eb; }
    </style>
  </head>
  <body>
    <h1>Tollan API</h1>
    <p>The machine-readable OpenAPI 3 specification is available at
      <a href="/api/openapi.yaml"><code>/api/openapi.yaml</code></a>.</p>
    <p>Interactive documentation is served from the web UI once it is built.</p>
  </body>
</html>`
