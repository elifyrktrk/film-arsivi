package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"filmarsivi/web/internal/auth"
)

var _ auth.Store = (*SQLite)(nil)

const authSchema = `
CREATE TABLE IF NOT EXISTS users (
  id            TEXT PRIMARY KEY,
  google_sub    TEXT NOT NULL UNIQUE,
  email         TEXT NOT NULL,
  name          TEXT NOT NULL DEFAULT '',
  avatar_url    TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL,
  last_login_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user ON sessions(user_id);
CREATE TABLE IF NOT EXISTS oauth_flows (
  state      TEXT PRIMARY KEY,
  nonce      TEXT NOT NULL,
  verifier   TEXT NOT NULL,
  next       TEXT NOT NULL DEFAULT '/',
  created_at TEXT NOT NULL
);
`

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func (s *SQLite) SaveOAuthFlow(ctx context.Context, f auth.Flow) error {
	// Old unfinished attempts are useless; drop them while we are here.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM oauth_flows WHERE created_at < ?`, ts(f.CreatedAt.Add(-auth.FlowTTL)))
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_flows (state, nonce, verifier, next, created_at) VALUES (?,?,?,?,?)`,
		f.State, f.Nonce, f.Verifier, f.Next, ts(f.CreatedAt))
	return err
}

func (s *SQLite) TakeOAuthFlow(ctx context.Context, state string) (*auth.Flow, error) {
	f := auth.Flow{State: state}
	var created string
	err := s.db.QueryRowContext(ctx, `DELETE FROM oauth_flows WHERE state = ? RETURNING nonce, verifier, next, created_at`, state).
		Scan(&f.Nonce, &f.Verifier, &f.Next, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f.CreatedAt = parseTS(created)
	return &f, nil
}

func (s *SQLite) UpsertGoogleUser(ctx context.Context, id auth.Identity) (auth.User, error) {
	now := ts(time.Now())
	u := auth.User{Email: id.Email, Name: id.Name, Picture: id.Picture}
	err := s.db.QueryRowContext(ctx, `
INSERT INTO users (id, google_sub, email, name, avatar_url, created_at, last_login_at) VALUES (?,?,?,?,?,?,?)
ON CONFLICT(google_sub) DO UPDATE SET email = excluded.email, name = excluded.name,
  avatar_url = excluded.avatar_url, last_login_at = excluded.last_login_at
RETURNING id`, "u_"+auth.RandomToken()[:20], id.Subject, id.Email, id.Name, id.Picture, now, now).Scan(&u.ID)
	return u, err
}

func (s *SQLite) CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at, created_at) VALUES (?,?,?,?)`,
		tokenHash, userID, ts(expires), ts(time.Now()))
	return err
}

func (s *SQLite) SessionUser(ctx context.Context, tokenHash string) (*auth.User, time.Time, error) {
	var (
		u   auth.User
		exp string
	)
	err := s.db.QueryRowContext(ctx, `
SELECT u.id, u.email, u.name, u.avatar_url, s.expires_at FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ?`, tokenHash).Scan(&u.ID, &u.Email, &u.Name, &u.Picture, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return &u, parseTS(exp), nil
}

func (s *SQLite) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *SQLite) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, ts(now))
	return err
}

const legacyClaimKey = "legacy_data_claimed_by"

// ClaimLegacyData moves single-user data to userID in one transaction, once.
// Ratings the user already has for the same title are kept as they are and the
// legacy copy stays under legacyUserID, so nothing is lost.
func (s *SQLite) ClaimLegacyData(ctx context.Context, legacyUserID, userID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var claimedBy string
	err = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, legacyClaimKey).Scan(&claimedBy)
	if err == nil {
		return false, nil // already claimed (by this or an earlier owner)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE ratings SET user_id = ? WHERE user_id = ?`, userID, legacyUserID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE categories SET user_id = ? WHERE user_id = ?`, userID, legacyUserID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?)`, legacyClaimKey, userID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
