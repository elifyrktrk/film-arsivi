// Mirrors packages/core/src/core.test.ts scenario by scenario so the TS and Go cores
// stay equal. Store-backed scenarios run against both Memory and SQLite.
package domain_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"filmarsivi/web/internal/domain"
	"filmarsivi/web/internal/store"
)

var ctx = context.Background()

const user = "u1"

func sp(s string) *string { return &s }
func ip(i int) *int       { return &i }

type titleOpt func(*domain.Title)

func kind(k domain.MediaKind) titleOpt { return func(t *domain.Title) { t.Kind = k } }
func year(y int) titleOpt              { return func(t *domain.Title) { t.Year = ip(y) } }

func title(id, name string, opts ...titleOpt) domain.Title {
	source, rest := domain.SplitTitleID(id)
	t := domain.Title{ID: id, Source: source, SourceID: rest, Kind: domain.KindMovie, Name: name, Year: ip(2000), Genres: []string{}}
	for _, o := range opts {
		o(&t)
	}
	return t
}

func clock() domain.Clock {
	t := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	return func() time.Time { t = t.Add(time.Second); return t }
}

func ids() func() string {
	n := 0
	return func() string { n++; return "c" + string(rune('0'+n)) }
}

func code(t *testing.T, err error, want domain.ErrorCode) {
	t.Helper()
	got, ok := domain.CodeOf(err)
	if !ok || got != want {
		t.Fatalf("want error %s, got %v", want, err)
	}
}

// must fails the test (via panic) on error; Go cannot pass (v, err) after t.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func eq[T comparable](t *testing.T, got, want []T) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// eachStore runs a scenario against every Store implementation.
func eachStore(t *testing.T, f func(t *testing.T, s domain.Store)) {
	t.Run("memory", func(t *testing.T) { f(t, store.NewMemory()) })
	t.Run("sqlite", func(t *testing.T) {
		db, err := store.OpenSQLite(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		f(t, db)
	})
}

// ---------- validateScore ----------

func TestValidateScore(t *testing.T) {
	t.Run("accepts integers 1–10 and null", func(t *testing.T) {
		for _, v := range []int{1, 10} {
			if got := must(domain.ValidateScore(ip(v))); *got != v {
				t.Fatal(got)
			}
		}
		if got := must(domain.ValidateScore(nil)); got != nil {
			t.Fatal(got)
		}
	})
	// TS also rejects 7.5 and NaN; Go ints cannot hold them, so the form parser covers that.
	for _, v := range []int{0, 11, -3} {
		_, err := domain.ValidateScore(ip(v))
		code(t, err, domain.ErrScoreOutOfRange)
	}
	for _, s := range []string{"0", "11", "7.5", "-3", "NaN", "abc"} {
		_, err := domain.ParseScore(s)
		code(t, err, domain.ErrScoreOutOfRange)
	}
	if got := must(domain.ParseScore("")); got != nil {
		t.Fatal("empty form value means no score")
	}
}

// ---------- rating service ----------

func newRatings(s domain.Store) *domain.RatingService {
	return &domain.RatingService{Store: s, Now: clock()}
}

func TestRatingService(t *testing.T) {
	movie := title("tmdb:movie:603", "Matrix")

	t.Run("keeps fields it was not asked to change", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			svc := newRatings(s)
			must(svc.Update(ctx, user, movie, domain.RatingPatch{SetScore: true, Score: ip(9)}))
			watched := domain.StatusWatched
			r := must(svc.Update(ctx, user, movie, domain.RatingPatch{SetStatus: true, Status: &watched}))
			if *r.Score != 9 || *r.Status != domain.StatusWatched || r.Note != "" {
				t.Fatalf("%+v", r)
			}
			got := must(s.GetTitle(ctx, movie.ID))
			if got == nil || got.ID != movie.ID || got.Name != movie.Name || *got.Year != 2000 {
				t.Fatalf("title not cached: %+v", got)
			}
		})
	})

	t.Run("trims notes and removes a record once everything is cleared", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			svc := newRatings(s)
			r := must(svc.Update(ctx, user, movie, domain.RatingPatch{SetNote: true, Note: "  güzel  "}))
			if r.Note != "güzel" {
				t.Fatalf("%q", r.Note)
			}
			if r := must(svc.Update(ctx, user, movie, domain.RatingPatch{SetNote: true, Note: ""})); r != nil {
				t.Fatalf("want nil, got %+v", r)
			}
			if r := must(s.GetRating(ctx, user, movie.ID)); r != nil {
				t.Fatal("record should be gone")
			}
		})
	})

	t.Run("lists the archive filtered and sorted", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			svc := newRatings(s)
			a := title("tmdb:movie:1", "Zeta")
			b := title("tmdb:movie:2", "Alfa")
			watched, planned := domain.StatusWatched, domain.StatusPlanned
			must(svc.Update(ctx, user, a, domain.RatingPatch{SetScore: true, Score: ip(6), SetStatus: true, Status: &watched}))
			must(svc.Update(ctx, user, b, domain.RatingPatch{SetScore: true, Score: ip(9), SetStatus: true, Status: &planned}))
			names := func(q domain.ArchiveQuery) []string {
				var out []string
				for _, e := range must(s.ListArchive(ctx, user, q)) {
					out = append(out, e.Title.Name)
				}
				return out
			}
			eq(t, names(domain.ArchiveQuery{Sort: domain.SortScore}), []string{"Alfa", "Zeta"})
			eq(t, names(domain.ArchiveQuery{Sort: domain.SortRecent}), []string{"Alfa", "Zeta"})
			eq(t, names(domain.ArchiveQuery{Status: domain.StatusWatched}), []string{"Zeta"})
			eq(t, names(domain.ArchiveQuery{Sort: domain.SortName}), []string{"Alfa", "Zeta"})
		})
	})

	t.Run("keeps users apart (Go only: the Store is user-scoped)", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			svc := newRatings(s)
			must(svc.Update(ctx, user, movie, domain.RatingPatch{SetScore: true, Score: ip(9)}))
			if r := must(s.GetRating(ctx, "someone-else", movie.ID)); r != nil {
				t.Fatal("rating leaked to another user")
			}
		})
	})
}

