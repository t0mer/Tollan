package meta

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event is a fired event occurrence (the searchable event stream).
type Event struct {
	ID             string    `json:"id"`
	DefinitionID   string    `json:"definition_id"`
	DefinitionName string    `json:"definition_name"`
	FiredAt        time.Time `json:"fired_at"`
	Message        string    `json:"message"`
	Count          int       `json:"count"`
	GroupKey       string    `json:"group_key"`
}

func (s *Store) migrateEvents() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS events (
  id              TEXT PRIMARY KEY,
  definition_id   TEXT NOT NULL,
  definition_name TEXT NOT NULL,
  fired_at        INTEGER NOT NULL,
  message         TEXT NOT NULL,
  count           INTEGER NOT NULL,
  group_key       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_fired ON events(fired_at);`
	_, err := s.db.Exec(ddl)
	return err
}

// InsertEvent records a fired event, assigning an id.
func (s *Store) InsertEvent(ctx context.Context, e Event) (Event, error) {
	e.ID = uuid.NewString()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (id, definition_id, definition_name, fired_at, message, count, group_key)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.DefinitionID, e.DefinitionName, e.FiredAt.UTC().UnixMilli(), e.Message, e.Count, e.GroupKey)
	return e, err
}

// ListEvents returns fired events, newest first.
func (s *Store) ListEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, definition_id, definition_name, fired_at, message, count, group_key
		 FROM events ORDER BY fired_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var fired int64
		if err := rows.Scan(&e.ID, &e.DefinitionID, &e.DefinitionName, &fired, &e.Message, &e.Count, &e.GroupKey); err != nil {
			return nil, err
		}
		e.FiredAt = time.UnixMilli(fired).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// PurgeEventsBefore deletes events older than cutoff.
func (s *Store) PurgeEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE fired_at < ?`, cutoff.UTC().UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
