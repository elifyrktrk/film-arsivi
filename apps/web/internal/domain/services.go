package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Clock returns the current time; tests inject a fake one.
type Clock func() time.Time

// isoTime formats like JavaScript's Date.toISOString so stored values sort the same.
func isoTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func defaultID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------- Ratings ----------

// ValidateScore accepts nil or an integer from 1 to 10.
func ValidateScore(score *int) (*int, error) {
	if score == nil {
		return nil, nil
	}
	if *score < ScoreMin || *score > ScoreMax {
		return nil, newErr(ErrScoreOutOfRange)
	}
	return score, nil
}

// ParseScore reads a form value: "" means no score; anything other than an integer
// 1–10 (e.g. "7.5", "0") is out of range.
func ParseScore(s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, newErr(ErrScoreOutOfRange)
	}
	return ValidateScore(&n)
}

// ValidateNote trims the note and checks its length.
func ValidateNote(note string) (string, error) {
	t := strings.TrimSpace(note)
	if jsLen(t) > NoteMaxLength {
		return "", newErr(ErrNoteTooLong)
	}
	return t, nil
}

// RatingPatch is a partial change. A field is applied only when its Set flag is true;
// a nil value with Set clears that field.
type RatingPatch struct {
	SetScore  bool
	Score     *int
	SetNote   bool
	Note      string
	SetStatus bool
	Status    *WatchStatus
}

// IsEmptyRating reports whether a rating carries no information.
func IsEmptyRating(r Rating) bool {
	return r.Score == nil && r.Note == "" && r.Status == nil
}

type RatingService struct {
	Store Store
	Now   Clock
}

func NewRatingService(s Store) *RatingService { return &RatingService{Store: s, Now: time.Now} }

func (s *RatingService) Get(ctx context.Context, userID, titleID string) (*Rating, error) {
	return s.Store.GetRating(ctx, userID, titleID)
}

