// Package svc wraps Tollan in an OS service (systemd on Linux, SCM on Windows)
// via kardianos/service. The same wrapper drives foreground execution and
// service-managed execution, so `tollan run` behaves correctly in both.
package svc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kardianos/service"

	"github.com/t0mer/tollan/internal/app"
)

const (
	serviceName        = "tollan"
	serviceDisplayName = "Tollan Log Management Server"
	serviceDescription = "Self-hosted log management server (ingest, search, alerting)."
	stopGrace          = 35 * time.Second
)

// program adapts app.App to the kardianos service.Interface.
type program struct {
	app    *app.App
	log    *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}
}

// Start is called by the service manager; it must not block.
func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		if err := p.app.Run(ctx); err != nil {
			p.log.Error("tollan exited with error", "error", err)
		}
	}()
	return nil
}

// Stop is called by the service manager; it triggers graceful shutdown.
func (p *program) Stop(service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done != nil {
		select {
		case <-p.done:
		case <-time.After(stopGrace):
			p.log.Warn("shutdown timed out")
		}
	}
	return nil
}

// Options configures the service definition baked into the unit at install time.
type Options struct {
	// Arguments are the process arguments the service manager runs, typically
	// "run" plus the resolved --data-dir / --config flags.
	Arguments []string
	// UserName, if set, runs the Linux service as that user.
	UserName string
	// WorkingDirectory sets the unit's WorkingDirectory (usually the data dir).
	WorkingDirectory string
}

// systemdScript is a hardened unit template: restart on failure, run as a
// dedicated user, and grant CAP_NET_BIND_SERVICE so inputs can bind privileged
// ports (e.g. 514) if configured.
const systemdScript = `[Unit]
Description={{Description}}
ConditionFileIsExecutable={{Path | cmdEscape}}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{Path | cmdEscape}}{{range Arguments}} {{. | cmd}}{{end}}
{{if WorkingDirectory}}WorkingDirectory={{WorkingDirectory | cmdEscape}}
{{end}}{{if UserName}}User={{UserName}}
{{end}}Restart=on-failure
RestartSec=5
AmbientCapabilities=CAP_NET_BIND_SERVICE
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`

// New builds a kardianos service bound to the given App.
func New(a *app.App, log *slog.Logger, opts Options) (service.Service, error) {
	cfg := &service.Config{
		Name:             serviceName,
		DisplayName:      serviceDisplayName,
		Description:      serviceDescription,
		Arguments:        opts.Arguments,
		UserName:         opts.UserName,
		WorkingDirectory: opts.WorkingDirectory,
		Option: service.KeyValue{
			"Restart":       "on-failure",
			"SystemdScript": systemdScript,
		},
	}
	prog := &program{app: a, log: log}
	svc, err := service.New(prog, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating service: %w", err)
	}
	return svc, nil
}

// Interactive reports whether the process is running interactively (not under a
// service manager). Used to decide where logs should go.
func Interactive() bool {
	return service.Interactive()
}

// Run executes under the service manager (or interactively), blocking until the
// service is stopped. Used by `tollan run`.
func Run(svc service.Service) error {
	return svc.Run()
}

// Control performs an install/uninstall/start/stop/restart action.
func Control(svc service.Service, action string) error {
	return service.Control(svc, action)
}

// Status returns a human-readable status string for the service.
func Status(svc service.Service) (string, error) {
	st, err := svc.Status()
	if err != nil {
		return "", err
	}
	switch st {
	case service.StatusRunning:
		return "running", nil
	case service.StatusStopped:
		return "stopped", nil
	default:
		return "unknown", nil
	}
}
