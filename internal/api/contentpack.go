package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/tollan/internal/meta"
)

// contentPackKinds are the entity kinds included in a content pack. Notification
// channels are excluded because they hold secrets (§10).
var contentPackKinds = []string{
	meta.KindStream, meta.KindPipeline, meta.KindLookup,
	meta.KindDashboard, meta.KindEvent, meta.KindOutput,
}

// bundle is a versioned, portable export of Tollan configuration.
type bundle struct {
	Version       string                       `json:"version"`
	Kinds         map[string][]json.RawMessage `json:"kinds"`
	SavedSearches []meta.SavedSearch           `json:"saved_searches"`
}

func (a *API) contentPackRoutes(r chi.Router) {
	r.Get("/export", a.handleExportPack)
	r.Post("/import", a.handleImportPack)
}

func (a *API) handleExportPack(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata store unavailable")
		return
	}
	b := bundle{Version: "1", Kinds: map[string][]json.RawMessage{}}
	for _, kind := range contentPackKinds {
		ents, err := a.deps.Meta.ListEntities(r.Context(), kind)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		blobs := make([]json.RawMessage, 0, len(ents))
		for _, e := range ents {
			blobs = append(blobs, e.Data)
		}
		b.Kinds[kind] = blobs
	}
	saved, _ := a.deps.Meta.ListSavedSearches(r.Context())
	b.SavedSearches = saved

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="tollan-content-pack.json"`)
	_ = json.NewEncoder(w).Encode(b)
}

// importDiff describes what an import would do to one entity.
type importDiff struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Action string `json:"action"` // create | update
}

func (a *API) handleImportPack(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata store unavailable")
		return
	}
	var b bundle
	if err := decodeJSONValue(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	dryRun := r.URL.Query().Get("dry_run") == "true"

	var diffs []importDiff
	for _, kind := range contentPackKinds {
		for _, blob := range b.Kinds[kind] {
			id, name := idAndName(blob)
			if id == "" {
				continue
			}
			action := "create"
			if _, err := a.deps.Meta.GetEntity(r.Context(), kind, id); err == nil {
				action = "update"
			}
			diffs = append(diffs, importDiff{Kind: kind, ID: id, Name: name, Action: action})
			if !dryRun {
				if _, err := a.deps.Meta.PutEntity(r.Context(), kind, id, name, blob); err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
			}
		}
	}
	if !dryRun {
		for _, ss := range b.SavedSearches {
			_, _ = a.deps.Meta.CreateSavedSearch(r.Context(), ss.Name, ss.Query, ss.TimeRange)
		}
		a.reload(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{"dry_run": dryRun, "changes": diffs})
}

func idAndName(blob json.RawMessage) (string, string) {
	var m map[string]any
	if err := json.Unmarshal(blob, &m); err != nil {
		return "", ""
	}
	id, _ := m["id"].(string)
	name, _ := m["name"].(string)
	return id, name
}
