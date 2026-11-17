// Package store implements domain.Store: Memory (reference behavior, used in tests)
// and SQLite (the server's storage). Both pass the same contract tests.
package store

import (
	"context"
	"slices"
	"strings"
	"sync"

	"filmarsivi/web/internal/domain"
)

// Store is the storage contract every implementation satisfies.
type Store = domain.Store

type memUserRating struct{ user, title string }

// Memory is an in-memory Store, the Go twin of packages/core/src/memory.ts.
type Memory struct {
	mu         sync.Mutex
	titles     map[string]domain.Title
	ratings    map[memUserRating]domain.Rating
	categories map[string]memCategory
	items      []domain.CategoryItem
}

type memCategory struct {
	user string
	domain.Category
}

func NewMemory() *Memory {
	return &Memory{
		titles:     map[string]domain.Title{},
		ratings:    map[memUserRating]domain.Rating{},
		categories: map[string]memCategory{},
	}
}

var _ domain.Store = (*Memory)(nil)

func ptr[T any](v T) *T { return &v }

func (m *Memory) GetTitle(_ context.Context, id string) (*domain.Title, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.titles[id]; ok {
		return &t, nil
	}
	return nil, nil
}

func (m *Memory) UpsertTitle(_ context.Context, t domain.Title) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.titles[t.ID] = t
	return nil
}

func (m *Memory) GetRating(_ context.Context, userID, titleID string) (*domain.Rating, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.ratings[memUserRating{userID, titleID}]; ok {
		return &r, nil
	}
	return nil, nil
}

func (m *Memory) UpsertRating(_ context.Context, userID string, r domain.Rating) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ratings[memUserRating{userID, r.TitleID}] = r
	return nil
}

func (m *Memory) DeleteRating(_ context.Context, userID, titleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.ratings, memUserRating{userID, titleID})
	return nil
}

func (m *Memory) ListArchive(_ context.Context, userID string, q domain.ArchiveQuery) ([]domain.ArchiveEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.ArchiveEntry{}
	for k, r := range m.ratings {
		if k.user != userID {
			continue
		}
		t, ok := m.titles[r.TitleID]
		if !ok {
			continue
		}
		if q.Status != "" && (r.Status == nil || *r.Status != q.Status) {
			continue
		}
		out = append(out, domain.ArchiveEntry{Title: t, Rating: r})
	}
	// Map order is random; fix a base order so ties sort deterministically.
	slices.SortFunc(out, func(a, b domain.ArchiveEntry) int { return strings.Compare(a.Title.ID, b.Title.ID) })
	domain.SortArchive(out, q.Sort)
	return out, nil
}

func (m *Memory) itemsOf(categoryID string) []domain.CategoryItem {
	var own []domain.CategoryItem
	for _, i := range m.items {
		if i.CategoryID == categoryID {
			own = append(own, i)
		}
	}
	slices.SortStableFunc(own, func(a, b domain.CategoryItem) int { return a.Position - b.Position })
	return own
}

func (m *Memory) ListCategories(_ context.Context, userID string) ([]domain.CategoryWithCount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.CategoryWithCount{}
	for _, c := range m.categories {
		if c.user != userID {
			continue
		}
		own := m.itemsOf(c.ID)
		byNewest := slices.Clone(own)
		slices.SortStableFunc(byNewest, func(a, b domain.CategoryItem) int { return strings.Compare(b.AddedAt, a.AddedAt) })
		posters := []string{}
		for _, i := range byNewest {
			if t, ok := m.titles[i.TitleID]; ok && t.PosterURL != nil && *t.PosterURL != "" && len(posters) < 3 {
				posters = append(posters, *t.PosterURL)
			}
		}
		out = append(out, domain.CategoryWithCount{Category: c.Category, ItemCount: len(own), PreviewPosters: posters})
	}
	slices.SortStableFunc(out, func(a, b domain.CategoryWithCount) int {
		if d := strings.Compare(a.CreatedAt, b.CreatedAt); d != 0 {
			return d
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (m *Memory) GetCategory(_ context.Context, userID, id string) (*domain.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.categories[id]; ok && c.user == userID {
		return ptr(c.Category), nil
	}
	return nil, nil
}

func (m *Memory) InsertCategory(_ context.Context, userID string, c domain.Category) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.categories[c.ID] = memCategory{userID, c}
	return nil
}

func (m *Memory) UpdateCategory(_ context.Context, userID string, c domain.Category) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.categories[c.ID]; ok && cur.user == userID {
		m.categories[c.ID] = memCategory{userID, c}
	}
	return nil
}

func (m *Memory) owns(userID, categoryID string) bool {
	c, ok := m.categories[categoryID]
	return ok && c.user == userID
}

func (m *Memory) DeleteCategory(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.owns(userID, id) {
		return nil
	}
	delete(m.categories, id)
	m.items = slices.DeleteFunc(m.items, func(i domain.CategoryItem) bool { return i.CategoryID == id })
	return nil
}

func (m *Memory) ListCategoryItems(_ context.Context, userID, categoryID string) ([]domain.CategoryItemWithTitle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.CategoryItemWithTitle{}
	if !m.owns(userID, categoryID) {
		return out, nil
	}
	for _, i := range m.itemsOf(categoryID) {
		if t, ok := m.titles[i.TitleID]; ok {
			out = append(out, domain.CategoryItemWithTitle{CategoryItem: i, Title: t})
		}
	}
	return out, nil
}

func (m *Memory) GetCategoryItem(_ context.Context, userID, categoryID, titleID string) (*domain.CategoryItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.owns(userID, categoryID) {
		return nil, nil
	}
	for _, i := range m.items {
		if i.CategoryID == categoryID && i.TitleID == titleID {
			return ptr(i), nil
		}
	}
	return nil, nil
}

func (m *Memory) InsertCategoryItem(_ context.Context, userID string, item domain.CategoryItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owns(userID, item.CategoryID) {
		m.items = append(m.items, item)
	}
	return nil
}

func (m *Memory) DeleteCategoryItem(_ context.Context, userID, categoryID, titleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.owns(userID, categoryID) {
		return nil
	}
	m.items = slices.DeleteFunc(m.items, func(i domain.CategoryItem) bool {
		return i.CategoryID == categoryID && i.TitleID == titleID
	})
	return nil
}

func (m *Memory) MaxPosition(_ context.Context, userID, categoryID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	max := -1
	for _, i := range m.itemsOf(categoryID) {
		if i.Position > max {
			max = i.Position
		}
	}
	return max, nil
}

func (m *Memory) CategoryIDsForTitle(_ context.Context, userID, titleID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []string{}
	for _, i := range m.items {
		if i.TitleID == titleID && m.owns(userID, i.CategoryID) {
			out = append(out, i.CategoryID)
		}
	}
	return out, nil
}
