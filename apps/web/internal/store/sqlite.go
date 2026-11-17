package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"filmarsivi/web/internal/domain"

	_ "modernc.org/sqlite" // CGO-free SQLite driver
)

// SQLite is a domain.Store on a single SQLite file.
type SQLite struct{ db *sql.DB }

var _ domain.Store = (*SQLite)(nil)

const schema = `
CREATE TABLE IF NOT EXISTS titles (
  id            TEXT PRIMARY KEY,
  source        TEXT NOT NULL,
  source_id     TEXT NOT NULL,
  kind          TEXT NOT NULL,
  name          TEXT NOT NULL,
  original_name TEXT,
  year          INTEGER,
  poster_url    TEXT,
  overview      TEXT,
  genres        TEXT NOT NULL DEFAULT '[]',
  format        TEXT,
  season_count  INTEGER,
  episode_count INTEGER
);
CREATE TABLE IF NOT EXISTS ratings (
  user_id    TEXT NOT NULL,
  title_id   TEXT NOT NULL REFERENCES titles(id),
  score      INTEGER CHECK (score BETWEEN 1 AND 10),
  note       TEXT NOT NULL DEFAULT '',
  status     TEXT CHECK (status IN ('watched','planned','dropped')),
  updated_at TEXT NOT NULL,
  PRIMARY KEY (user_id, title_id)
);
CREATE TABLE IF NOT EXISTS categories (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL,
  name        TEXT NOT NULL,
  color       TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS categories_user ON categories(user_id, created_at);
CREATE TABLE IF NOT EXISTS category_items (
  category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  title_id    TEXT NOT NULL REFERENCES titles(id),
  position    INTEGER NOT NULL,
  added_at    TEXT NOT NULL,
  PRIMARY KEY (category_id, title_id)
);
CREATE INDEX IF NOT EXISTS category_items_title ON category_items(title_id);
`

// OpenSQLite opens (and migrates) the database at path. Use ":memory:" for tests.
func OpenSQLite(path string) (*SQLite, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1) // each connection would get its own empty database
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

type scanner interface{ Scan(dest ...any) error }

const titleCols = `t.id, t.source, t.source_id, t.kind, t.name, t.original_name, t.year, t.poster_url, t.overview, t.genres, t.format, t.season_count, t.episode_count`

func nullInt(p sql.NullInt64) *int {
	if !p.Valid {
		return nil
	}
	v := int(p.Int64)
	return &v
}

func nullStr(p sql.NullString) *string {
	if !p.Valid {
		return nil
	}
	return &p.String
}

func scanTitle(row scanner, extra ...any) (domain.Title, error) {
	var (
		t                              domain.Title
		orig, poster, overview, format sql.NullString
		year, seasons, episodes        sql.NullInt64
		genres                         string
	)
	dest := append([]any{&t.ID, &t.Source, &t.SourceID, &t.Kind, &t.Name, &orig, &year, &poster, &overview, &genres, &format, &seasons, &episodes}, extra...)
	if err := row.Scan(dest...); err != nil {
		return t, err
	}
	t.OriginalName, t.PosterURL, t.Overview, t.Format = nullStr(orig), nullStr(poster), nullStr(overview), nullStr(format)
	t.Year, t.SeasonCount, t.EpisodeCount = nullInt(year), nullInt(seasons), nullInt(episodes)
	if err := json.Unmarshal([]byte(genres), &t.Genres); err != nil || t.Genres == nil {
		t.Genres = []string{}
	}
	return t, nil
}