// ---------- category service ----------

func newCategories(s domain.Store) *domain.CategoryService {
	return &domain.CategoryService{Store: s, Now: clock(), NewID: ids()}
}

func TestCategoryService(t *testing.T) {
	in := func(name string, color domain.CategoryColor) domain.CategoryInput {
		return domain.CategoryInput{Name: name, Color: color}
	}

	t.Run("validates names and rejects duplicates regardless of case and spacing", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			svc := newCategories(s)
			_, err := svc.Create(ctx, user, in("   ", "blue"))
			code(t, err, domain.ErrCategoryNameRequired)
			c := must(svc.Create(ctx, user, in("İyi  Hissettirenler", "blue")))
			if c.Name != "İyi Hissettirenler" {
				t.Fatalf("%q", c.Name)
			}
			_, err = svc.Create(ctx, user, in("iyi hissettirenler", "red"))
			code(t, err, domain.ErrCategoryNameTaken)
		})
	})

	t.Run("limits name and description length (Go extra)", func(t *testing.T) {
		svc := newCategories(store.NewMemory())
		_, err := svc.Create(ctx, user, in(strings.Repeat("a", 41), "blue"))
		code(t, err, domain.ErrCategoryNameTooLong)
		_, err = svc.Create(ctx, user, domain.CategoryInput{Name: "x", Color: "blue", Description: strings.Repeat("ş", 201)})
		code(t, err, domain.ErrCategoryDescriptionTooLong)
		_, err = svc.Create(ctx, user, in("x", "gold"))
		code(t, err, domain.ErrInvalidColor)
		must(svc.Create(ctx, user, in(strings.Repeat("ğ", 40), "blue")))
	})

	t.Run("allows renaming a category to its own name", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			svc := newCategories(s)
			c := must(svc.Create(ctx, user, in("Twist", "blue")))
			u := must(svc.Update(ctx, user, c.ID, in("twist", "red")))
			if u.Name != "twist" || u.Color != "red" {
				t.Fatalf("%+v", u)
			}
		})
	})

	t.Run("appends items in order and ignores duplicates", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			svc := newCategories(s)
			c := must(svc.Create(ctx, user, in("Liste", "teal")))
			a := title("anilist:1", "A")
			b := title("anilist:2", "B")
			for _, x := range []domain.Title{a, b, a} {
				if err := svc.AddTitle(ctx, user, c.ID, x); err != nil {
					t.Fatal(err)
				}
			}
			var got []string
			for _, i := range must(svc.ListItems(ctx, user, c.ID)) {
				got = append(got, i.Title.Name+":"+string(rune('0'+i.Position)))
			}
			eq(t, got, []string{"A:0", "B:1"})
			if list := must(svc.List(ctx, user)); list[0].ItemCount != 2 {
				t.Fatalf("%+v", list)
			}
		})
	})

	t.Run("sets a title's categories and deleting a category keeps the title", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			svc := newCategories(s)
			c1 := must(svc.Create(ctx, user, in("Bir", "teal")))
			c2 := must(svc.Create(ctx, user, in("İki", "pink")))
			x := title("tmdb:tv:1399", "Dizi")
			if err := svc.SetCategoriesForTitle(ctx, user, x, []string{c1.ID, c2.ID}); err != nil {
				t.Fatal(err)
			}
			if err := svc.SetCategoriesForTitle(ctx, user, x, []string{c2.ID}); err != nil {
				t.Fatal(err)
			}
			eq(t, must(svc.CategoryIDsForTitle(ctx, user, x.ID)), []string{c2.ID})
			if err := svc.Remove(ctx, user, c2.ID); err != nil {
				t.Fatal(err)
			}
			eq(t, must(svc.CategoryIDsForTitle(ctx, user, x.ID)), []string{})
			if got := must(s.GetTitle(ctx, x.ID)); got == nil {
				t.Fatal("title should stay")
			}
		})
	})

	t.Run("refuses to add to a missing category", func(t *testing.T) {
		eachStore(t, func(t *testing.T, s domain.Store) {
			err := newCategories(s).AddTitle(ctx, user, "nope", title("anilist:1", "A"))
			code(t, err, domain.ErrCategoryNotFound)
		})
	})
}

