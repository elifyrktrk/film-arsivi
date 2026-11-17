package sources

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"filmarsivi/web/internal/domain"
)

func media(id int, english string) AniListMedia {
	var m AniListMedia
	m.ID = id
	m.Title.English = sp(english)
	return m
}

// fakeAniList answers word searches from `search` and id_in queries from `byID`.
func fakeAniList(t *testing.T, search map[string][]AniListMedia, byID map[int]AniListMedia, calls *int32, status int) *AniList {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.Unmarshal(body, &req)
		var out []AniListMedia
		if strings.Contains(req.Query, "id_in") {
			ids := req.Variables["ids"].([]any)
			for i := len(ids) - 1; i >= 0; i-- { // reversed: the client must restore order
				if m, ok := byID[int(ids[i].(float64))]; ok {
					out = append(out, m)
				}
			}
		} else {
			out = search[req.Variables["search"].(string)]
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Page": map[string]any{"media": out}}})
	}))
	t.Cleanup(srv.Close)
	return &AniList{HTTP: srv.Client(), Endpoint: srv.URL}
}

// fakeKitsu returns a JSON:API page where each entry maps to the given AniList ids
// (0 = an entry without an AniList mapping).
func fakeKitsu(t *testing.T, anilistIDs []int, calls *int32, fail bool) *Kitsu {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("include") != "mappings" || r.URL.Query().Get("filter[text]") == "" {
			t.Errorf("unexpected Kitsu query %s", r.URL.RawQuery)
		}
		var data, included []map[string]any
		for i, id := range anilistIDs {
			mal := map[string]any{"type": "mappings", "id": "mal" + strconv.Itoa(i), "attributes": map[string]any{"externalSite": "myanimelist/anime", "externalId": "1"}}
			refs := []map[string]any{{"type": "mappings", "id": "mal" + strconv.Itoa(i)}}
			included = append(included, mal)
			if id != 0 {
				refs = append(refs, map[string]any{"type": "mappings", "id": "al" + strconv.Itoa(i)})
				included = append(included, map[string]any{"type": "mappings", "id": "al" + strconv.Itoa(i), "attributes": map[string]any{"externalSite": "anilist/anime", "externalId": strconv.Itoa(id)}})
			}
			data = append(data, map[string]any{"id": strconv.Itoa(1000 + i), "type": "anime", "relationships": map[string]any{"mappings": map[string]any{"data": refs}}})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data, "included": included})
	}))
	t.Cleanup(srv.Close)
	return &Kitsu{HTTP: srv.Client(), BaseURL: srv.URL}
}

func ids(ts []domain.Title) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

func TestAnimeFallsBackToKitsuForPartialWords(t *testing.T) {
	var aCalls, kCalls int32
	al := fakeAniList(t,
		map[string][]AniListMedia{"frie": {}},
		map[int]AniListMedia{154587: media(154587, "Frieren: Beyond Journey's End"), 21: media(21, "One Piece")},
		&aCalls, 0)
	// Kitsu: Frieren, an entry without AniList mapping (skipped), One Piece
	k := fakeKitsu(t, []int{154587, 0, 21}, &kCalls, false)
	got, err := (&Anime{AniList: al, Kitsu: k}).Search(context.Background(), "frie")
	if err != nil {
		t.Fatal(err)
	}
	if g := strings.Join(ids(got), ","); g != "anilist:154587,anilist:21" {
		t.Fatalf("got %s", g)
	}
	if got[0].Source != domain.SourceAniList {
		t.Fatal("fallback results must look like AniList results")
	}
	if aCalls != 2 || kCalls != 1 {
		t.Fatalf("want 1 extra AniList + 1 Kitsu request, got anilist=%d kitsu=%d", aCalls, kCalls)
	}
}

func TestAnimeSkipsFallbackWhenAniListHasEnough(t *testing.T) {
	var aCalls, kCalls int32
	five := []AniListMedia{media(1, "A"), media(2, "B"), media(3, "C"), media(4, "D"), media(5, "E")}
	al := fakeAniList(t, map[string][]AniListMedia{"frieren": five}, nil, &aCalls, 0)
	k := fakeKitsu(t, []int{154587}, &kCalls, false)
	got, _ := (&Anime{AniList: al, Kitsu: k}).Search(context.Background(), "frieren")
	if len(got) != 5 || aCalls != 1 {
		t.Fatalf("no id_in query expected: %d results, %d AniList calls", len(got), aCalls)
	}
}

func TestAnimeFallbackIsBestEffort(t *testing.T) {
	var aCalls, kCalls int32
	al := fakeAniList(t, map[string][]AniListMedia{"jujut": {media(9, "Jujutsu Kaisen")}}, nil, &aCalls, 0)
	k := fakeKitsu(t, nil, &kCalls, true)
	got, err := (&Anime{AniList: al, Kitsu: k}).Search(context.Background(), "jujut")
	if err != nil || len(got) != 1 {
		t.Fatal("a Kitsu failure must not fail the search", err, got)
	}

	var c2 int32
	limited := fakeAniList(t, nil, nil, &c2, http.StatusTooManyRequests)
	_, err = (&Anime{AniList: limited, Kitsu: k}).Search(context.Background(), "x y")
	var rl interface{ RateLimited() bool }
	if !errors.As(err, &rl) || !rl.RateLimited() {
		t.Fatal("429 must be reported as rate limited", err)
	}
}

type countingSource struct {
	calls int
	err   error
}

func (c *countingSource) Search(context.Context, string) ([]domain.Title, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return []domain.Title{{ID: "anilist:1"}}, nil
}
func (c *countingSource) GetByID(context.Context, string) (*domain.Title, error) { return nil, nil }

func TestCachedSearch(t *testing.T) {
	src := &countingSource{}
	now := time.Now()
	c := NewCached(src, 10*time.Minute)
	c.Now = func() time.Time { return now }
	for _, q := range []string{"frieren", "Frieren", "  FRIEREN "} {
		if got, _ := c.Search(context.Background(), q); len(got) != 1 {
			t.Fatal(got)
		}
	}
	if src.calls != 1 {
		t.Fatalf("same query within 10 min must be served from cache, got %d calls", src.calls)
	}
	now = now.Add(11 * time.Minute)
	c.Search(context.Background(), "frieren")
	if src.calls != 2 {
		t.Fatal("expired entry not refreshed")
	}

	failing := &countingSource{err: errors.New("down")}
	fc := NewCached(failing, time.Minute)
	fc.Search(context.Background(), "x")
	fc.Search(context.Background(), "x")
	if failing.calls != 2 {
		t.Fatal("errors must not be cached")
	}
}
