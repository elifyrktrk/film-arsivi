package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// KitsuAPI is Kitsu's public JSON:API (no key). Kitsu matches partial words
// ("frie" → Frieren) where AniList's search does not, so it is used only to find
// AniList ids; titles still come from AniList.
const KitsuAPI = "https://kitsu.io/api/edge"

type Kitsu struct {
	HTTP    *http.Client
	BaseURL string
}

func NewKitsu(c *http.Client) *Kitsu { return &Kitsu{HTTP: c, BaseURL: KitsuAPI} }

// AniListIDs returns the AniList ids of Kitsu's text matches, in Kitsu's order.
// Matches without an AniList mapping are skipped.
func (k *Kitsu) AniListIDs(ctx context.Context, query string, limit int) ([]int, error) {
	q := url.Values{
		"filter[text]":     {query},
		"page[limit]":      {strconv.Itoa(limit)},
		"include":          {"mappings"},
		"fields[anime]":    {"canonicalTitle,mappings"},
		"fields[mappings]": {"externalSite,externalId"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.BaseURL+"/anime?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.api+json")
	res, err := k.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, StatusError{Source: "Kitsu", Code: res.StatusCode}
	}
	type ref struct {
		ID string `json:"id"`
	}
	var doc struct {
		Data []struct {
			Relationships struct {
				Mappings struct {
					Data []ref `json:"data"`
				} `json:"mappings"`
			} `json:"relationships"`
		} `json:"data"`
		Included []struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				ExternalSite string `json:"externalSite"`
				ExternalID   string `json:"externalId"`
			} `json:"attributes"`
		} `json:"included"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return nil, err
	}
	anilistByMapping := map[string]int{}
	for _, inc := range doc.Included {
		if inc.Type == "mappings" && inc.Attributes.ExternalSite == "anilist/anime" {
			if id, err := strconv.Atoi(inc.Attributes.ExternalID); err == nil && id > 0 {
				anilistByMapping[inc.ID] = id
			}
		}
	}
	var out []int
	seen := map[int]bool{}
	for _, a := range doc.Data {
		for _, m := range a.Relationships.Mappings.Data {
			if id, ok := anilistByMapping[m.ID]; ok && !seen[id] {
				seen[id] = true
				out = append(out, id)
				break
			}
		}
	}
	return out, nil
}
