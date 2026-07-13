// Package sqlite implements logstore.Store with one modernc.org/sqlite database
// per UTC day, each carrying an FTS5 index over the message body. Retention is a
// whole-file delete; cross-day search fans out over the partitions in range and
// merges. CGO stays disabled (pure-Go driver).
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/t0mer/tollan/internal/schema"
)

const (
	dayLayout    = "2006-01-02"
	defaultLimit = 100
)

// Store is a day-partitioned SQLite log store.
type Store struct {
	dir string

	mu  sync.Mutex
	dbs map[string]*sql.DB // day -> open handle
}

// Open creates (if needed) the store directory and returns a Store.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating log store dir: %w", err)
	}
	return &Store{dir: dir, dbs: make(map[string]*sql.DB)}, nil
}

func dayOf(t time.Time) string { return t.UTC().Format(dayLayout) }

func (s *Store) pathFor(day string) string {
	return filepath.Join(s.dir, day+".db")
}

// db returns the open handle for a day, opening and initializing it on demand.
func (s *Store) db(day string) (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.dbs[day]; ok {
		return db, nil
	}
	// WAL + synchronous=NORMAL balances throughput and durability; the ingest
	// journal (which commits only after a store succeeds) covers any writes lost
	// to a crash by replaying them on restart.
	dsn := "file:" + s.pathFor(day) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening partition %s: %w", day, err)
	}
	// A single connection sidesteps writer-lock contention; WAL still allows the
	// cross-day fan-out to read other partitions concurrently.
	db.SetMaxOpenConns(1)
	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	s.dbs[day] = db
	return db, nil
}

func initSchema(db *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS messages (
  rowid    INTEGER PRIMARY KEY,
  id       TEXT UNIQUE NOT NULL,
  ts       INTEGER NOT NULL,
  received INTEGER NOT NULL,
  source   TEXT,
  stream   TEXT,
  input_id TEXT,
  body     TEXT,
  fields   TEXT
);
CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(ts);
CREATE INDEX IF NOT EXISTS idx_messages_stream ON messages(stream);
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts
  USING fts5(body, content='messages', content_rowid='rowid');`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("initializing schema: %w", err)
	}
	return nil
}

// Store persists a batch of messages, grouping them by UTC day.
func (s *Store) Store(ctx context.Context, msgs []*schema.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	byDay := make(map[string][]*schema.Message)
	for _, m := range msgs {
		m.EnsureTimestamp()
		byDay[dayOf(m.Timestamp)] = append(byDay[dayOf(m.Timestamp)], m)
	}
	for day, batch := range byDay {
		if err := s.storeDay(ctx, day, batch); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) storeDay(ctx context.Context, day string, batch []*schema.Message) error {
	db, err := s.db(day)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	insMsg, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO messages
	  (id, ts, received, source, stream, input_id, body, fields)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insMsg.Close()
	insFTS, err := tx.PrepareContext(ctx, `INSERT INTO messages_fts (rowid, body) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer insFTS.Close()

	for _, m := range batch {
		fieldsJSON := "{}"
		if len(m.Fields) > 0 {
			b, err := json.Marshal(m.Fields)
			if err != nil {
				return fmt.Errorf("encoding fields: %w", err)
			}
			fieldsJSON = string(b)
		}
		res, err := insMsg.ExecContext(ctx,
			m.ID, m.Timestamp.UTC().UnixMilli(), m.ReceivedAt.UTC().UnixMilli(),
			m.Source, m.Stream, m.InputID, m.Body, fieldsJSON)
		if err != nil {
			return fmt.Errorf("inserting message: %w", err)
		}
		// Skip FTS insert for duplicates that were ignored.
		if affected, _ := res.RowsAffected(); affected == 0 {
			continue
		}
		rowid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := insFTS.ExecContext(ctx, rowid, m.Body); err != nil {
			return fmt.Errorf("inserting fts row: %w", err)
		}
	}
	return tx.Commit()
}

// Close closes all open partition handles.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for day, db := range s.dbs {
		if err := db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(s.dbs, day)
	}
	return firstErr
}
