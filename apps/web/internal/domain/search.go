package domain

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// SourceState is how a source answered a search.
type SourceState string

const (
	SourceOK       SourceState = "ok"
	SourceDisabled SourceState = "disabled"
	SourceError    SourceState = "error"
	// SourceLimited: the source answered 429 (too many requests).
	SourceLimited SourceState = "limited"
)

// stateOf maps a source error to its state. Errors with RateLimited() true
// (sources.StatusError on 429) become SourceLimited.
func stateOf(err error) SourceState {
	var rl interface{ RateLimited() bool }
	if errors.As(err, &rl) && rl.RateLimited() {
		return SourceLimited
	}
	return SourceError
}

type SourceStates struct {
	TMDB    SourceState `json:"tmdb"`
	AniList SourceState `json:"anilist"`
}

type SearchResult struct {
	Results []Title      `json:"results"`
	Sources SourceStates `json:"sources"`
}

// ErrSearchFailed means no source answered.
var ErrSearchFailed = errors.New("search_failed")

// SourceClient is what the TMDB and AniList clients provide.
type SourceClient interface {
	Search(ctx context.Context, query string) ([]Title, error)
	// GetByID takes the source id part (`movie:603` for TMDB, `21` for AniList).
	GetByID(ctx context.Context, sourceID string) (*Title, error)
}

func titleNames(t Title) []string {
	names := []string{NormalizeTitle(t.Name)}
	if t.OriginalName != nil && *t.OriginalName != "" {
		names = append(names, NormalizeTitle(*t.OriginalName))
	}
	return names
}

func matchScore(query string, t Title) int {
	q := NormalizeTitle(query)
	names := titleNames(t)
	switch {
	case slices.Contains(names, q):
		return 3
	case slices.ContainsFunc(names, func(n string) bool { return strings.HasPrefix(n, q) }):
		return 2
	case slices.ContainsFunc(names, func(n string) bool { return strings.Contains(n, q) }):
		return 1
	}
	return 0
}

func sameWork(a, b Title) bool {
	if a.Year != nil && b.Year != nil && *a.Year != 0 && *b.Year != 0 {
		d := *a.Year - *b.Year
		if d > 1 || d < -1 {
			return false
		}
	}
	bn := titleNames(b)
	return slices.ContainsFunc(titleNames(a), func(x string) bool { return slices.Contains(bn, x) })
}

// MergeSearchResults merges TMDB and AniList results into one list.
//   - Anime that appears in both sources is kept once, from AniList.
//   - Ordered by how well the name matches the query, then by each source's own rank,
//     alternating sources on ties so neither dominates the top of the list.
func MergeSearchResults(query string, tmdb, anilist []Title) []Title {
	type ranked struct {
		t             Title
		i, src, score int
	}
	var rs []ranked
	for _, t := range tmdb {
		if t.Kind == KindAnime && slices.ContainsFunc(anilist, func(a Title) bool { return sameWork(t, a) }) {
			continue
		}
		rs = append(rs, ranked{t: t, i: len(rs), src: 0})
	}
	for i, t := range anilist {
		rs = append(rs, ranked{t: t, i: i, src: 1})
	}
	for k := range rs {
		rs[k].score = matchScore(query, rs[k].t)
	}
	slices.SortStableFunc(rs, func(a, b ranked) int {
		if a.score != b.score {
			return b.score - a.score
		}
		if a.i != b.i {
			return a.i - b.i
		}
		return a.src - b.src
	})
	seen := map[string]bool{}
	out := make([]Title, 0, len(rs))
	for _, r := range rs {
		if !seen[r.t.ID] {
			seen[r.t.ID] = true
			out = append(out, r.t)
		}
	}
	return out
}

// SearchService searches both sources and loads single titles. TMDB is nil when no
// API key is configured.
type SearchService struct {
	AniList SourceClient
	TMDB    SourceClient
}

func (s *SearchService) TMDBEnabled() bool { return s.TMDB != nil }

func (s *SearchService) Search(ctx context.Context, query string) (SearchResult, error) {
	q := strings.TrimSpace(query)
	tmdbIdle := SourceOK
	if s.TMDB == nil {
		tmdbIdle = SourceDisabled
	}
	if jsLen(q) < 2 {
		return SearchResult{Results: []Title{}, Sources: SourceStates{TMDB: tmdbIdle, AniList: SourceOK}}, nil
	}
	var (
		wg         sync.WaitGroup
		tRes, aRes []Title
		tErr, aErr error
	)
	if s.TMDB != nil {
		wg.Add(1)
		go func() { defer wg.Done(); tRes, tErr = s.TMDB.Search(ctx, q) }()
	}
	wg.Add(1)
	go func() { defer wg.Done(); aRes, aErr = s.AniList.Search(ctx, q) }()
	wg.Wait()

	states := SourceStates{TMDB: SourceDisabled, AniList: SourceOK}
	if s.TMDB != nil {
		states.TMDB = SourceOK
		if tErr != nil {
			states.TMDB = stateOf(tErr)
			tRes = nil
		}
	}
	if aErr != nil {
		states.AniList = stateOf(aErr)
		aRes = nil
	}
	if states.AniList != SourceOK && states.TMDB != SourceOK {
		return SearchResult{Sources: states}, ErrSearchFailed
	}
	return SearchResult{Results: MergeSearchResults(q, tRes, aRes), Sources: states}, nil
}

// GetTitle loads a title from its source. Returns nil for unknown ids.
func (s *SearchService) GetTitle(ctx context.Context, id string) (*Title, error) {
	source, sourceID := SplitTitleID(id)
	switch {
	case source == SourceAniList:
		return s.AniList.GetByID(ctx, sourceID)
	case source == SourceTMDB && s.TMDB != nil:
		return s.TMDB.GetByID(ctx, sourceID)
	}
	return nil, nil
}

// ---------- Archive sorting ----------

// SortArchive sorts entries in place by the given order (default: most recent first).
func SortArchive(entries []ArchiveEntry, sort ArchiveSort) {
	switch sort {
	case SortScore:
		val := func(e ArchiveEntry) int {
			if e.Rating.Score == nil {
				return 0
			}
			return *e.Rating.Score
		}
		slices.SortStableFunc(entries, func(a, b ArchiveEntry) int {
			if d := val(b) - val(a); d != 0 {
				return d
			}
			return strings.Compare(b.Rating.UpdatedAt, a.Rating.UpdatedAt)
		})
	case SortName:
		c := collate.New(language.Turkish) // a Collator is not safe for concurrent use
		slices.SortStableFunc(entries, func(a, b ArchiveEntry) int { return c.CompareString(a.Title.Name, b.Title.Name) })
	default:
		slices.SortStableFunc(entries, func(a, b ArchiveEntry) int {
			return strings.Compare(b.Rating.UpdatedAt, a.Rating.UpdatedAt)
		})
	}
}
