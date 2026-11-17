package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"filmarsivi/web/internal/domain"
)

const (
	TMDBAPI          = "https://api.themoviedb.org/3"
	tmdbImg          = "https://image.tmdb.org/t/p/w342"
	animationGenreID = 16
)

// TMDBAttribution is the notice TMDB requires apps to show.
const TMDBAttribution = "This product uses the TMDB API but is not endorsed or certified by TMDB."

type tmdbGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// TMDBItem covers both /search/multi results and /movie|/tv details.
type TMDBItem struct {
	ID               int         `json:"id"`
	MediaType        string      `json:"media_type"`
	Title            *string     `json:"title"`
	Name             *string     `json:"name"`
	OriginalTitle    *string     `json:"original_title"`
	OriginalName     *string     `json:"original_name"`
	ReleaseDate      *string     `json:"release_date"`
	FirstAirDate     *string     `json:"first_air_date"`
	PosterPath       *string     `json:"poster_path"`
	Overview         *string     `json:"overview"`
	GenreIDs         []int       `json:"genre_ids"`
	Genres           []tmdbGenre `json:"genres"`
	OriginalLanguage string      `json:"original_language"`
	NumberOfSeasons  *int        `json:"number_of_seasons"`
	NumberOfEpisodes *int        `json:"number_of_episodes"`
}

func yearOf(date *string) *int {
	if date == nil || len(*date) < 4 {
		return nil
	}
	y, err := strconv.Atoi((*date)[:4])
	if err != nil || y <= 0 {
		return nil
	}
	return &y
}

func firstNonNil(a, b *string) *string {
	if a != nil {
		return a
	}
	return b
}

// IsJapaneseAnimation: Japanese animation on TMDB is shown as anime so it can be
// merged with AniList.
func IsJapaneseAnimation(it TMDBItem) bool {
	ids := it.GenreIDs
	if ids == nil {
		for _, g := range it.Genres {
			ids = append(ids, g.ID)
		}
	}
	for _, id := range ids {
		if id == animationGenreID {
			return it.OriginalLanguage == "ja"
		}
	}
	return false
}

// MapTMDBItem converts a TMDB item to a Title. genreNames resolves search genre ids.
func MapTMDBItem(it TMDBItem, genreNames func([]int) []string) domain.Title {
	typ := "movie"
	if it.MediaType == "tv" {
		typ = "tv"
	}
	name := "#" + strconv.Itoa(it.ID)
	if n := firstNonNil(it.Title, it.Name); n != nil {
		name = *n
	}
	original := firstNonNil(it.OriginalTitle, it.OriginalName)
	if original != nil && *original == name {
		original = nil
	}
	var genres []string
	if it.Genres != nil {
		for _, g := range it.Genres {
			genres = append(genres, g.Name)
		}
	} else {
		genres = genreNames(it.GenreIDs)
	}
	if genres == nil {
		genres = []string{}
	}
	kind := domain.MediaKind(typ)
	if IsJapaneseAnimation(it) {
		kind = domain.KindAnime
	}
	var poster *string
	if it.PosterPath != nil && *it.PosterPath != "" {
		p := tmdbImg + *it.PosterPath
		poster = &p
	}
	sourceID := typ + ":" + strconv.Itoa(it.ID)
	t := domain.Title{
		ID: domain.MakeTitleID(domain.SourceTMDB, sourceID), Source: domain.SourceTMDB, SourceID: sourceID,
		Kind: kind, Name: name, OriginalName: original, Year: yearOf(firstNonNil(it.ReleaseDate, it.FirstAirDate)),
		PosterURL: poster, Overview: domain.CleanText(it.Overview), Genres: genres,
	}
	if typ == "tv" {
		t.SeasonCount, t.EpisodeCount = it.NumberOfSeasons, it.NumberOfEpisodes
	}
	return t
}

// TMDB is a client for the TMDB v3 API. Key may be a v3 API key or a v4 read access
// token (starts with "eyJ"), which is sent as a Bearer header instead.
type TMDB struct {
	HTTP     *http.Client
	Key      string
	Language string
	BaseURL  string

	mu     sync.Mutex
	genres map[int]string
}

func NewTMDB(c *http.Client, key string) *TMDB {
	return &TMDB{HTTP: c, Key: key, Language: "tr-TR", BaseURL: TMDBAPI}
}

func (t *TMDB) get(ctx context.Context, path string, params url.Values, out any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("language", t.Language)
	bearer := strings.HasPrefix(t.Key, "eyJ")
	if !bearer {
		params.Set("api_key", t.Key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.BaseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+t.Key)
	}
	res, err := t.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return StatusError{Source: "TMDB", Code: res.StatusCode}
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (t *TMDB) genreMap(ctx context.Context) (map[int]string, error) {
	t.mu.Lock()
	cached := t.genres
	t.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	m := map[int]string{}
	for _, kind := range []string{"movie", "tv"} {
		var data struct {
			Genres []tmdbGenre `json:"genres"`
		}
		if err := t.get(ctx, "/genre/"+kind+"/list", nil, &data); err != nil {
			return nil, err
		}
		for _, g := range data.Genres {
			m[g.ID] = g.Name
		}
	}
	t.mu.Lock()
	t.genres = m
	t.mu.Unlock()
	return m, nil
}

func (t *TMDB) Search(ctx context.Context, query string) ([]domain.Title, error) {
	var (
		wg   sync.WaitGroup
		gmap map[int]string
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		gmap, _ = t.genreMap(ctx) // genre names are optional
	}()
	var data struct {
		Results []TMDBItem `json:"results"`
	}
	err := t.get(ctx, "/search/multi", url.Values{"query": {query}, "include_adult": {"false"}, "page": {"1"}}, &data)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	names := func(ids []int) []string {
		var out []string
		for _, id := range ids {
			if n, ok := gmap[id]; ok && n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	out := []domain.Title{}
	for _, r := range data.Results {
		if r.MediaType == "movie" || r.MediaType == "tv" {
			out = append(out, MapTMDBItem(r, names))
		}
	}
	return out, nil
}

// GetByID takes `movie:603` or `tv:1399`.
func (t *TMDB) GetByID(ctx context.Context, sourceID string) (*domain.Title, error) {
	typ, id, _ := strings.Cut(sourceID, ":")
	if (typ != "movie" && typ != "tv") || id == "" {
		return nil, nil
	}
	if _, err := strconv.Atoi(id); err != nil {
		return nil, nil
	}
	var it TMDBItem
	if err := t.get(ctx, "/"+typ+"/"+id, nil, &it); err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	it.MediaType = typ
	if it.Genres == nil {
		it.Genres = []tmdbGenre{}
	}
	title := MapTMDBItem(it, func([]int) []string { return nil })
	return &title, nil
}
