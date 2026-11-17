package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"filmarsivi/web/internal/domain"

	_ "modernc.org/sqlite"
)

var ctx = context.Background()

func TestIMDbRatingsReplaceAndLookup(t *testing.T) {
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if at, _ := db.IMDbUpdatedAt(ctx); !at.IsZero() {
		t.Fatal("never synced should be zero time")
	}
	put := func(rows ...domain.IMDbRating) func(func(domain.IMDbRating) error) error {
		return func(add func(domain.IMDbRating) error) error {
			for _, r := range rows {
				if err := add(r); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if err := db.ReplaceIMDbRatings(ctx, put(domain.IMDbRating{TConst: "tt1", Rating: 8.7, Votes: 100})); err != nil {
		t.Fatal(err)
	}
	r, _ := db.IMDbRating(ctx, "tt1")
	if r == nil || r.Rating != 8.7 || r.Votes != 100 {
		t.Fatalf("%+v", r)
	}
	if at, _ := db.IMDbUpdatedAt(ctx); at.IsZero() {
		t.Fatal("sync time not stored")
	}
	// a failing refresh rolls back and keeps the old rows
	err = db.ReplaceIMDbRatings(ctx, func(add func(domain.IMDbRating) error) error {
		add(domain.IMDbRating{TConst: "tt2", Rating: 5, Votes: 1})
		return errors.New("download cut off")
	})
	if err == nil {
		t.Fatal("want error")
	}
	if r, _ := db.IMDbRating(ctx, "tt1"); r == nil {
		t.Fatal("old data lost")
	}
	if r, _ := db.IMDbRating(ctx, "tt2"); r != nil {
		t.Fatal("partial data kept")
	}
}

func TestMigratesOldTitlesTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// schema of the first release, without imdb_id / anilist_score
	_, err = raw.Exec(`CREATE TABLE titles (id TEXT PRIMARY KEY, source TEXT NOT NULL, source_id TEXT NOT NULL, kind TEXT NOT NULL,
		name TEXT NOT NULL, original_name TEXT, year INTEGER, poster_url TEXT, overview TEXT, genres TEXT NOT NULL DEFAULT '[]',
		format TEXT, season_count INTEGER, episode_count INTEGER);
		INSERT INTO titles (id, source, source_id, kind, name) VALUES ('tmdb:movie:603','tmdb','movie:603','movie','Matrix');`)
	raw.Close()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.GetTitle(ctx, "tmdb:movie:603")
	if err != nil || got == nil || got.IMDbID != nil {
		t.Fatal(got, err)
	}
	imdb := "tt0133093"
	got.IMDbID = &imdb
	if err := db.UpsertTitle(ctx, *got); err != nil {
		t.Fatal(err)
	}
	// a later upsert without an imdb id (e.g. from search) must not erase it
	got.IMDbID = nil
	db.UpsertTitle(ctx, *got)
	if again, _ := db.GetTitle(ctx, "tmdb:movie:603"); again.IMDbID == nil || *again.IMDbID != imdb {
		t.Fatal("imdb id lost")
	}
}
