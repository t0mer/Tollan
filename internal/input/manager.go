package input

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Build constructs an input from its config.
func Build(cfg Config, pub Publisher, log *slog.Logger) (Input, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	il := log.With("input", cfg.ID, "type", cfg.Type)
	switch cfg.Type {
	case "raw":
		return NewRaw(cfg, pub, il), nil
	case "syslog":
		return NewSyslog(cfg, pub, il), nil
	case "gelf":
		return NewGelf(cfg, pub, il), nil
	case "cef":
		return NewCEF(cfg, pub, il), nil
	case "httpjson":
		return NewHTTPJSON(cfg, pub, il), nil
	case "beats":
		return NewBeats(cfg, pub, il), nil
	case "netflow", "ipfix":
		return NewNetFlow(cfg, pub, il), nil
	case "docker":
		return NewDocker(cfg, pub, il), nil
	default:
		return nil, fmt.Errorf("input %q: unknown type %q", cfg.ID, cfg.Type)
	}
}

// Status is a snapshot of a running input for the API/UI.
type Status struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Bind     string   `json:"bind"`
	Protocol Protocol `json:"protocol"`
	Running  bool     `json:"running"`
}

// Manager starts, tracks and stops the configured inputs.
type Manager struct {
	log *slog.Logger

	mu      sync.Mutex
	inputs  map[string]Input
	configs map[string]Config
}

// NewManager returns an empty input manager.
func NewManager(log *slog.Logger) *Manager {
	return &Manager{
		log:     log,
		inputs:  make(map[string]Input),
		configs: make(map[string]Config),
	}
}

// StartAll builds and starts every configured input. On the first failure it
// stops the inputs already started and returns the error.
func (m *Manager) StartAll(cfgs []Config, pub Publisher) error {
	for _, cfg := range cfgs {
		in, err := Build(cfg, pub, m.log)
		if err != nil {
			_ = m.StopAll(context.Background())
			return err
		}
		if err := in.Start(); err != nil {
			_ = m.StopAll(context.Background())
			return fmt.Errorf("starting input %q: %w", cfg.ID, err)
		}
		m.mu.Lock()
		m.inputs[cfg.ID] = in
		m.configs[cfg.ID] = cfg
		m.mu.Unlock()
		m.log.Info("input started", "input", cfg.ID, "type", cfg.Type,
			"bind", cfg.Bind, "protocol", cfg.Protocol)
	}
	return nil
}

// StopAll stops all running inputs.
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	inputs := make([]Input, 0, len(m.inputs))
	for id, in := range m.inputs {
		inputs = append(inputs, in)
		delete(m.inputs, id)
	}
	m.mu.Unlock()

	var firstErr error
	for _, in := range inputs {
		if err := in.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// List returns the status of all managed inputs.
func (m *Manager) List() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.configs))
	for id, cfg := range m.configs {
		_, running := m.inputs[id]
		out = append(out, Status{
			ID: cfg.ID, Type: cfg.Type, Bind: cfg.Bind,
			Protocol: cfg.Protocol, Running: running,
		})
	}
	return out
}