func (s *SQLite) GetTitle(ctx context.Context, id string) (*domain.Title, error) {
	t, err := scanTitle(s.db.QueryRowContext(ctx, `SELECT `+titleCols+` FROM titles t WHERE t.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *SQLite) UpsertTitle(ctx context.Context, t domain.Title) error {
	genres, _ := json.Marshal(t.Genres)
	if t.Genres == nil {
		genres = []byte("[]")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO titles (id, source, source_id, kind, name, original_name, year, poster_url, overview, genres, format, season_count, episode_count)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET source=excluded.source, source_id=excluded.source_id, kind=excluded.kind,
  name=excluded.name, original_name=excluded.original_name, year=excluded.year, poster_url=excluded.poster_url,
  overview=excluded.overview, genres=excluded.genres, format=excluded.format,
  season_count=excluded.season_count, episode_count=excluded.episode_count`,
		t.ID, t.Source, t.SourceID, t.Kind, t.Name, t.OriginalName, t.Year, t.PosterURL, t.Overview, string(genres), t.Format, t.SeasonCount, t.EpisodeCount)
	return err
}

func scanRatingTail(score *sql.NullInt64, status *sql.NullString, r *domain.Rating) {
	r.Score = nullInt(*score)
	if status.Valid {
		st := domain.WatchStatus(status.String)
		r.Status = &st
	}
}

func (s *SQLite) GetRating(ctx context.Context, userID, titleID string) (*domain.Rating, error) {
	var (
		r      domain.Rating
		score  sql.NullInt64
		status sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `SELECT title_id, score, note, status, updated_at FROM ratings WHERE user_id = ? AND title_id = ?`, userID, titleID).
		Scan(&r.TitleID, &score, &r.Note, &status, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	scanRatingTail(&score, &status, &r)
	return &r, nil
}

func (s *SQLite) UpsertRating(ctx context.Context, userID string, r domain.Rating) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ratings (user_id, title_id, score, note, status, updated_at) VALUES (?,?,?,?,?,?)
ON CONFLICT(user_id, title_id) DO UPDATE SET score=excluded.score, note=excluded.note, status=excluded.status, updated_at=excluded.updated_at`,
		userID, r.TitleID, r.Score, r.Note, r.Status, r.UpdatedAt)
	return err
}

func (s *SQLite) DeleteRating(ctx context.Context, userID, titleID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ratings WHERE user_id = ? AND title_id = ?`, userID, titleID)
	return err
}

func (s *SQLite) ListArchive(ctx context.Context, userID string, q domain.ArchiveQuery) ([]domain.ArchiveEntry, error) {
	query := `SELECT ` + titleCols + `, r.score, r.note, r.status, r.updated_at
FROM ratings r JOIN titles t ON t.id = r.title_id WHERE r.user_id = ?`
	args := []any{userID}
	if q.Status != "" {
		query += ` AND r.status = ?`
		args = append(args, q.Status)
	}
	query += ` ORDER BY t.id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ArchiveEntry{}
	for rows.Next() {
		var (
			r      domain.Rating
			score  sql.NullInt64
			status sql.NullString
		)
		t, err := scanTitle(rows, &score, &r.Note, &status, &r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		r.TitleID = t.ID
		scanRatingTail(&score, &status, &r)
		out = append(out, domain.ArchiveEntry{Title: t, Rating: r})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Sorting stays in Go so every store orders exactly like the TS core (tr-TR collation).
	domain.SortArchive(out, q.Sort)
	return out, nil
}

func (s *SQLite) ListCategories(ctx context.Context, userID string) ([]domain.CategoryWithCount, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id, c.name, c.color, c.description, c.created_at, c.updated_at,
  (SELECT COUNT(*) FROM category_items i WHERE i.category_id = c.id)
FROM categories c WHERE c.user_id = ? ORDER BY c.created_at, c.id`, userID)
	if err != nil {
		return nil, err
	}
	out := []domain.CategoryWithCount{}
	for rows.Next() {
		var c domain.CategoryWithCount
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.Description, &c.CreatedAt, &c.UpdatedAt, &c.ItemCount); err != nil {
			rows.Close()
			return nil, err
		}
		c.PreviewPosters = []string{}
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for k := range out {
		prow, err := s.db.QueryContext(ctx, `
SELECT t.poster_url FROM category_items i JOIN titles t ON t.id = i.title_id
WHERE i.category_id = ? AND t.poster_url IS NOT NULL AND t.poster_url != ''
ORDER BY i.added_at DESC, i.position DESC LIMIT 3`, out[k].ID)
		if err != nil {
			return nil, err
		}
		for prow.Next() {
			var u string
			if err := prow.Scan(&u); err != nil {
				prow.Close()
				return nil, err
			}
			out[k].PreviewPosters = append(out[k].PreviewPosters, u)
		}
		prow.Close()
	}
	return out, nil
}

