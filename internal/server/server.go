// Package server wires the HTTP surface: middleware, health, Prometheus
// metrics, the REST API and the embedded single-page web UI.
package server

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/t0mer/tollan/internal/api"
	"github.com/t0mer/tollan/internal/config"
	"github.com/t0mer/tollan/internal/metrics"
	"github.com/t0mer/tollan/internal/version"
)

// Server is the HTTP server for the UI and API.
type Server struct {
	cfg     config.HTTPConfig
	log     *slog.Logger
	metrics *metrics.Metrics
	http    *http.Server
}

// Options bundles the server dependencies.
type Options struct {
	Config  config.HTTPConfig
	Logger  *slog.Logger
	Metrics *metrics.Metrics
	// APISpec is the raw OpenAPI spec bytes served under /api.
	APISpec []byte
	// WebUI is the built single-page app filesystem (embedded).
	WebUI fs.FS
}

// New builds a Server with the routes mounted.
func New(opts Options) *Server {
	s := &Server{
		cfg:     opts.Config,
		log:     opts.Logger,
		metrics: opts.Metrics,
	}
	s.http = &http.Server{
		Addr:         opts.Config.Addr,
		Handler:      s.routes(opts),
		ReadTimeout:  opts.Config.ReadTimeout,
		WriteTimeout: opts.Config.WriteTimeout,
		IdleTimeout:  opts.Config.IdleTimeout,
	}
	return s
}

func (s *Server) routes(opts Options) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.requestLogger)
	r.Use(middleware.Recoverer)

	r.Get("/health", s.handleHealth)
	if s.metrics != nil {
		r.Handle("/metrics", s.metrics.Handler())
	}

	r.Mount("/api", api.New(opts.APISpec).Routes())

	if opts.WebUI != nil {
		r.Handle("/*", spaHandler(opts.WebUI))
	}
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","version":"` + version.Version + `"}`))
}

// requestLogger logs each request at debug level with method, path and status.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		s.log.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration", time.Since(start).String(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

// spaHandler serves static assets from the embedded UI and falls back to
// index.html for client-side routes (single-page app behaviour).
func spaHandler(uiFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(uiFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(uiFS, p); err != nil {
			// Unknown path → serve the SPA shell so the client router handles it.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// Addr returns the configured listen address.
func (s *Server) Addr() string { return s.cfg.Addr }

// Start begins serving and blocks until the server stops. It returns nil on a
// graceful shutdown.
func (s *Server) Start() error {
	s.log.Info("http server listening", "addr", s.cfg.Addr)
	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server, waiting up to the context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
