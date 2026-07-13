// Package app assembles Tollan's subsystems from configuration and runs them
// with coordinated graceful shutdown. Both the foreground `run` command and the
// OS-service wrapper drive the same App.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tollan "github.com/t0mer/tollan"
	"github.com/t0mer/tollan/internal/auth"
	"github.com/t0mer/tollan/internal/config"
	"github.com/t0mer/tollan/internal/crypto"
	"github.com/t0mer/tollan/internal/event"
	"github.com/t0mer/tollan/internal/geoip"
	"github.com/t0mer/tollan/internal/input"
	"github.com/t0mer/tollan/internal/journal"
	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/logstore/sqlite"
	"github.com/t0mer/tollan/internal/lookup"
	"github.com/t0mer/tollan/internal/meta"
	"github.com/t0mer/tollan/internal/metrics"
	"github.com/t0mer/tollan/internal/notify"
	"github.com/t0mer/tollan/internal/output"
	"github.com/t0mer/tollan/internal/pipeline"
	"github.com/t0mer/tollan/internal/processing"
	"github.com/t0mer/tollan/internal/retention"
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
	meta      *meta.Store
	router    *stream.Router
	engine    *pipeline.Engine
	lookups   *lookup.Manager
	geo       *geoip.Resolver
	retention *retention.Manager
	events    *event.Engine
	outputs   *output.Manager
	processor *processing.Processor
	inputs    *input.Manager
	inputCfgs []input.Config
	server    *server.Server

	policyMu sync.Mutex
	policies []retention.StreamPolicy
}

