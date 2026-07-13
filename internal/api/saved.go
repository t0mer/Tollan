package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/tollan/internal/meta"
)

// SavedSearchStore is the subset of the metadata store the API needs.
type SavedSearchStore interface {
	ListSavedSearches(ctx context.Context) ([]meta.SavedSearch, error)
	CreateSavedSearch(ctx context.Context, name, query, timeRange string) (meta.SavedSearch, error)
	UpdateSavedSearch(ctx context.Context, id, name, query, timeRange string) (meta.SavedSearch, error)
	DeleteSavedSearch(ctx context.Context, id string) error
}

type savedSearchBody struct {
	Name      string `json:"name"`
	Query     string `json:"query"`
	TimeRange string `json:"time_range"`
}

func (a *API) savedRoutes(r chi.Router) {
	r.Get("/", a.handleListSaved)
	r.Post("/", a.handleCreateSaved)
	r.Put("/{id}", a.handleUpdateSaved)
	r.Delete("/{id}", a.handleDeleteSaved)
}

func (a *API) handleListSaved(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeJSON(w, http.StatusOK, []meta.SavedSearch{})
		return
	}
	list, err := a.deps.Meta.ListSavedSearches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []meta.SavedSearch{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) handleCreateSaved(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata store unavailable")
		return
	}
	var body savedSearchBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ss, err := a.deps.Meta.CreateSavedSearch(r.Context(), body.Name, body.Query, body.TimeRange)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ss)
}

func (a *API) handleUpdateSaved(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata store unavailable")
		return
	}
	id := chi.URLParam(r, "id")
	var body savedSearchBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ss, err := a.deps.Meta.UpdateSavedSearch(r.Context(), id, body.Name, body.Query, body.TimeRange)
	if errors.Is(err, meta.ErrNotFound) {
		writeError(w, http.StatusNotFound, "saved search not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ss)
}

func (a *API) handleDeleteSaved(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata store unavailable")
		return
	}
	id := chi.URLParam(r, "id")
	err := a.deps.Meta.DeleteSavedSearch(r.Context(), id)
	if errors.Is(err, meta.ErrNotFound) {
		writeError(w, http.StatusNotFound, "saved search not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeJSON strictly decodes a request body.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
