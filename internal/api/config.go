package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/t0mer/tollan/internal/lookup"
	"github.com/t0mer/tollan/internal/meta"
	"github.com/t0mer/tollan/internal/pipeline"
	"github.com/t0mer/tollan/internal/pipeline/dsl"
	"github.com/t0mer/tollan/internal/search/query"
	"github.com/t0mer/tollan/internal/stream"
)

// validator normalizes and validates an entity's JSON, returning its display
// name and the normalized bytes to store (with id injected).
type validator func(id string, raw json.RawMessage) (name string, out json.RawMessage, err error)

// configRoutes mounts CRUD for the config entity kinds.
func (a *API) configRoutes(r chi.Router) {
	r.Route("/streams", func(r chi.Router) { a.entityRoutes(r, meta.KindStream, validateStream) })
	r.Route("/pipelines", func(r chi.Router) { a.entityRoutes(r, meta.KindPipeline, validatePipeline) })
	r.Route("/lookups", func(r chi.Router) { a.entityRoutes(r, meta.KindLookup, validateLookup) })
	r.Route("/dashboards", func(r chi.Router) { a.entityRoutes(r, meta.KindDashboard, validateNamed) })
	r.Route("/event-definitions", func(r chi.Router) { a.entityRoutes(r, meta.KindEvent, validateEventDef) })
}

// validateEventDef validates an event definition: name required and the query
// must parse.
func validateEventDef(id string, raw json.RawMessage) (string, json.RawMessage, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", nil, err
	}
	name, _ := m["name"].(string)
	if name == "" {
		return "", nil, errors.New("name is required")
	}
	if q, ok := m["query"].(string); ok {
		if _, err := query.Parse(q); err != nil {
			return "", nil, err
		}
	}
	m["id"] = id
	out, err := json.Marshal(m)
	return name, out, err
}

// validateNamed is a generic validator: it requires a "name" field and injects
// the id, preserving all other fields. Used for UI-defined entities like
// dashboards, events, channels and outputs.
func validateNamed(id string, raw json.RawMessage) (string, json.RawMessage, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", nil, err
	}
	name, _ := m["name"].(string)
	if name == "" {
		return "", nil, errors.New("name is required")
	}
	m["id"] = id
	out, err := json.Marshal(m)
	return name, out, err
}

// entityRoutes registers list/create/update/delete for a kind.
func (a *API) entityRoutes(r chi.Router, kind string, v validator) {
	r.Get("/", a.listEntities(kind))
	r.Post("/", a.putEntity(kind, v, true))
	r.Put("/{id}", a.putEntity(kind, v, false))
	r.Delete("/{id}", a.deleteEntity(kind))
}

func (a *API) listEntities(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Meta == nil {
			writeJSON(w, http.StatusOK, []json.RawMessage{})
			return
		}
		ents, err := a.deps.Meta.ListEntities(r.Context(), kind)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]json.RawMessage, 0, len(ents))
		for _, e := range ents {
			out = append(out, e.Data)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (a *API) putEntity(kind string, v validator, create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Meta == nil {
			writeError(w, http.StatusServiceUnavailable, "metadata store unavailable")
			return
		}
		id := chi.URLParam(r, "id")
		if create {
			id = uuid.NewString()
		}
		raw, err := readBody(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		name, normalized, err := v(id, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ent, err := a.deps.Meta.PutEntity(r.Context(), kind, id, name, normalized)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.reload(r.Context())
		status := http.StatusOK
		if create {
			status = http.StatusCreated
		}
		writeJSON(w, status, ent.Data)
	}
}

func (a *API) deleteEntity(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Meta == nil {
			writeError(w, http.StatusServiceUnavailable, "metadata store unavailable")
			return
		}
		id := chi.URLParam(r, "id")
		err := a.deps.Meta.DeleteEntity(r.Context(), kind, id)
		if errors.Is(err, meta.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.reload(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}

// reload triggers a config reload, logging (ignoring) any error since the write
// already succeeded.
func (a *API) reload(ctx context.Context) {
	if a.deps.Reload != nil {
		_ = a.deps.Reload(ctx)
	}
}

func readBody(r *http.Request) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := decodeJSON(r, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// --- validators ---

func validateStream(id string, raw json.RawMessage) (string, json.RawMessage, error) {
	var s stream.Stream
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", nil, err
	}
	s.ID = id
	if s.Name == "" {
		return "", nil, errors.New("name is required")
	}
	if s.Combinator == "" {
		s.Combinator = stream.And
	}
	if _, err := stream.Compile(s); err != nil {
		return "", nil, err
	}
	out, err := json.Marshal(s)
	return s.Name, out, err
}

func validatePipeline(id string, raw json.RawMessage) (string, json.RawMessage, error) {
	var p pipeline.Pipeline
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", nil, err
	}
	p.ID = id
	if p.Name == "" {
		return "", nil, errors.New("name is required")
	}
	for _, rule := range p.Rules {
		if _, err := dsl.CompileRule(rule.Name, rule.When, rule.Then); err != nil {
			return "", nil, err
		}
	}
	out, err := json.Marshal(p)
	return p.Name, out, err
}

func validateLookup(id string, raw json.RawMessage) (string, json.RawMessage, error) {
	var c lookup.Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", nil, err
	}
	if c.Name == "" {
		return "", nil, errors.New("name is required")
	}
	if c.SourceType != lookup.SourceFile && c.SourceType != lookup.SourceURL {
		return "", nil, errors.New("source_type must be 'file' or 'url'")
	}
	// Wrap with id-bearing envelope for listing/deletion by id.
	env := struct {
		lookup.Config
		ID string `json:"id"`
	}{Config: c, ID: id}
	out, err := json.Marshal(env)
	return c.Name, out, err
}
