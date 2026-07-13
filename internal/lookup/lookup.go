// Package lookup provides in-memory, CSV-backed lookup tables usable from
// pipeline rules via lookup(table, key, target). Tables load from a file path or
// http(s) URL and can be refreshed on a schedule.
package lookup

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// SourceType is where a lookup table's CSV comes from.
type SourceType string

const (
	SourceFile SourceType = "file"
	SourceURL  SourceType = "url"
)

// Config describes a lookup table.
type Config struct {
	Name       string     `json:"name"`
	SourceType SourceType `json:"source_type"`
	Source     string     `json:"source"` // path or URL
	KeyColumn  string     `json:"key_column"`
	ValueColumn string    `json:"value_column"`
}

// Manager holds loaded lookup tables.
type Manager struct {
	mu     sync.RWMutex
	tables map[string]map[string]string
	client *http.Client
}

// NewManager returns an empty lookup manager.
func NewManager() *Manager {
	return &Manager{
		tables: make(map[string]map[string]string),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Lookup returns the value for a key in a table (implements dsl.Env in part).
func (m *Manager) Lookup(table, key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tables[table]
	if !ok {
		return "", false
	}
	v, ok := t[key]
	return v, ok
}

// Set replaces a table's data directly (used in tests and content-pack import).
func (m *Manager) Set(name string, data map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tables[name] = data
}

// Remove drops a table.
func (m *Manager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tables, name)
}

// Load loads (or reloads) a table from its source.
func (m *Manager) Load(ctx context.Context, cfg Config) error {
	data, err := m.fetch(ctx, cfg)
	if err != nil {
		return err
	}
	parsed, err := parseCSV(data, cfg.KeyColumn, cfg.ValueColumn)
	if err != nil {
		return fmt.Errorf("lookup %q: %w", cfg.Name, err)
	}
	m.Set(cfg.Name, parsed)
	return nil
}

func (m *Manager) fetch(ctx context.Context, cfg Config) (io.ReadCloser, error) {
	switch cfg.SourceType {
	case SourceURL:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.Source, nil)
		if err != nil {
			return nil, err
		}
		resp, err := m.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching lookup %q: %w", cfg.Name, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("fetching lookup %q: status %d", cfg.Name, resp.StatusCode)
		}
		return resp.Body, nil
	default:
		f, err := os.Open(cfg.Source)
		if err != nil {
			return nil, fmt.Errorf("opening lookup %q: %w", cfg.Name, err)
		}
		return f, nil
	}
}

// parseCSV reads a CSV with a header row and builds a key→value map from the
// named columns.
func parseCSV(r io.ReadCloser, keyCol, valCol string) (map[string]string, error) {
	defer r.Close()
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	keyIdx, valIdx := -1, -1
	for i, h := range header {
		switch strings.TrimSpace(h) {
		case keyCol:
			keyIdx = i
		case valCol:
			valIdx = i
		}
	}
	if keyIdx < 0 || valIdx < 0 {
		return nil, fmt.Errorf("columns %q/%q not found in header", keyCol, valCol)
	}
	out := map[string]string{}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if keyIdx < len(rec) && valIdx < len(rec) {
			out[rec[keyIdx]] = rec[valIdx]
		}
	}
	return out, nil
}
