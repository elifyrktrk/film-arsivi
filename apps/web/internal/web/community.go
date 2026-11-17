package web

import (
	"context"
	"strconv"
	"strings"
	"time"

	"filmarsivi/web/internal/domain"
)

// IMDbLookup finds IMDb ratings by tconst. Optional: nil hides IMDb scores.
type IMDbLookup interface {
	IMDbRating(ctx context.Context, tconst string) (*domain.IMDbRating, error)
}

// decimalTR formats with one decimal and a Turkish decimal comma: 8.66 → "8,7".
func decimalTR(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', 1, 64), ".", ",", 1)
}

// AniListScore converts AniList's 0–100 averageScore to a 10-point label ("8,9").
// Returns "" when there is no score.
func AniListScore(s *int) string {
	if s == nil || *s <= 0 {
		return ""
	}
	return decimalTR(float64(*s) / 10)
}

// VotesTR shortens vote counts Turkish style: 850, 4,5 B, 45 B, 2,1 Mn.
func VotesTR(n int) string {
	short := func(v float64, unit string) string {
		if v < 10 {
			return strings.TrimSuffix(decimalTR(v), ",0") + " " + unit
		}
		return strconv.Itoa(int(v)) + " " + unit
	}
	switch {
	case n >= 1_000_000:
		return short(float64(n)/1_000_000, "Mn")
	case n >= 1_000:
		return short(float64(n)/1_000, "B")
	}
	return strconv.Itoa(n)
}

// communityScore is a source's own average, shown smaller than the user's score.
type communityScore struct {
	Source string // "IMDb" | "AniList"
	Score  string // "8,7"
	Votes  string // "2,1 Mn" (IMDb only)
}

// communityFor returns the community score for a title, or nil when there is none:
// AniList titles use AniList's average; TMDB titles use IMDb via the imdbId from TMDB.
func (s *Server) communityFor(t domain.Title) *communityScore {
	if v := AniListScore(t.AniListScore); v != "" {
		return &communityScore{Source: "AniList", Score: v}
	}
	if t.IMDbID == nil || s.IMDb == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := s.IMDb.IMDbRating(ctx, *t.IMDbID)
	if err != nil || r == nil {
		return nil
	}
	return &communityScore{Source: "IMDb", Score: decimalTR(r.Rating), Votes: VotesTR(r.Votes)}
}
