package meta

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Entity kinds stored in the generic config table.
const (
	KindStream    = "stream"
	KindPipeline  = "pipeline"
	KindLookup    = "lookup"
	KindDashboard = "dashboard"
	KindEvent     = "event"
	KindChannel   = "channel"
	KindOutput    = "output"
)

// Entity is a stored configuration object. Data holds the JSON-encoded typed
// entity (a stream, pipeline, dashboard, ...).
type Entity struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func (s *Store) migrateEntities() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS entities (
  kind       TEXT NOT NULL,
  id         TEXT NOT NULL,
  name       TEXT NOT NULL,
  data       TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (kind, id)
);`
	_, err := s.db.Exec(ddl)
	return err
}

// ListEntities returns all entities of a kind, newest first.
func (s *Store) ListEntities(ctx context.Context, kind string) ([]Entity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT kind, id, name, data, created_at, updated_at
		 FROM entities WHERE kind = ? ORDER BY updated_at DESC`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEntity returns one entity.
func (s *Store) GetEntity(ctx context.Context, kind, id string) (Entity, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT kind, id, name, data, created_at, updated_at
		 FROM entities WHERE kind = ? AND id = ?`, kind, id)
	e, err := scanEntity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Entity{}, ErrNotFound
	}
	return e, err
}

// PutEntity inserts or updates an entity, preserving created_at on update.
func (s *Store) PutEntity(ctx context.Context, kind, id, name string, data json.RawMessage) (Entity, error) {
	now := time.Now().UTC()
	nowMs := now.UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO entities (kind, id, name, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(kind, id) DO UPDATE SET name = excluded.name, data = excluded.data, updated_at = excluded.updated_at`,
		kind, id, name, string(data), nowMs, nowMs)
	if err != nil {
		return Entity{}, err
	}
	return s.GetEntity(ctx, kind, id)
}

// DeleteEntity removes an entity.
func (s *Store) DeleteEntity(ctx context.Context, kind, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM entities WHERE kind = ? AND id = ?`, kind, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanEntity(sc scanner) (Entity, error) {
	var e Entity
	var data string
	var created, updated int64
	if err := sc.Scan(&e.Kind, &e.ID, &e.Name, &data, &created, &updated); err != nil {
		return Entity{}, err
	}
	e.Data = json.RawMessage(data)
	e.CreatedAt = time.UnixMilli(created).UTC()
	e.UpdatedAt = time.UnixMilli(updated).UTC()
	return e, nil
}
