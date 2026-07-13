package meta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when an entity does not exist.
var ErrNotFound = errors.New("not found")

// SavedSearch is a named, reusable query with an optional time range.
type SavedSearch struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	TimeRange string    `json:"time_range"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListSavedSearches returns all saved searches, newest first.
func (s *Store) ListSavedSearches(ctx context.Context) ([]SavedSearch, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, query, time_range, created_at, updated_at
		 FROM saved_searches ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedSearch
	for rows.Next() {
		ss, err := scanSaved(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

// CreateSavedSearch inserts a new saved search, assigning an id and timestamps.
func (s *Store) CreateSavedSearch(ctx context.Context, name, query, timeRange string) (SavedSearch, error) {
	if name == "" {
		return SavedSearch{}, fmt.Errorf("name is required")
	}
	now := time.Now().UTC()
	ss := SavedSearch{
		ID: uuid.NewString(), Name: name, Query: query, TimeRange: timeRange,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO saved_searches (id, name, query, time_range, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ss.ID, ss.Name, ss.Query, ss.TimeRange, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return SavedSearch{}, err
	}
	return ss, nil
}

// UpdateSavedSearch updates an existing saved search.
func (s *Store) UpdateSavedSearch(ctx context.Context, id, name, query, timeRange string) (SavedSearch, error) {
	if name == "" {
		return SavedSearch{}, fmt.Errorf("name is required")
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE saved_searches SET name = ?, query = ?, time_range = ?, updated_at = ?
		 WHERE id = ?`,
		name, query, timeRange, now.UnixMilli(), id)
	if err != nil {
		return SavedSearch{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return SavedSearch{}, ErrNotFound
	}
	return s.GetSavedSearch(ctx, id)
}

// GetSavedSearch returns one saved search by id.
func (s *Store) GetSavedSearch(ctx context.Context, id string) (SavedSearch, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, query, time_range, created_at, updated_at
		 FROM saved_searches WHERE id = ?`, id)
	ss, err := scanSaved(row)
	if errors.Is(err, sql.ErrNoRows) {
		return SavedSearch{}, ErrNotFound
	}
	return ss, err
}

// DeleteSavedSearch removes a saved search by id.
func (s *Store) DeleteSavedSearch(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM saved_searches WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanner abstracts *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanSaved(sc scanner) (SavedSearch, error) {
	var ss SavedSearch
	var created, updated int64
	if err := sc.Scan(&ss.ID, &ss.Name, &ss.Query, &ss.TimeRange, &created, &updated); err != nil {
		return SavedSearch{}, err
	}
	ss.CreatedAt = time.UnixMilli(created).UTC()
	ss.UpdatedAt = time.UnixMilli(updated).UTC()
	return ss, nil
}
