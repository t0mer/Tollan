// Package api implements Tollan's spec-first REST API. The router mounted here
// is the single surface the web UI consumes. The canonical contract lives in
// api/openapi.yaml (embedded) and is served, with a docs page, at /api/docs.
package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/tollan/internal/auth"
	"github.com/t0mer/tollan/internal/crypto"
	"github.com/t0mer/tollan/internal/input"
	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/meta"
	"github.com/t0mer/tollan/internal/notify"
	"github.com/t0mer/tollan/internal/version"
)

// InputLister exposes the running inputs to the API.
type InputLister interface {
	List() []input.Status
}

// ConfigStore is the subset of the metadata store for config entities.
type ConfigStore interface {
	ListEntities(ctx context.Context, kind string) ([]meta.Entity, error)
	GetEntity(ctx context.Context, kind, id string) (meta.Entity, error)
	PutEntity(ctx context.Context, kind, id, name string, data json.RawMessage) (meta.Entity, error)
	DeleteEntity(ctx context.Context, kind, id string) error
}

// UserStore is the user and API-token persistence the API needs.
type UserStore interface {
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, username, role, passwordHash string) (meta.User, error)
	GetUser(ctx context.Context, id string) (meta.User, error)
	GetUserByUsername(ctx context.Context, username string) (meta.User, error)
	ListUsers(ctx context.Context) ([]meta.User, error)
	UpdateUser(ctx context.Context, id, role, passwordHash string) error
	DeleteUser(ctx context.Context, id string) error
	CreateToken(ctx context.Context, userID, name, hash string) (meta.APIToken, error)
	ListTokens(ctx context.Context, userID string) ([]meta.APIToken, error)
	GetTokenByHash(ctx context.Context, hash string) (meta.APIToken, error)
	TouchToken(ctx context.Context, id string)
	DeleteToken(ctx context.Context, userID, id string) error
}

// MetaStore combines saved searches, config-entity, event, user and agent
// storage.
type MetaStore interface {
	SavedSearchStore
	ConfigStore
	UserStore
	AgentStore
	ListEvents(ctx context.Context, limit int) ([]meta.Event, error)
}

// Deps are the API handler dependencies.
type Deps struct {
	Spec   []byte
	Store  logstore.Store
	Inputs InputLister
	Meta   MetaStore
	// Reload re-applies config to the running engine after a config change.
	Reload func(context.Context) error
	// Cipher encrypts notification-channel secrets at rest.
	Cipher *crypto.Cipher
	// Notifier sends test notifications.
	Notifier *notify.Notifier
	// AuthEnabled gates authentication; when false the API is open (lab mode).
	AuthEnabled bool
	// Sessioner signs session cookies.
	Sessioner *auth.Sessioner
	// EnrollmentToken guards agent registration (empty = open enrollment).
	EnrollmentToken string
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
		r.Use(a.authMiddleware)
		r.Route("/auth", a.authRoutes)
		r.Route("/users", a.userRoutes)
		r.Route("/tokens", a.tokenRoutes)
		r.Get("/version", a.handleVersion)
		r.Get("/search", a.handleSearch)
		r.Get("/search/histogram", a.handleHistogram)
		r.Get("/search/fields", a.handleFields)
		r.Get("/search/aggregate", a.handleAggregate)
		r.Get("/search/export", a.handleExport)
		r.Get("/inputs", a.handleInputs)
		r.Get("/events", a.handleListEvents)
		r.Route("/agents", a.agentRoutes)
		r.Route("/saved-searches", a.savedRoutes)
		r.Route("/notifications", a.notificationRoutes)
		r.Route("/content-packs", a.contentPackRoutes)
		a.configRoutes(r)
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
