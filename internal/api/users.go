package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/tollan/internal/auth"
	"github.com/t0mer/tollan/internal/meta"
)

func (a *API) userRoutes(r chi.Router) {
	r.Get("/", a.handleListUsers)
	r.Post("/", a.handleCreateUser)
	r.Put("/{id}", a.handleUpdateUser)
	r.Delete("/{id}", a.handleDeleteUser)
}

func (a *API) tokenRoutes(r chi.Router) {
	r.Get("/", a.handleListTokens)
	r.Post("/", a.handleCreateToken)
	r.Delete("/{id}", a.handleDeleteToken)
}

type userBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func validRole(role string) bool {
	switch role {
	case meta.RoleAdmin, meta.RoleEditor, meta.RoleViewer:
		return true
	}
	return false
}

func (a *API) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.deps.Meta.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]string, 0, len(users))
	for _, u := range users {
		out = append(out, publicUser(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var b userBody
	if err := decodeJSONValue(r, &b); err != nil || b.Username == "" || b.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if !validRole(b.Role) {
		b.Role = meta.RoleViewer
	}
	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := a.deps.Meta.CreateUser(r.Context(), b.Username, b.Role, hash)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not create user (name taken?)")
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(u))
}

func (a *API) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b userBody
	if err := decodeJSONValue(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !validRole(b.Role) {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}
	hash := ""
	if b.Password != "" {
		h, err := auth.HashPassword(b.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		hash = h
	}
	if err := a.deps.Meta.UpdateUser(r.Context(), id, b.Role, hash); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if u, ok := userFrom(r.Context()); ok && u.ID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	err := a.deps.Meta.DeleteUser(r.Context(), id)
	if errors.Is(err, meta.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListTokens(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tokens, err := a.deps.Meta.ListTokens(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tokens == nil {
		tokens = []meta.APIToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (a *API) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if u.ID == "" {
		writeError(w, http.StatusBadRequest, "token creation requires a user (not available in open mode)")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSONValue(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tok, err := a.deps.Meta.CreateToken(r.Context(), u.ID, body.Name, hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The plaintext token is returned exactly once.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": tok.ID, "name": tok.Name, "token": plaintext,
	})
}

func (a *API) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	err := a.deps.Meta.DeleteToken(r.Context(), u.ID, chi.URLParam(r, "id"))
	if errors.Is(err, meta.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
