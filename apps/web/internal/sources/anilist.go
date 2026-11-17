// Package sources holds the TMDB and AniList clients (Go port of packages/core/src/sources).
package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"filmarsivi/web/internal/domain"
)

const AniListEndpoint = "https://graphql.anilist.co"

const mediaFields = `
  id
  title { romaji english native }
  format
  episodes
  seasonYear
  startDate { year }
  coverImage { large }
  description(asHtml: false)
  genres
  averageScore
`

const searchQuery = `
query ($search: String, $perPage: Int) {
  Page(perPage: $perPage) {
    media(search: $search, type: ANIME, isAdult: false, sort: SEARCH_MATCH) {` + mediaFields + `}
  }
}`

const detailQuery = `
query ($id: Int) {
  Media(id: $id, type: ANIME) {` + mediaFields + `}
}`

type AniListMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  *string `json:"romaji"`
		English *string `json:"english"`
		Native  *string `json:"native"`
	} `json:"title"`
	Format     *string `json:"format"`
	Episodes   *int    `json:"episodes"`
	SeasonYear *int    `json:"seasonYear"`
	StartDate  *struct {
		Year *int `json:"year"`
	} `json:"startDate"`
	CoverImage *struct {
		Large *string `json:"large"`
	} `json:"coverImage"`
	Description  *string  `json:"description"`
	Genres       []string `json:"genres"`
	AverageScore *int     `json:"averageScore"`
}

func nonEmpty(p *string) bool { return p != nil && *p != "" }

// MapAniListMedia converts AniList media to a Title.
func MapAniListMedia(m AniListMedia) domain.Title {
	var name string
	switch {
	case nonEmpty(m.Title.English):
		name = *m.Title.English
	case nonEmpty(m.Title.Romaji):
		name = *m.Title.Romaji
	case nonEmpty(m.Title.Native):
		name = *m.Title.Native
	default:
		name = "#" + strconv.Itoa(m.ID)
	}
	original := m.Title.Native
	if nonEmpty(m.Title.Romaji) && *m.Title.Romaji != name {
		original = m.Title.Romaji
	}
	if !nonEmpty(original) || *original == name {
		original = nil
	}
	year := m.SeasonYear
	if year == nil && m.StartDate != nil {
		year = m.StartDate.Year
	}
	var poster *string
	if m.CoverImage != nil {
		poster = m.CoverImage.Large
	}
	genres := m.Genres
	if genres == nil {
		genres = []string{}
	}
	id := strconv.Itoa(m.ID)
	return domain.Title{
		ID: domain.MakeTitleID(domain.SourceAniList, id), Source: domain.SourceAniList, SourceID: id,
		Kind: domain.KindAnime, Name: name, OriginalName: original, Year: year, PosterURL: poster,
		Overview: domain.CleanText(m.Description), Genres: genres, Format: m.Format,
		EpisodeCount: m.Episodes, AniListScore: m.AverageScore,
	}
}

// AniList is a keyless GraphQL client.
type AniList struct {
	HTTP     *http.Client
	Endpoint string
}

func NewAniList(c *http.Client) *AniList { return &AniList{HTTP: c, Endpoint: AniListEndpoint} }

func (a *AniList) gql(ctx context.Context, query string, vars map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := a.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return StatusError{Source: "AniList", Code: res.StatusCode}
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		return err
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return fmt.Errorf("AniList: empty response")
	}
	return json.Unmarshal(env.Data, out)
}

func (a *AniList) Search(ctx context.Context, query string) ([]domain.Title, error) {
	var data struct {
		Page struct {
			Media []AniListMedia `json:"media"`
		} `json:"Page"`
	}
	if err := a.gql(ctx, searchQuery, map[string]any{"search": query, "perPage": 15}, &data); err != nil {
		return nil, err
	}
	out := make([]domain.Title, 0, len(data.Page.Media))
	for _, m := range data.Page.Media {
		out = append(out, MapAniListMedia(m))
	}
	return out, nil
}

func (a *AniList) GetByID(ctx context.Context, id string) (*domain.Title, error) {
	n, err := strconv.Atoi(id)
	if err != nil {
		return nil, nil
	}
	var data struct {
		Media *AniListMedia `json:"Media"`
	}
	if err := a.gql(ctx, detailQuery, map[string]any{"id": n}, &data); err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if data.Media == nil {
		return nil, nil
	}
	t := MapAniListMedia(*data.Media)
	return &t, nil
}
