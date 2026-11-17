package sources

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"filmarsivi/web/internal/domain"
)

const sampleTSV = "tconst\taverageRating\tnumVotes\n" +
	"tt0133093\t8.7\t2100345\n" +
	"tt0000001\t5.7\t2093\n" +
	"broken line\n" +
	"tt0000002\t\\N\t10\n" + // IMDb writes \N for missing values
	"nm0000001\t7.0\t5\n" + // not a title
	"tt0000003\t6.5\t165\n"

func TestParseIMDbRatings(t *testing.T) {
	var got []domain.IMDbRating
	n, err := ParseIMDbRatings(strings.NewReader(sampleTSV), func(r domain.IMDbRating) error { got = append(got, r); return nil })
	if err != nil || n != 3 || len(got) != 3 {
		t.Fatal(n, err, got)
	}
	if got[0] != (domain.IMDbRating{TConst: "tt0133093", Rating: 8.7, Votes: 2100345}) {
		t.Fatalf("%+v", got[0])
	}
}

type memSink struct {
	rows []domain.IMDbRating
}

func (m *memSink) ReplaceIMDbRatings(_ context.Context, fill func(add func(domain.IMDbRating) error) error) error {
	var next []domain.IMDbRating
	if err := fill(func(r domain.IMDbRating) error { next = append(next, r); return nil }); err != nil {
		return err // keep old rows
	}
	m.rows = next
	return nil
}

func TestSyncIMDbRatings(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(sampleTSV))
	gz.Close()
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	sink := &memSink{}
	n, err := SyncIMDbRatings(context.Background(), srv.Client(), srv.URL, sink)
	if err != nil || n != 3 || len(sink.rows) != 3 {
		t.Fatal(n, err)
	}
	fail = true
	_, err = SyncIMDbRatings(context.Background(), srv.Client(), srv.URL, sink)
	var se StatusError
	if !errors.As(err, &se) || len(sink.rows) != 3 {
		t.Fatal("failed download must keep previous data", err, len(sink.rows))
	}
}

func TestAniListAverageScoreAndTMDBIMDbID(t *testing.T) {
	m := frierenMedia()
	m.AverageScore = ip(89)
	if tl := MapAniListMedia(m); tl.AniListScore == nil || *tl.AniListScore != 89 {
		t.Fatal("averageScore not mapped")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("append_to_response") != "external_ids" {
			t.Error("detail must ask for external_ids")
		}
		switch r.URL.Path {
		case "/movie/603":
			w.Write([]byte(`{"id":603,"title":"Matrix","imdb_id":"tt0133093","external_ids":{"imdb_id":"tt0133093"}}`))
		case "/tv/1399":
			w.Write([]byte(`{"id":1399,"name":"Taht Oyunları","external_ids":{"imdb_id":"tt0944947"}}`))
		case "/tv/2":
			w.Write([]byte(`{"id":2,"name":"Yerli","external_ids":{"imdb_id":""}}`))
		}
	}))
	defer srv.Close()
	c := NewTMDB(srv.Client(), "k")
	c.BaseURL = srv.URL
	for id, want := range map[string]string{"movie:603": "tt0133093", "tv:1399": "tt0944947", "tv:2": ""} {
		tl, err := c.GetByID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if tl.IMDbID != nil {
			got = *tl.IMDbID
		}
		if got != want {
			t.Fatalf("%s: imdb id %q, want %q", id, got, want)
		}
	}
}
