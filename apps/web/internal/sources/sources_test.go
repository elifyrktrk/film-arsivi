package sources

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"filmarsivi/web/internal/domain"
)

func sp(s string) *string { return &s }
func ip(i int) *int       { return &i }

func frierenMedia() AniListMedia {
	var m AniListMedia
	m.ID = 154587
	m.Title.Romaji, m.Title.English, m.Title.Native = sp("Sousou no Frieren"), sp("Frieren: Beyond Journey's End"), sp("葬送のフリーレン")
	m.Format, m.Episodes, m.SeasonYear = sp("TV"), ip(28), ip(2023)
	m.CoverImage = &struct {
		Large *string `json:"large"`
	}{Large: sp("https://img/x.jpg")}
	m.Description = sp("Elf mage<br><br>Frieren &amp; friends <i>travel</i>.")
	m.Genres = []string{"Adventure", "Drama"}
	return m
}

// Mirrors "AniList mapping › maps media and strips HTML from descriptions".
func TestAniListMapping(t *testing.T) {
	tl := MapAniListMedia(frierenMedia())
	if tl.ID != "anilist:154587" || tl.Kind != domain.KindAnime || tl.Name != "Frieren: Beyond Journey's End" ||
		*tl.OriginalName != "Sousou no Frieren" || *tl.EpisodeCount != 28 || *tl.Overview != "Elf mage\n\nFrieren & friends travel." {
		t.Fatalf("%+v", tl)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || !strings.Contains(string(body), `"search":"frieren"`) {
			t.Errorf("unexpected request %s %s", r.Method, body)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Page": map[string]any{"media": []AniListMedia{frierenMedia()}}}})
	}))
	defer srv.Close()
	c := &AniList{HTTP: srv.Client(), Endpoint: srv.URL}
	res, err := c.Search(context.Background(), "frieren")
	if err != nil || len(res) != 1 || res[0].ID != "anilist:154587" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestAniListNotFoundIsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"data":{"Media":null}}`))
	}))
	defer srv.Close()
	got, err := (&AniList{HTTP: srv.Client(), Endpoint: srv.URL}).GetByID(context.Background(), "999999999")
	if err != nil || got != nil {
		t.Fatal(got, err)
	}
}

func TestTMDB(t *testing.T) {
	var lastAuth, lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth, lastQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		switch r.URL.Path {
		case "/genre/movie/list":
			w.Write([]byte(`{"genres":[{"id":16,"name":"Animasyon"},{"id":18,"name":"Dram"}]}`))
		case "/genre/tv/list":
			w.Write([]byte(`{"genres":[]}`))
		case "/search/multi":
			w.Write([]byte(`{"results":[
			  {"id":1,"media_type":"movie","title":"Ruhların Kaçışı","original_title":"千と千尋の神隠し","release_date":"2001-07-20","poster_path":"/a.jpg","genre_ids":[16],"original_language":"ja"},
			  {"id":2,"media_type":"person","name":"Biri"},
			  {"id":3,"media_type":"tv","name":"Dizi","original_name":"Dizi","first_air_date":"","genre_ids":[18],"original_language":"tr"}]}`))
		case "/tv/1399":
			w.Write([]byte(`{"id":1399,"name":"Taht Oyunları","first_air_date":"2011-04-17","genres":[{"id":18,"name":"Dram"}],"number_of_seasons":8,"number_of_episodes":73}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewTMDB(srv.Client(), "v3key")
	c.BaseURL = srv.URL
	res, err := c.Search(context.Background(), "x")
	if err != nil || len(res) != 2 {
		t.Fatal(err, res)
	}
	if res[0].Kind != domain.KindAnime || res[0].Genres[0] != "Animasyon" || *res[0].Year != 2001 || *res[0].PosterURL != tmdbImg+"/a.jpg" {
		t.Fatalf("Japanese animation should map to anime: %+v", res[0])
	}
	if res[1].Kind != domain.KindTV || res[1].OriginalName != nil || res[1].Year != nil {
		t.Fatalf("%+v", res[1])
	}
	if !strings.Contains(lastQuery, "api_key=v3key") || lastAuth != "" || !strings.Contains(lastQuery, "language=tr-TR") {
		t.Fatal("v3 key goes in the query", lastQuery, lastAuth)
	}

	c.Key = "eyJtoken"
	tv, err := c.GetByID(context.Background(), "tv:1399")
	if err != nil || tv.ID != "tmdb:tv:1399" || *tv.SeasonCount != 8 || *tv.EpisodeCount != 73 || tv.Genres[0] != "Dram" {
		t.Fatalf("%v %+v", err, tv)
	}
	if lastAuth != "Bearer eyJtoken" || strings.Contains(lastQuery, "api_key") {
		t.Fatal("v4 token goes in the Authorization header")
	}
	if got, err := c.GetByID(context.Background(), "movie:404"); got != nil || err != nil {
		t.Fatal("missing title should be nil", got, err)
	}
	if got, _ := c.GetByID(context.Background(), "person:1"); got != nil {
		t.Fatal("unknown type")
	}
}
