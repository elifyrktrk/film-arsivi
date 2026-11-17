// Package domain is the Go port of packages/core: models, validation rules, the
// rating and category services and the search merge rules. Behavior is kept equal
// to the TypeScript core; domain_test.go mirrors packages/core/src/core.test.ts.
package domain

import (
	"context"
	"strings"
)

// Source is where a title's data comes from.
type Source string

const (
	SourceTMDB    Source = "tmdb"
	SourceAniList Source = "anilist"
	SourceManual  Source = "manual"
)

// MediaKind is what the user sees as the kind of content. Anime is its own kind.
type MediaKind string

const (
	KindMovie MediaKind = "movie"
	KindTV    MediaKind = "tv"
	KindAnime MediaKind = "anime"
)

// WatchStatus is the personal watch status.
type WatchStatus string

const (
	StatusWatched WatchStatus = "watched"
	StatusPlanned WatchStatus = "planned"
	StatusDropped WatchStatus = "dropped"
)

// WatchStatuses lists statuses in display order.
var WatchStatuses = []WatchStatus{StatusWatched, StatusPlanned, StatusDropped}

// ParseWatchStatus returns the status for s, or false when s is not a known status.
func ParseWatchStatus(s string) (WatchStatus, bool) {
	for _, w := range WatchStatuses {
		if string(w) == s {
			return w, true
		}
	}
	return "", false
}

const (
	ScoreMin               = 1
	ScoreMax               = 10
	NoteMaxLength          = 1000
	CategoryNameMax        = 40
	CategoryDescriptionMax = 200
)

// Title is a film, series or anime as stored locally. ID is `${source}:${sourceId}`,
// e.g. `tmdb:movie:603`, `anilist:21`.
type Title struct {
	ID           string    `json:"id"`
	Source       Source    `json:"source"`
	SourceID     string    `json:"sourceId"`
	Kind         MediaKind `json:"kind"`
	Name         string    `json:"name"`
	OriginalName *string   `json:"originalName"`
	Year         *int      `json:"year"`
	PosterURL    *string   `json:"posterUrl"`
	Overview     *string   `json:"overview"`
	Genres       []string  `json:"genres"`
	// Format is the AniList format such as TV, MOVIE, OVA. Nil for TMDB/manual.
	Format       *string `json:"format"`
	SeasonCount  *int    `json:"seasonCount"`
	EpisodeCount *int    `json:"episodeCount"`

	// Go-only fields (not in packages/core). Community scores are shown next to,
	// never instead of, the user's own score.
	// IMDbID is the IMDb tconst (tt0133093), known from TMDB details.
	IMDbID *string `json:"imdbId,omitempty"`
	// AniListScore is AniList's averageScore, 0–100.
	AniListScore *int `json:"anilistScore,omitempty"`
}

// IMDbRating is one row of IMDb's title.ratings dataset.
type IMDbRating struct {
	TConst string
	Rating float64 // 1.0–10.0
	Votes  int
}

// MakeTitleID builds a stable title id.
func MakeTitleID(source Source, sourceID string) string {
	return string(source) + ":" + sourceID
}

// SplitTitleID returns the source and source id parts of a title id.
func SplitTitleID(id string) (Source, string) {
	source, rest, _ := strings.Cut(id, ":")
	return Source(source), rest
}

// Rating is the user's personal record for one title.
type Rating struct {
	TitleID   string       `json:"titleId"`
	Score     *int         `json:"score"`
	Note      string       `json:"note"`
	Status    *WatchStatus `json:"status"`
	UpdatedAt string       `json:"updatedAt"`
}

// CategoryColor is stored as a key so each theme can render its own shade.
type CategoryColor string

// CategoryColors lists the allowed color keys in display order.
var CategoryColors = []CategoryColor{"violet", "blue", "teal", "green", "amber", "orange", "red", "pink", "slate"}

// ParseCategoryColor returns the color for s, or false when it is not a known key.
func ParseCategoryColor(s string) (CategoryColor, bool) {
	for _, c := range CategoryColors {
		if string(c) == s {
			return c, true
		}
	}
	return "", false
}

type Category struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Color       CategoryColor `json:"color"`
	Description string        `json:"description"`
	CreatedAt   string        `json:"createdAt"`
	UpdatedAt   string        `json:"updatedAt"`
}

type CategoryItem struct {
	CategoryID string `json:"categoryId"`
	TitleID    string `json:"titleId"`
	Position   int    `json:"position"`
	AddedAt    string `json:"addedAt"`
}

// CategoryItemWithTitle is an item joined with its cached title.
type CategoryItemWithTitle struct {
	CategoryItem
	Title Title `json:"title"`
}

// CategoryWithCount adds list-screen data to a category.
type CategoryWithCount struct {
	Category
	ItemCount int `json:"itemCount"`
	// PreviewPosters holds up to 3 poster URLs, newest first.
	PreviewPosters []string `json:"previewPosters"`
}

// ArchiveEntry is a rated title joined with its cached data.
type ArchiveEntry struct {
	Title  Title  `json:"title"`
	Rating Rating `json:"rating"`
}

type ArchiveSort string

const (
	SortRecent ArchiveSort = "recent"
	SortScore  ArchiveSort = "score"
	SortName   ArchiveSort = "name"
)

// ArchiveQuery filters the archive. An empty Status means all statuses.
type ArchiveQuery struct {
	Status WatchStatus
	Sort   ArchiveSort
}

// Store is the storage contract (Go version of packages/core/src/repository.ts).
// Every per-user method takes a userID so the same contract serves a multi-user
// server later; titles are a shared cache and carry no user. Business rules live in
// the services, so implementations only store and fetch.
type Store interface {
	GetTitle(ctx context.Context, id string) (*Title, error)
	// UpsertTitle inserts or refreshes the cached copy of a title.
	UpsertTitle(ctx context.Context, t Title) error

	GetRating(ctx context.Context, userID, titleID string) (*Rating, error)
	UpsertRating(ctx context.Context, userID string, r Rating) error
	DeleteRating(ctx context.Context, userID, titleID string) error
	// ListArchive returns all rated titles joined with their cached title data.
	ListArchive(ctx context.Context, userID string, q ArchiveQuery) ([]ArchiveEntry, error)

	// ListCategories returns categories oldest first.
	ListCategories(ctx context.Context, userID string) ([]CategoryWithCount, error)
	GetCategory(ctx context.Context, userID, id string) (*Category, error)
	InsertCategory(ctx context.Context, userID string, c Category) error
	UpdateCategory(ctx context.Context, userID string, c Category) error
	// DeleteCategory deletes the category and its items, never the titles themselves.
	DeleteCategory(ctx context.Context, userID, id string) error

	ListCategoryItems(ctx context.Context, userID, categoryID string) ([]CategoryItemWithTitle, error)
	GetCategoryItem(ctx context.Context, userID, categoryID, titleID string) (*CategoryItem, error)
	InsertCategoryItem(ctx context.Context, userID string, item CategoryItem) error
	DeleteCategoryItem(ctx context.Context, userID, categoryID, titleID string) error
	// MaxPosition returns the highest position in the category, or -1 when empty.
	MaxPosition(ctx context.Context, userID, categoryID string) (int, error)
	// CategoryIDsForTitle returns ids of the user's categories that contain the title.
	CategoryIDsForTitle(ctx context.Context, userID, titleID string) ([]string, error)
}
