package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/tollan/internal/auth"
	"github.com/t0mer/tollan/internal/meta"
)

const sessionCookie = "tollan_session"
const sessionTTL = 12 * time.Hour

type ctxKey int

const userCtxKey ctxKey = 0

// currentUser is the authenticated principal for a request.
type currentUser struct {
	ID       string
	Username string
	Role     string
}

func userFrom(ctx context.Context) (currentUser, bool) {
	u, ok := ctx.Value(userCtxKey).(currentUser)
	return u, ok
}

// authMiddleware authenticates requests and enforces role-based access. In lab
// mode (auth disabled) or bootstrap (no users yet) it is fully open so the first
// admin can be created.
func (a *API) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always allow the auth entrypoints and health.
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/v1/auth/login") || strings.HasPrefix(p, "/api/v1/auth/setup") ||
			strings.HasPrefix(p, "/api/v1/auth/status") || isAgentSelfService(p) {
			next.ServeHTTP(w, r)
			return
		}

		if !a.deps.AuthEnabled {
			next.ServeHTTP(w, withUser(r, currentUser{Username: "anonymous", Role: meta.RoleAdmin}))
			return
		}
		if a.deps.Meta != nil {
			if n, err := a.deps.Meta.CountUsers(r.Context()); err == nil && n == 0 {
				// Bootstrap: no users yet — open until an admin is created.
				next.ServeHTTP(w, withUser(r, currentUser{Username: "bootstrap", Role: meta.RoleAdmin}))
				return
			}
		}

		user, ok := a.resolveUser(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !allowed(user.Role, r.Method, p) {
			writeError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		next.ServeHTTP(w, withUser(r, user))
	})
}

// isAgentSelfService reports whether a path is an agent's own enrollment,
// heartbeat or config poll (authenticated by enrollment token / agent id, not a
// user session).
func isAgentSelfService(p string) bool {
	if p == "/api/v1/agents/register" {
		return true
	}
	return strings.HasPrefix(p, "/api/v1/agents/") &&
		(strings.HasSuffix(p, "/heartbeat") || strings.HasSuffix(p, "/config"))
}

func withUser(r *http.Request, u currentUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userCtxKey, u))
}

// resolveUser authenticates via Bearer token or session cookie.
func (a *API) resolveUser(r *http.Request) (currentUser, bool) {
	if bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); bearer != "" && bearer != r.Header.Get("Authorization") {
		if tok, err := a.deps.Meta.GetTokenByHash(r.Context(), auth.HashToken(bearer)); err == nil {
			if u, err := a.deps.Meta.GetUser(r.Context(), tok.UserID); err == nil {
				a.deps.Meta.TouchToken(r.Context(), tok.ID)
				return currentUser{ID: u.ID, Username: u.Username, Role: u.Role}, true
			}
		}
	}
	if c, err := r.Cookie(sessionCookie); err == nil && a.deps.Sessioner != nil {
		if uid, ok := a.deps.Sessioner.Verify(c.Value); ok {
			if u, err := a.deps.Meta.GetUser(r.Context(), uid); err == nil {
				return currentUser{ID: u.ID, Username: u.Username, Role: u.Role}, true
			}
		}
	}
	return currentUser{}, false
}

// allowed applies role-based access control.
func allowed(role, method, path string) bool {
	// User management is admin-only.
	if strings.HasPrefix(path, "/api/v1/users") {
		return role == meta.RoleAdmin
	}
	// Own tokens and auth/me are available to any authenticated user.
	if strings.HasPrefix(path, "/api/v1/auth") || strings.HasPrefix(path, "/api/v1/tokens") {
		return true
	}
	// Reads are open to any authenticated user.
	if method == http.MethodGet || method == http.MethodHead {
		return true
	}
	// Writes require editor or admin.
	return role == meta.RoleAdmin || role == meta.RoleEditor
}

// --- auth endpoints ---

func (a *API) authRoutes(r chi.Router) {
	r.Get("/status", a.handleAuthStatus)
	r.Post("/setup", a.handleSetup)
	r.Post("/login", a.handleLogin)
	r.Post("/logout", a.handleLogout)
	r.Get("/me", a.handleMe)
}

type authStatus struct {
	AuthEnabled bool `json:"auth_enabled"`
	NeedsSetup  bool `json:"needs_setup"`
}

func (a *API) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	st := authStatus{AuthEnabled: a.deps.AuthEnabled}
	if a.deps.AuthEnabled && a.deps.Meta != nil {
		if n, err := a.deps.Meta.CountUsers(r.Context()); err == nil {
			st.NeedsSetup = n == 0
		}
	}
	writeJSON(w, http.StatusOK, st)
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *API) handleSetup(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if n, _ := a.deps.Meta.CountUsers(r.Context()); n > 0 {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}
	var c credentials
	if err := decodeJSONValue(r, &c); err != nil || c.Username == "" || c.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := a.deps.Meta.CreateUser(r.Context(), c.Username, meta.RoleAdmin, hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.setSession(w, r, u.ID)
	writeJSON(w, http.StatusCreated, publicUser(u))
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	if a.deps.Meta == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	var c credentials
	if err := decodeJSONValue(r, &c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, err := a.deps.Meta.GetUserByUsername(r.Context(), c.Username)
	if err != nil || !auth.VerifyPassword(c.Password, u.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	a.setSession(w, r, u.ID)
	writeJSON(w, http.StatusOK, publicUser(u))
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": u.ID, "username": u.Username, "role": u.Role})
}

func (a *API) setSession(w http.ResponseWriter, r *http.Request, userID string) {
	if a.deps.Sessioner == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    a.deps.Sessioner.Sign(userID, sessionTTL),
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// isHTTPS reports whether the request arrived over TLS, directly or via a
// trusted reverse proxy, so the session cookie can carry the Secure flag.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func publicUser(u meta.User) map[string]string {
	return map[string]string{"id": u.ID, "username": u.Username, "role": u.Role}
}