// ---------- mergeSearchResults ----------

func idsOf(ts []domain.Title) []string {
	out := []string{}
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

func TestMergeSearchResults(t *testing.T) {
	t.Run("drops TMDB anime that AniList also returned", func(t *testing.T) {
		tmdb := []domain.Title{
			title("tmdb:tv:37854", "One Piece", kind(domain.KindAnime), year(1999)),
			title("tmdb:tv:111110", "One Piece", kind(domain.KindTV), year(2023)),
		}
		anilist := []domain.Title{title("anilist:21", "ONE PIECE", kind(domain.KindAnime), year(1999))}
		got := idsOf(domain.MergeSearchResults("one piece", tmdb, anilist))
		if !slices.Contains(got, "anilist:21") || !slices.Contains(got, "tmdb:tv:111110") || slices.Contains(got, "tmdb:tv:37854") {
			t.Fatal(got)
		}
	})

	t.Run("ranks exact matches first and alternates sources on ties", func(t *testing.T) {
		tmdb := []domain.Title{title("tmdb:movie:1", "Dune: Part Two"), title("tmdb:movie:2", "Dune")}
		anilist := []domain.Title{title("anilist:5", "Dune Anime", kind(domain.KindAnime))}
		eq(t, idsOf(domain.MergeSearchResults("dune", tmdb, anilist)), []string{"tmdb:movie:2", "tmdb:movie:1", "anilist:5"})
	})

	t.Run("matches Turkish characters loosely", func(t *testing.T) {
		got := domain.MergeSearchResults("sahsiyet", []domain.Title{title("tmdb:tv:9", "Şahsiyet")}, nil)
		if len(got) == 0 || got[0].ID != "tmdb:tv:9" {
			t.Fatal(idsOf(got))
		}
	})
}

// ---------- search service ----------

type fakeSource struct {
	results []domain.Title
	err     error
}

func (f fakeSource) Search(context.Context, string) ([]domain.Title, error) { return f.results, f.err }
func (f fakeSource) GetByID(context.Context, string) (*domain.Title, error) { return nil, nil }

func TestSearchService(t *testing.T) {
	anilistOK := fakeSource{results: []domain.Title{title("anilist:1", "Frieren", kind(domain.KindAnime))}}
	down := fakeSource{err: errors.New("down")}

	t.Run("reports TMDB as disabled without a key", func(t *testing.T) {
		r := must((&domain.SearchService{AniList: anilistOK}).Search(ctx, "frieren"))
		if r.Sources != (domain.SourceStates{TMDB: domain.SourceDisabled, AniList: domain.SourceOK}) || len(r.Results) != 1 {
			t.Fatalf("%+v", r)
		}
	})

	t.Run("keeps results from one source when the other fails", func(t *testing.T) {
		r := must((&domain.SearchService{AniList: anilistOK, TMDB: down}).Search(ctx, "frieren"))
		if r.Sources.TMDB != domain.SourceError || len(r.Results) != 1 {
			t.Fatalf("%+v", r)
		}
	})

	t.Run("fails when no source answers", func(t *testing.T) {
		_, err := (&domain.SearchService{AniList: down}).Search(ctx, "x y")
		if !errors.Is(err, domain.ErrSearchFailed) {
			t.Fatal(err)
		}
	})
}

// ---------- text ----------

func TestCleanText(t *testing.T) {
	if domain.CleanText(sp("  <br> ")) != nil {
		t.Fatal("cleanText returns null for empty input")
	}
	got := domain.CleanText(sp("Elf mage<br><br>Frieren &amp; friends <i>travel</i>."))
	if got == nil || *got != "Elf mage\n\nFrieren & friends travel." {
		t.Fatalf("%q", *got)
	}
}