// Update applies a partial change to the user's record for a title. The title is
// cached so the archive can list it without the source. Returns nil when the change
// cleared everything (the record is then removed).
func (s *RatingService) Update(ctx context.Context, userID string, title Title, p RatingPatch) (*Rating, error) {
	current, err := s.Store.GetRating(ctx, userID, title.ID)
	if err != nil {
		return nil, err
	}
	next := Rating{TitleID: title.ID, UpdatedAt: isoTime(s.Now())}
	if current != nil {
		next.Score, next.Note, next.Status = current.Score, current.Note, current.Status
	}
	if p.SetScore {
		if next.Score, err = ValidateScore(p.Score); err != nil {
			return nil, err
		}
	}
	if p.SetNote {
		if next.Note, err = ValidateNote(p.Note); err != nil {
			return nil, err
		}
	}
	if p.SetStatus {
		next.Status = p.Status
	}
	if IsEmptyRating(next) {
		if current != nil {
			if err := s.Store.DeleteRating(ctx, userID, title.ID); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if err := s.Store.UpsertTitle(ctx, title); err != nil {
		return nil, err
	}
	if err := s.Store.UpsertRating(ctx, userID, next); err != nil {
		return nil, err
	}
	return &next, nil
}

func (s *RatingService) Remove(ctx context.Context, userID, titleID string) error {
	return s.Store.DeleteRating(ctx, userID, titleID)
}

// ---------- Categories ----------

type CategoryInput struct {
	Name        string
	Color       CategoryColor
	Description string
}

// ValidateCategoryInput normalizes and checks a category form.
func ValidateCategoryInput(in CategoryInput) (CategoryInput, error) {
	name := normalizeName(in.Name)
	if name == "" {
		return in, newErr(ErrCategoryNameRequired)
	}
	if jsLen(name) > CategoryNameMax {
		return in, newErr(ErrCategoryNameTooLong)
	}
	desc := strings.TrimSpace(in.Description)
	if jsLen(desc) > CategoryDescriptionMax {
		return in, newErr(ErrCategoryDescriptionTooLong)
	}
	if _, ok := ParseCategoryColor(string(in.Color)); !ok {
		return in, newErr(ErrInvalidColor)
	}
	return CategoryInput{Name: name, Color: in.Color, Description: desc}, nil
}

type CategoryService struct {
	Store Store
	Now   Clock
	NewID func() string
}

func NewCategoryService(s Store) *CategoryService {
	return &CategoryService{Store: s, Now: time.Now, NewID: defaultID}
}

func (s *CategoryService) assertNameFree(ctx context.Context, userID, name, exceptID string) error {
	key := nameKey(name)
	all, err := s.Store.ListCategories(ctx, userID)
	if err != nil {
		return err
	}
	for _, c := range all {
		if c.ID != exceptID && nameKey(c.Name) == key {
			return newErr(ErrCategoryNameTaken)
		}
	}
	return nil
}

func (s *CategoryService) require(ctx context.Context, userID, id string) (*Category, error) {
	c, err := s.Store.GetCategory(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, newErr(ErrCategoryNotFound)
	}
	return c, nil
}

func (s *CategoryService) List(ctx context.Context, userID string) ([]CategoryWithCount, error) {
	return s.Store.ListCategories(ctx, userID)
}

func (s *CategoryService) Get(ctx context.Context, userID, id string) (*Category, error) {
	return s.Store.GetCategory(ctx, userID, id)
}

func (s *CategoryService) ListItems(ctx context.Context, userID, id string) ([]CategoryItemWithTitle, error) {
	return s.Store.ListCategoryItems(ctx, userID, id)
}

func (s *CategoryService) CategoryIDsForTitle(ctx context.Context, userID, titleID string) ([]string, error) {
	return s.Store.CategoryIDsForTitle(ctx, userID, titleID)
}

func (s *CategoryService) Create(ctx context.Context, userID string, in CategoryInput) (*Category, error) {
	valid, err := ValidateCategoryInput(in)
	if err != nil {
		return nil, err
	}
	if err := s.assertNameFree(ctx, userID, valid.Name, ""); err != nil {
		return nil, err
	}
	ts := isoTime(s.Now())
	c := Category{ID: s.NewID(), Name: valid.Name, Color: valid.Color, Description: valid.Description, CreatedAt: ts, UpdatedAt: ts}
	if err := s.Store.InsertCategory(ctx, userID, c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *CategoryService) Update(ctx context.Context, userID, id string, in CategoryInput) (*Category, error) {
	current, err := s.require(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	valid, err := ValidateCategoryInput(in)
	if err != nil {
		return nil, err
	}
	if err := s.assertNameFree(ctx, userID, valid.Name, id); err != nil {
		return nil, err
	}
	next := *current
	next.Name, next.Color, next.Description = valid.Name, valid.Color, valid.Description
	next.UpdatedAt = isoTime(s.Now())
	if err := s.Store.UpdateCategory(ctx, userID, next); err != nil {
		return nil, err
	}
	return &next, nil
}

func (s *CategoryService) Remove(ctx context.Context, userID, id string) error {
	return s.Store.DeleteCategory(ctx, userID, id)
}

// AddTitle adds a title to the end of a category. Adding twice is a no-op.
func (s *CategoryService) AddTitle(ctx context.Context, userID, categoryID string, title Title) error {
	if _, err := s.require(ctx, userID, categoryID); err != nil {
		return err
	}
	existing, err := s.Store.GetCategoryItem(ctx, userID, categoryID, title.ID)
	if err != nil || existing != nil {
		return err
	}
	if err := s.Store.UpsertTitle(ctx, title); err != nil {
		return err
	}
	maxPos, err := s.Store.MaxPosition(ctx, userID, categoryID)
	if err != nil {
		return err
	}
	return s.Store.InsertCategoryItem(ctx, userID, CategoryItem{
		CategoryID: categoryID, TitleID: title.ID, Position: maxPos + 1, AddedAt: isoTime(s.Now()),
	})
}

func (s *CategoryService) RemoveTitle(ctx context.Context, userID, categoryID, titleID string) error {
	return s.Store.DeleteCategoryItem(ctx, userID, categoryID, titleID)
}

// SetCategoriesForTitle puts the title in exactly the given set of categories.
func (s *CategoryService) SetCategoriesForTitle(ctx context.Context, userID string, title Title, categoryIDs []string) error {
	current, err := s.Store.CategoryIDsForTitle(ctx, userID, title.ID)
	if err != nil {
		return err
	}
	for _, id := range categoryIDs {
		if !slices.Contains(current, id) {
			if err := s.AddTitle(ctx, userID, id, title); err != nil {
				return err
			}
		}
	}
	for _, id := range current {
		if !slices.Contains(categoryIDs, id) {
			if err := s.Store.DeleteCategoryItem(ctx, userID, id, title.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
