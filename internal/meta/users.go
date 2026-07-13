package meta

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Roles.
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// User is a local account.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// APIToken is a named, revocable bearer token owned by a user.
type APIToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Hash      string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
}

func (s *Store) migrateUsers() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS users (
  id            TEXT PRIMARY KEY,
  username      TEXT UNIQUE NOT NULL,
  role          TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS api_tokens (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  name       TEXT NOT NULL,
  hash       TEXT UNIQUE NOT NULL,
  created_at INTEGER NOT NULL,
  last_used  INTEGER NOT NULL DEFAULT 0
);`
	_, err := s.db.Exec(ddl)
	return err
}

// CountUsers returns the number of users (0 means first-run/bootstrap).
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser inserts a user with a pre-computed password hash.
func (s *Store) CreateUser(ctx context.Context, username, role, passwordHash string) (User, error) {
	u := User{ID: uuid.NewString(), Username: username, Role: role, PasswordHash: passwordHash, CreatedAt: time.Now().UTC()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, username, role, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.Role, u.PasswordHash, u.CreatedAt.UnixMilli())
	return u, err
}

// GetUserByUsername looks up a user by username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, username, role, password_hash, created_at FROM users WHERE username = ?`, username))
}

// GetUser looks up a user by id.
func (s *Store) GetUser(ctx context.Context, id string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, username, role, password_hash, created_at FROM users WHERE id = ?`, id))
}

// ListUsers returns all users.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, role, password_hash, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUser updates a user's role and (if non-empty) password hash.
func (s *Store) UpdateUser(ctx context.Context, id, role, passwordHash string) error {
	if passwordHash != "" {
		_, err := s.db.ExecContext(ctx, `UPDATE users SET role = ?, password_hash = ? WHERE id = ?`, role, passwordHash, id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET role = ? WHERE id = ?`, role, id)
	return err
}

// DeleteUser removes a user and its tokens.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE user_id = ?`, id)
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateToken stores an API token by its hash.
func (s *Store) CreateToken(ctx context.Context, userID, name, hash string) (APIToken, error) {
	t := APIToken{ID: uuid.NewString(), UserID: userID, Name: name, Hash: hash, CreatedAt: time.Now().UTC()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (id, user_id, name, hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.Name, t.Hash, t.CreatedAt.UnixMilli())
	return t, err
}

// ListTokens returns a user's tokens.
func (s *Store) ListTokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, hash, created_at, last_used FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTokenByHash resolves a token hash to its record.
func (s *Store) GetTokenByHash(ctx context.Context, hash string) (APIToken, error) {
	return scanToken(s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, hash, created_at, last_used FROM api_tokens WHERE hash = ?`, hash))
}

// TouchToken records a token's last-used time.
func (s *Store) TouchToken(ctx context.Context, id string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used = ? WHERE id = ?`, time.Now().UTC().UnixMilli(), id)
}

// DeleteToken revokes a token owned by userID.
func (s *Store) DeleteToken(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanUser(sc scanner) (User, error) {
	var u User
	var created int64
	err := sc.Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt = time.UnixMilli(created).UTC()
	return u, nil
}

func scanToken(sc scanner) (APIToken, error) {
	var t APIToken
	var created, lastUsed int64
	err := sc.Scan(&t.ID, &t.UserID, &t.Name, &t.Hash, &created, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return APIToken{}, ErrNotFound
	}
	if err != nil {
		return APIToken{}, err
	}
	t.CreatedAt = time.UnixMilli(created).UTC()
	if lastUsed > 0 {
		t.LastUsed = time.UnixMilli(lastUsed).UTC()
	}
	return t, nil
}