// New builds the App from resolved configuration, opening the stores and wiring
// the ingest → pipeline → store path.
func New(cfg config.Config, log *slog.Logger) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating data dir %q: %w", cfg.DataDir, err)
	}

	// Tollan logs via slog. A few dependencies (e.g. go-lumber) write through the
	// standard library logger; route that into slog at debug level so it is
	// demoted below the default info level rather than cluttering stdout — but
	// still visible with --log-level debug and never silently discarded.
	stdlog.SetOutput(stdlogBridge{log})
	stdlog.SetFlags(0)

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

	metaStore, err := meta.Open(filepath.Join(cfg.DataDir, "tollan.db"))
	if err != nil {
		return nil, fmt.Errorf("opening metadata store: %w", err)
	}

	key, err := crypto.LoadOrCreateKey(filepath.Join(cfg.DataDir, "secret.key"))
	if err != nil {
		return nil, fmt.Errorf("loading secret key: %w", err)
	}
	cipher, err := crypto.New(key)
	if err != nil {
		return nil, fmt.Errorf("initializing cipher: %w", err)
	}
	notifier := notify.New()

	geo, err := geoip.New(cfg.GeoIP.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening geoip: %w", err)
	}
	lookups := lookup.NewManager()
	router := stream.NewRouter()
	engine := pipeline.NewEngine(router, pipeline.Env{Lookups: lookups, Geo: geo})
	outputs := output.NewManager(log, m)

	processor := processing.New(processing.Options{
		Journal: jnl,
		Store:   store,
		Engine:  engine,
		Outputs: outputs,
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

	a := &App{
		cfg:       cfg,
		log:       log,
		metrics:   m,
		journal:   jnl,
		store:     store,
		meta:      metaStore,
		router:    router,
		engine:    engine,
		lookups:   lookups,
		geo:       geo,
		outputs:   outputs,
		processor: processor,
		inputs:    mgr,
		inputCfgs: inputCfgs,
	}
	a.retention = retention.New(store, cfg.Retention.Days, a.streamPolicies, log)
	a.events = event.New(store, metaStore, notifier, cipher, m, log)

	a.server = server.New(server.Options{
		Config:          cfg.HTTP,
		Logger:          log,
		Metrics:         m,
		APISpec:         tollan.OpenAPISpec(),
		WebUI:           webUI,
		Store:           store,
		Inputs:          mgr,
		Meta:            metaStore,
		Reload:          a.Reload,
		Cipher:          cipher,
		Notifier:        notifier,
		AuthEnabled:     cfg.Auth.Mode != "disabled",
		Sessioner:       auth.NewSessioner(key),
		EnrollmentToken: cfg.Agent.EnrollmentToken,
	})

	// Load configured streams/pipelines/lookups so processing starts correctly.
	if err := a.Reload(context.Background()); err != nil {
		log.Warn("initial config load had errors", "error", err)
	}
	return a, nil
}

// Reload recompiles streams, pipelines and lookup tables from the metadata store
// and installs them into the running engine, router and lookup manager. It is
// invoked at startup and after config CRUD via the API.
func (a *App) Reload(ctx context.Context) error {
	// Streams.
	sEnts, err := a.meta.ListEntities(ctx, meta.KindStream)
	if err != nil {
		return err
	}
	var compiled []*stream.Compiled
	var policies []retention.StreamPolicy
	for _, e := range sEnts {
		var s stream.Stream
		if err := json.Unmarshal(e.Data, &s); err != nil {
			a.log.Warn("skipping malformed stream", "id", e.ID, "error", err)
			continue
		}
		s.ID = e.ID
		c, err := stream.Compile(s)
		if err != nil {
			a.log.Warn("skipping stream with bad rules", "id", e.ID, "error", err)
			continue
		}
		compiled = append(compiled, c)
		if s.RetentionDays > 0 {
			policies = append(policies, retention.StreamPolicy{ID: s.ID, Days: s.RetentionDays})
		}
	}
	a.router.SetStreams(compiled)
	a.setPolicies(policies)

	// Lookup tables.
	lEnts, err := a.meta.ListEntities(ctx, meta.KindLookup)
	if err != nil {
		return err
	}
	for _, e := range lEnts {
		var cfg lookup.Config
		if err := json.Unmarshal(e.Data, &cfg); err != nil {
			continue
		}
		if cfg.Name == "" {
			cfg.Name = e.Name
		}
		if err := a.lookups.Load(ctx, cfg); err != nil {
			a.log.Warn("loading lookup table failed", "name", cfg.Name, "error", err)
		}
	}

	// Pipelines.
	pEnts, err := a.meta.ListEntities(ctx, meta.KindPipeline)
	if err != nil {
		return err
	}
	pipelines := make([]pipeline.Pipeline, 0, len(pEnts))
	for _, e := range pEnts {
		var p pipeline.Pipeline
		if err := json.Unmarshal(e.Data, &p); err != nil {
			a.log.Warn("skipping malformed pipeline", "id", e.ID, "error", err)
			continue
		}
		p.ID = e.ID
		pipelines = append(pipelines, p)
	}
	if err := a.engine.SetPipelines(pipelines); err != nil {
		return fmt.Errorf("installing pipelines: %w", err)
	}

	// Outputs.
	oEnts, err := a.meta.ListEntities(ctx, meta.KindOutput)
	if err != nil {
		return err
	}
	outs := make([]output.Output, 0, len(oEnts))
	for _, e := range oEnts {
		var o output.Output
		if err := json.Unmarshal(e.Data, &o); err != nil {
			continue
		}
		o.ID = e.ID
		outs = append(outs, o)
	}
	a.outputs.SetOutputs(outs)
	return nil
}

func (a *App) setPolicies(p []retention.StreamPolicy) {
	a.policyMu.Lock()
	a.policies = p
	a.policyMu.Unlock()
}

func (a *App) streamPolicies() []retention.StreamPolicy {
	a.policyMu.Lock()
	defer a.policyMu.Unlock()
	out := make([]retention.StreamPolicy, len(a.policies))
	copy(out, a.policies)
	return out
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

	a.retention.Start()
	a.events.Start()

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

	a.retention.Stop()
	a.events.Stop()
	if err := a.server.Shutdown(sctx); err != nil {
		a.log.Warn("http shutdown", "error", err)
	}
	if err := a.inputs.StopAll(sctx); err != nil {
		a.log.Warn("input shutdown", "error", err)
	}

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

	a.outputs.Stop()
	if err := a.store.Close(); err != nil {
		a.log.Warn("store close", "error", err)
	}
	if err := a.meta.Close(); err != nil {
		a.log.Warn("metadata store close", "error", err)
	}
	if a.geo != nil {
		_ = a.geo.Close()
	}
	a.log.Info("shutdown complete")
}

// stdlogBridge forwards standard-library log output into slog at debug level so
// dependency logs are preserved but demoted below the default level.
type stdlogBridge struct{ log *slog.Logger }

func (b stdlogBridge) Write(p []byte) (int, error) {
	b.log.Debug(strings.TrimRight(string(p), "\n"), "source", "stdlib")
	return len(p), nil
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
