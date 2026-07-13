// Package retention prunes expired log data on a schedule: whole day partitions
// past the global retention, plus per-stream row deletion for streams with a
// shorter retention than the global default.
package retention

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/t0mer/tollan/internal/logstore"
)

// StreamPolicy is a per-stream retention override.
type StreamPolicy struct {
	ID   string
	Days int
}

// Manager runs retention sweeps.
type Manager struct {
	store      logstore.Store
	globalDays int
	policies   func() []StreamPolicy
	log        *slog.Logger
	cron       *cron.Cron
}

// New creates a retention manager. policies returns the current per-stream
// overrides (evaluated each sweep so config changes take effect).
func New(store logstore.Store, globalDays int, policies func() []StreamPolicy, log *slog.Logger) *Manager {
	return &Manager{store: store, globalDays: globalDays, policies: policies, log: log}
}

// Start schedules a nightly sweep (03:00) and runs one immediately.
func (m *Manager) Start() {
	m.cron = cron.New()
	_, _ = m.cron.AddFunc("0 3 * * *", func() {
		if err := m.Sweep(context.Background()); err != nil {
			m.log.Error("retention sweep failed", "error", err)
		}
	})
	m.cron.Start()
	go func() {
		if err := m.Sweep(context.Background()); err != nil {
			m.log.Error("initial retention sweep failed", "error", err)
		}
	}()
}

// Stop halts the scheduler.
func (m *Manager) Stop() {
	if m.cron != nil {
		m.cron.Stop()
	}
}

// Sweep runs one retention pass.
func (m *Manager) Sweep(ctx context.Context) error {
	now := time.Now().UTC()
	if m.globalDays > 0 {
		cutoff := now.AddDate(0, 0, -m.globalDays)
		n, err := m.store.DropBefore(ctx, cutoff)
		if err != nil {
			return err
		}
		if n > 0 {
			m.log.Info("retention: dropped day partitions", "count", n, "older_than", cutoff.Format("2006-01-02"))
		}
	}
	for _, p := range m.policies() {
		if p.Days <= 0 || (m.globalDays > 0 && p.Days >= m.globalDays) {
			continue // covered by global partition retention
		}
		cutoff := now.AddDate(0, 0, -p.Days)
		n, err := m.store.DeleteStreamBefore(ctx, p.ID, cutoff)
		if err != nil {
			m.log.Error("retention: stream prune failed", "stream", p.ID, "error", err)
			continue
		}
		if n > 0 {
			m.log.Info("retention: pruned stream rows", "stream", p.ID, "rows", n)
		}
	}
	return nil
}
