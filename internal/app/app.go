// Package app assembles Tollan's subsystems from configuration and runs them
// with coordinated graceful shutdown. Both the foreground `run` command and the
// OS-service wrapper drive the same App.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	tollan "github.com/t0mer/tollan"
	"github.com/t0mer/tollan/internal/config"
	"github.com/t0mer/tollan/internal/metrics"
	"github.com/t0mer/tollan/internal/server"
	"github.com/t0mer/tollan/internal/version"
)

// shutdownTimeout bounds graceful shutdown (§12: max 30s).
const shutdownTimeout = 30 * time.Second

// App holds the assembled, runnable subsystems.
type App struct {
	cfg    config.Config
	log    *slog.Logger
	server *server.Server
}

// New builds the App from resolved configuration.
func New(cfg config.Config, log *slog.Logger) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating data dir %q: %w", cfg.DataDir, err)
	}

	m := metrics.New()

	webUI, err := tollan.WebDistFS()
	if err != nil {
		return nil, fmt.Errorf("loading embedded web UI: %w", err)
	}

	srv := server.New(server.Options{
		Config:  cfg.HTTP,
		Logger:  log,
		Metrics: m,
		APISpec: tollan.OpenAPISpec(),
		WebUI:   webUI,
	})

	return &App{cfg: cfg, log: log, server: srv}, nil
}

// Run starts all subsystems and blocks until ctx is cancelled, then performs a
// graceful shutdown.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting tollan",
		"version", version.Version,
		"data_dir", a.cfg.DataDir,
		"auth", a.cfg.Auth.Mode,
	)
	if a.cfg.Auth.Mode == "disabled" {
		a.log.Warn("auth is DISABLED — the API and UI are open to anyone who can reach them")
	}

	errCh := make(chan error, 1)
	go func() { errCh <- a.server.Start() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		a.log.Info("shutdown requested, draining")
		sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(sctx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		a.log.Info("shutdown complete")
		return nil
	}
}
