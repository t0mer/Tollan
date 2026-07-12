// Package app assembles Tollan's subsystems from configuration and runs them
// with coordinated graceful shutdown. Both the foreground `run` command and the
// OS-service wrapper drive the same App.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	tollan "github.com/t0mer/tollan"
	"github.com/t0mer/tollan/internal/config"
	"github.com/t0mer/tollan/internal/input"
	"github.com/t0mer/tollan/internal/journal"
	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/logstore/sqlite"
	"github.com/t0mer/tollan/internal/metrics"
	"github.com/t0mer/tollan/internal/processing"
	"github.com/t0mer/tollan/internal/server"
	"github.com/t0mer/tollan/internal/stream"
	"github.com/t0mer/tollan/internal/version"
)

// shutdownTimeout bounds graceful shutdown (§12: max 30s).
const shutdownTimeout = 30 * time.Second

// App holds the assembled, runnable subsystems.
type App struct {
	cfg     config.Config
	log     *slog.Logger
	metrics *metrics.Metrics

	journal   *journal.Journal
	store     logstore.Store
	processor *processing.Processor
	inputs    *input.Manager
	inputCfgs []input.Config
	server    *server.Server
}

// New builds the App from resolved configuration, opening the journal and log
// store and wiring the ingest pipeline.
func New(cfg config.Config, log *slog.Logger) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating data dir %q: %w", cfg.DataDir, err)
	}

	m := metrics.New()

	jnl, err := journal.Open(journal.Options{
		Dir:             filepath.Join(cfg.DataDir, "journal"),
		MaxSegmentBytes: cfg.Journal.MaxSegmentBytes,
		MaxTotalBytes:   cfg.Journal.MaxTotalBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("opening journal: %w", err)
	}

	store, err := sqlite.Open(filepath.Join(cfg.DataDir, "logs"))
	if err != nil {
		return nil, fmt.Errorf("opening log store: %w", err)
	}

	processor := processing.New(processing.Options{
		Journal: jnl,
		Store:   store,
		Router:  stream.NewRouter(),
		Logger:  log,
		Metrics: m,
	})

	inputCfgs := cfg.Inputs
	if len(inputCfgs) == 0 {
		inputCfgs = config.DefaultInputs()
	}

	webUI, err := tollan.WebDistFS()
	if err != nil {
		return nil, fmt.Errorf("loading embedded web UI: %w", err)
	}

	mgr := input.NewManager(log)

	srv := server.New(server.Options{
		Config:  cfg.HTTP,
		Logger:  log,
		Metrics: m,
		APISpec: tollan.OpenAPISpec(),
		WebUI:   webUI,
		Store:   store,
		Inputs:  mgr,
	})

	return &App{
		cfg:       cfg,
		log:       log,
		metrics:   m,
		journal:   jnl,
		store:     store,
		processor: processor,
		inputs:    mgr,
		inputCfgs: inputCfgs,
		server:    srv,
	}, nil
}

// Run starts all subsystems and blocks until ctx is cancelled, then performs a
// graceful shutdown in the order: stop HTTP → stop inputs → flush+close journal
// → drain processor → close store.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting tollan",
		"version", version.Version,
		"data_dir", a.cfg.DataDir,
		"auth", a.cfg.Auth.Mode,
	)
	if a.cfg.Auth.Mode == "disabled" {
		a.log.Warn("auth is DISABLED — the API and UI are open to anyone who can reach them")
	}

	// Ingest pipeline: inputs publish to the journal; the processor drains it.
	pub := &journalPublisher{journal: a.journal, metrics: a.metrics}
	if err := a.inputs.StartAll(a.inputCfgs, pub); err != nil {
		return fmt.Errorf("starting inputs: %w", err)
	}

	procCtx, procCancel := context.WithCancel(context.Background())
	procDone := make(chan error, 1)
	go func() { procDone <- a.processor.Run(procCtx) }()

	srvErr := make(chan error, 1)
	go func() { srvErr <- a.server.Start() }()

	select {
	case err := <-srvErr:
		a.shutdown(procCancel, procDone)
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case err := <-procDone:
		// Processor should only return on drain; an early return is an error.
		a.shutdown(procCancel, procDone)
		return err
	case <-ctx.Done():
		a.shutdown(procCancel, procDone)
		return nil
	}
}

// shutdown drains subsystems in order, bounded by shutdownTimeout.
func (a *App) shutdown(procCancel context.CancelFunc, procDone chan error) {
	a.log.Info("shutdown requested, draining")
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(sctx); err != nil {
		a.log.Warn("http shutdown", "error", err)
	}
	if err := a.inputs.StopAll(sctx); err != nil {
		a.log.Warn("input shutdown", "error", err)
	}

	// Flush and close the journal so the processor drains remaining records and
	// then returns ErrClosed.
	if err := a.journal.Sync(); err != nil {
		a.log.Warn("journal sync", "error", err)
	}
	if err := a.journal.Close(); err != nil {
		a.log.Warn("journal close", "error", err)
	}

	select {
	case <-procDone:
	case <-sctx.Done():
		a.log.Warn("processor drain timed out; forcing stop")
		procCancel()
		<-procDone
	}

	if err := a.store.Close(); err != nil {
		a.log.Warn("store close", "error", err)
	}
	a.log.Info("shutdown complete")
}

// journalPublisher adapts input.RawMessage to the ingest journal and counts
// received messages per input.
type journalPublisher struct {
	journal *journal.Journal
	metrics *metrics.Metrics
}

func (p *journalPublisher) Publish(rm input.RawMessage) error {
	_, err := p.journal.Append(journal.Record{
		InputID:    rm.InputID,
		InputType:  rm.InputType,
		Source:     rm.Source,
		ReceivedAt: rm.ReceivedAt,
		Payload:    rm.Payload,
	})
	if err == nil && p.metrics != nil {
		p.metrics.MessagesIn.WithLabelValues(rm.InputID, rm.InputType).Inc()
	}
	return err
}
