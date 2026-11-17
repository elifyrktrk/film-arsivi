package store

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filmarsivi/web/internal/sources"
)

// TestIMDbLiveSync downloads the real dataset. Opt-in: IMDB_LIVE=1 go test ./internal/store -run Live -v
func TestIMDbLiveSync(t *testing.T) {
	if os.Getenv("IMDB_LIVE") != "1" {
		t.Skip("set IMDB_LIVE=1 to download the real IMDb dataset")
	}
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Now()
	n, err := sources.SyncIMDbRatings(ctx, &http.Client{Timeout: 10 * time.Minute}, sources.IMDbRatingsURL, db)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := db.IMDbRating(ctx, "tt0133093") // The Matrix
	t.Logf("rows=%d took=%s matrix=%+v", n, time.Since(start).Round(time.Millisecond), r)
	if n < 100000 || r == nil {
		t.Fatal("unexpected dataset")
	}
}
