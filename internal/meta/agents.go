package meta

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// AgentConfig is the collector configuration pushed to an agent.
type AgentConfig struct {
	Version         int          `json:"version"`
	Files           []FileSource `json:"files"`
	Journald        bool         `json:"journald"`
	WindowsEventLog bool         `json:"windows_event_log"`
}

// FileSource describes a set of files to tail.
type FileSource struct {
	Paths            []string `json:"paths"`             // glob patterns
	MultilinePattern string   `json:"multiline_pattern"` // lines matching start a new event
}

// Agent is an enrolled collector.
type Agent struct {
	ID            string      `json:"id"`
	Hostname      string      `json:"hostname"`
	OS            string      `json:"os"`
	Version       string      `json:"version"`
	Tags          []string    `json:"tags"`
	EnrolledAt    time.Time   `json:"enrolled_at"`
	LastSeen      time.Time   `json:"last_seen"`
	Shipped       int64       `json:"shipped"`
	ConfigVersion int         `json:"config_version"`
	Config        AgentConfig `json:"config"`
	// SecretHash is the hash of the per-agent secret used to authenticate
	// heartbeat and config-poll requests; never serialized.
	SecretHash string `json:"-"`
}

func (s *Store) migrateAgents() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS agents (
  id             TEXT PRIMARY KEY,
  hostname       TEXT NOT NULL,
  os             TEXT NOT NULL DEFAULT '',
  version        TEXT NOT NULL DEFAULT '',
  tags           TEXT NOT NULL DEFAULT '[]',
  enrolled_at    INTEGER NOT NULL,
  last_seen      INTEGER NOT NULL DEFAULT 0,
  shipped        INTEGER NOT NULL DEFAULT 0,
  config         TEXT NOT NULL DEFAULT '{}',
  config_version INTEGER NOT NULL DEFAULT 0,
  secret_hash    TEXT NOT NULL DEFAULT ''
);`
	if _, err := s.db.Exec(ddl); err != nil {
		return err
	}
	// Add the secret_hash column to pre-existing tables (idempotent).
	_, _ = s.db.Exec(`ALTER TABLE agents ADD COLUMN secret_hash TEXT NOT NULL DEFAULT ''`)
	return nil
}

// UpsertAgent registers or updates an agent's identity on enrollment, rotating
// its authentication secret.
func (s *Store) UpsertAgent(ctx context.Context, a Agent) error {
	tags, _ := json.Marshal(a.Tags)
	cfg, _ := json.Marshal(a.Config)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agents (id, hostname, os, version, tags, enrolled_at, last_seen, config, config_version, secret_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET hostname=excluded.hostname, os=excluded.os, version=excluded.version, last_seen=excluded.last_seen, secret_hash=excluded.secret_hash`,
		a.ID, a.Hostname, a.OS, a.Version, string(tags),
		a.EnrolledAt.UnixMilli(), time.Now().UTC().UnixMilli(), string(cfg), a.ConfigVersion, a.SecretHash)
	return err
}

// Heartbeat updates an agent's last-seen time and shipped counter.
func (s *Store) Heartbeat(ctx context.Context, id string, shipped int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET last_seen = ?, shipped = ? WHERE id = ?`,
		time.Now().UTC().UnixMilli(), shipped, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetAgentConfig replaces an agent's collector config and bumps the version.
func (s *Store) SetAgentConfig(ctx context.Context, id string, cfg AgentConfig, tags []string) error {
	cfg.Version++
	cfgJSON, _ := json.Marshal(cfg)
	tagsJSON, _ := json.Marshal(tags)
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET config = ?, config_version = ?, tags = ? WHERE id = ?`,
		string(cfgJSON), cfg.Version, string(tagsJSON), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAgent returns one agent.
func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) {
	return scanAgent(s.db.QueryRowContext(ctx,
		`SELECT id, hostname, os, version, tags, enrolled_at, last_seen, shipped, config, config_version, secret_hash FROM agents WHERE id = ?`, id))
}

// ListAgents returns all agents, most-recently-seen first.
func (s *Store) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, hostname, os, version, tags, enrolled_at, last_seen, shipped, config, config_version, secret_hash FROM agents ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAgent removes an agent.
func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanAgent(sc scanner) (Agent, error) {
	var a Agent
	var tags, cfg string
	var enrolled, lastSeen int64
	err := sc.Scan(&a.ID, &a.Hostname, &a.OS, &a.Version, &tags, &enrolled, &lastSeen, &a.Shipped, &cfg, &a.ConfigVersion, &a.SecretHash)
	if errors.Is(err, sql.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, err
	}
	_ = json.Unmarshal([]byte(tags), &a.Tags)
	_ = json.Unmarshal([]byte(cfg), &a.Config)
	a.EnrolledAt = time.UnixMilli(enrolled).UTC()
	if lastSeen > 0 {
		a.LastSeen = time.UnixMilli(lastSeen).UTC()
	}
	return a, nil
}