func (s *SQLite) GetCategory(ctx context.Context, userID, id string) (*domain.Category, error) {
	var c domain.Category
	err := s.db.QueryRowContext(ctx, `SELECT id, name, color, description, created_at, updated_at FROM categories WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&c.ID, &c.Name, &c.Color, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *SQLite) InsertCategory(ctx context.Context, userID string, c domain.Category) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO categories (id, user_id, name, color, description, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		c.ID, userID, c.Name, c.Color, c.Description, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *SQLite) UpdateCategory(ctx context.Context, userID string, c domain.Category) error {
	_, err := s.db.ExecContext(ctx, `UPDATE categories SET name = ?, color = ?, description = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		c.Name, c.Color, c.Description, c.UpdatedAt, c.ID, userID)
	return err
}

func (s *SQLite) DeleteCategory(ctx context.Context, userID, id string) error {
	// Items go with the category (ON DELETE CASCADE); titles stay.
	_, err := s.db.ExecContext(ctx, `DELETE FROM categories WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

const ownsCategory = `category_id IN (SELECT id FROM categories WHERE id = ? AND user_id = ?)`

func (s *SQLite) ListCategoryItems(ctx context.Context, userID, categoryID string) ([]domain.CategoryItemWithTitle, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+titleCols+`, i.category_id, i.position, i.added_at
FROM category_items i JOIN titles t ON t.id = i.title_id
WHERE i.`+ownsCategory+` ORDER BY i.position`, categoryID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CategoryItemWithTitle{}
	for rows.Next() {
		var it domain.CategoryItemWithTitle
		t, err := scanTitle(rows, &it.CategoryID, &it.Position, &it.AddedAt)
		if err != nil {
			return nil, err
		}
		it.Title, it.TitleID = t, t.ID
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *SQLite) GetCategoryItem(ctx context.Context, userID, categoryID, titleID string) (*domain.CategoryItem, error) {
	var it domain.CategoryItem
	err := s.db.QueryRowContext(ctx, `SELECT category_id, title_id, position, added_at FROM category_items WHERE `+ownsCategory+` AND title_id = ?`,
		categoryID, userID, titleID).Scan(&it.CategoryID, &it.TitleID, &it.Position, &it.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (s *SQLite) InsertCategoryItem(ctx context.Context, userID string, item domain.CategoryItem) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO category_items (category_id, title_id, position, added_at)
SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM categories WHERE id = ? AND user_id = ?)`,
		item.CategoryID, item.TitleID, item.Position, item.AddedAt, item.CategoryID, userID)
	return err
}

func (s *SQLite) DeleteCategoryItem(ctx context.Context, userID, categoryID, titleID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM category_items WHERE `+ownsCategory+` AND title_id = ?`, categoryID, userID, titleID)
	return err
}

func (s *SQLite) MaxPosition(ctx context.Context, userID, categoryID string) (int, error) {
	var max int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) FROM category_items WHERE `+ownsCategory, categoryID, userID).Scan(&max)
	return max, err
}

func (s *SQLite) CategoryIDsForTitle(ctx context.Context, userID, titleID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.category_id FROM category_items i JOIN categories c ON c.id = i.category_id
WHERE i.title_id = ? AND c.user_id = ? ORDER BY i.rowid`, titleID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
