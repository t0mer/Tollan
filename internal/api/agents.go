package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/tollan/internal/meta"
)

// AgentStore is the fleet persistence the API needs.
type AgentStore interface {
	UpsertAgent(ctx context.Context, a meta.Agent) error
	Heartbeat(ctx context.Context, id string, shipped int64) error
	SetAgentConfig(ctx context.Context, id string, cfg meta.AgentConfig, tags []string) error
	GetAgent(ctx context.Context, id string) (meta.Agent, error)
	ListAgents(ctx context.Context) ([]meta.Agent, error)
	DeleteAgent(ctx context.Context, id string) error
}

func (a *API) agentRoutes(r chi.Router) {
	// Agent self-service (authenticated by enrollment token / agent id).
	r.Post("/register", a.handleAgentRegister)
	r.Post("/{id}/heartbeat", a.handleAgentHeartbeat)
	r.Get("/{id}/config", a.handleAgentConfig)
	// Fleet management (user-authenticated).
	r.Get("/", a.handleListAgents)
	r.Put("/{id}", a.handleUpdateAgent)
	r.Delete("/{id}", a.handleDeleteAgent)
}

type registerRequest struct {
	EnrollmentToken string   `json:"enrollment_token"`
	ID              string   `json:"id"`
	Hostname        string   `json:"hostname"`
	OS              string   `json:"os"`
	Version         string   `json:"version"`
	Tags            []string `json:"tags"`
}

func (a *API) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	var req registerRequest
	if err := decodeJSONValue(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if a.deps.EnrollmentToken != "" &&
		subtle.ConstantTimeCompare([]byte(req.EnrollmentToken), []byte(a.deps.EnrollmentToken)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid enrollment token")
		return
	}
	if req.ID == "" || req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "id and hostname required")
		return
	}
	agent := meta.Agent{
		ID: req.ID, Hostname: req.Hostname, OS: req.OS, Version: req.Version,
		Tags: req.Tags, EnrolledAt: time.Now().UTC(),
	}
	if err := a.deps.Meta.UpsertAgent(r.Context(), agent); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Return current config so the agent starts collecting immediately.
	stored, _ := a.deps.Meta.GetAgent(r.Context(), req.ID)
	writeJSON(w, http.StatusOK, map[string]any{"id": req.ID, "config": stored.Config})
}

func (a *API) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Shipped int64 `json:"shipped"`
	}
	_ = decodeJSONValue(r, &body)
	if err := a.deps.Meta.Heartbeat(r.Context(), id, body.Shipped); err != nil {
		writeError(w, http.StatusNotFound, "unknown agent")
		return
	}
	agent, _ := a.deps.Meta.GetAgent(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]int{"config_version": agent.ConfigVersion})
}

func (a *API) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	agent, err := a.deps.Meta.GetAgent(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown agent")
		return
	}
	writeJSON(w, http.StatusOK, agent.Config)
}

func (a *API) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := a.deps.Meta.ListAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agents == nil {
		agents = []meta.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

type updateAgentRequest struct {
	Tags   []string         `json:"tags"`
	Config meta.AgentConfig `json:"config"`
}

func (a *API) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req updateAgentRequest
	if err := decodeJSONValue(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := a.deps.Meta.SetAgentConfig(r.Context(), id, req.Config, req.Tags); err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			writeError(w, http.StatusNotFound, "unknown agent")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	err := a.deps.Meta.DeleteAgent(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, meta.ErrNotFound) {
		writeError(w, http.StatusNotFound, "unknown agent")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
