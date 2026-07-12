// Package api implements Tollan's spec-first REST API. The router mounted here
// is the single surface the web UI consumes. The canonical contract lives in
// api/openapi.yaml (embedded) and is served, with an interactive docs UI, at
// /api/docs.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/tollan/internal/version"
)

// API holds the dependencies handlers need. It grows as subsystems are wired in.
type API struct {
	spec []byte
}

// New constructs an API with the embedded OpenAPI spec bytes.
func New(spec []byte) *API {
	return &API{spec: spec}
}

// Routes returns the chi router for everything under the API surface: the
// versioned endpoints plus the OpenAPI spec and docs UI.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/openapi.yaml", a.handleSpec)
	r.Get("/docs", a.handleDocs)

	r.Route("/v1", func(r chi.Router) {
		r.Get("/version", a.handleVersion)
	})
	return r
}

func (a *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Get())
}

func (a *API) handleSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(a.spec)
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
