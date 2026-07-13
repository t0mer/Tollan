// Package meta is Tollan's configuration/metadata store: a single SQLite
// database (data/tollan.db) holding application entities such as saved searches
// (and, in later phases, users, streams, pipelines, alerts...). It is kept
// entirely separate from the log partitions.
package meta

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store is the metadata database handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the metadata database at path and applies the
// schema migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("creating metadata dir: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening metadata db: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS saved_searches (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  query      TEXT NOT NULL,
  time_range TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrating metadata db: %w", err)
	}
	if err := s.migrateEntities(); err != nil {
		return fmt.Errorf("migrating entities: %w", err)
	}
	if err := s.migrateEvents(); err != nil {
		return fmt.Errorf("migrating events: %w", err)
	}
	if err := s.migrateUsers(); err != nil {
		return fmt.Errorf("migrating users: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }
