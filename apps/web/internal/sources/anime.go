package sources

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"filmarsivi/web/internal/domain"
)

// Anime searches AniList and, when AniList finds few results (typically because the
// last word is only partly typed: "frie", "jujut"), asks Kitsu which AniList ids
// match and loads them from AniList. At most two extra requests per search: one to
// Kitsu (sent in parallel with AniList) and one AniList id_in query.
type Anime struct {
	AniList *AniList
	Kitsu   *Kitsu // nil disables the fallback
	// MinResults below which the Kitsu fallback is used (default 5).
	MinResults int
	// MaxResults caps the merged list (default 15, AniList's page size).
	MaxResults int
}

func (a *Anime) Search(ctx context.Context, query string) ([]domain.Title, error) {
	minResults, maxResults := a.MinResults, a.MaxResults
	if minResults == 0 {
		minResults = 5
	}
	if maxResults == 0 {
		maxResults = 15
	}
	var (
		wg      sync.WaitGroup
		kitsuID []int
		kErr    error
	)
	if a.Kitsu != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kitsuID, kErr = a.Kitsu.AniListIDs(ctx, query, 10)
		}()
	}
	main, err := a.AniList.Search(ctx, query)
	wg.Wait()
	if err != nil {
		return nil, err // AniList itself is down or rate limited: report it
	}
	if a.Kitsu == nil || kErr != nil || len(main) >= minResults || len(kitsuID) == 0 {
		return main, nil // the fallback is best effort
	}
	have := map[string]bool{}
	for _, t := range main {
		have[t.ID] = true
	}
	var missing []int
	for _, id := range kitsuID {
		if !have[domain.MakeTitleID(domain.SourceAniList, strconv.Itoa(id))] {
			missing = append(missing, id)
		}
	}
	if len(missing) > maxResults-len(main) {
		missing = missing[:maxResults-len(main)]
	}
	extra, err := a.AniList.GetByIDs(ctx, missing)
	if err != nil {
		return main, nil
	}
	return append(main, extra...), nil
}

func (a *Anime) GetByID(ctx context.Context, id string) (*domain.Title, error) {
	return a.AniList.GetByID(ctx, id)
}

// Cached keeps search answers for a while so typing, going back and retrying do not
// hit the sources (AniList allows ~90 requests a minute). Errors are not cached.
type Cached struct {
	Source domain.SourceClient
	TTL    time.Duration
	Now    func() time.Time

	mu    sync.Mutex
	items map[string]cachedSearch
}

type cachedSearch struct {
	titles []domain.Title
	exp    time.Time
}

func NewCached(src domain.SourceClient, ttl time.Duration) *Cached {
	return &Cached{Source: src, TTL: ttl, Now: time.Now, items: map[string]cachedSearch{}}
}

func (c *Cached) Search(ctx context.Context, query string) ([]domain.Title, error) {
	key := strings.Join(strings.Fields(strings.ToLower(query)), " ")
	c.mu.Lock()
	if it, ok := c.items[key]; ok && c.Now().Before(it.exp) {
		c.mu.Unlock()
		return it.titles, nil
	}
	c.mu.Unlock()
	titles, err := c.Source.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.items) >= 500 {
		now := c.Now()
		for k, it := range c.items {
			if !now.Before(it.exp) {
				delete(c.items, k)
			}
		}
		if len(c.items) >= 500 {
			clear(c.items)
		}
	}
	c.items[key] = cachedSearch{titles: titles, exp: c.Now().Add(c.TTL)}
	c.mu.Unlock()
	return titles, nil
}

func (c *Cached) GetByID(ctx context.Context, id string) (*domain.Title, error) {
	return c.Source.GetByID(ctx, id)
}
