// Package web serves the server-rendered HTML site.
package web

//go:generate go run ../../cmd/synctokens ../../../../packages/tokens/dist/tokens.css static/tokens.css

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"filmarsivi/web/internal/domain"
	"filmarsivi/web/internal/i18n"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

func init() {
	// Go does not know this type; with nosniff, text/plain would make browsers ignore it.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// LocalUser is the only user until sign-in exists. Every store call already takes a
// user id, so adding accounts means replacing this constant with the session's user.
const LocalUser = "local"

type Server struct {
	Search     *domain.SearchService
	Ratings    *domain.RatingService
	Categories *domain.CategoryService
	Store      domain.Store
	Log        *slog.Logger

	T     i18n.Strings
	pages map[string]*template.Template
	cache *titleCache
}

func New(search *domain.SearchService, st domain.Store, log *slog.Logger) (*Server, error) {
	s := &Server{
		Search:     search,
		Ratings:    domain.NewRatingService(st),
		Categories: domain.NewCategoryService(st),
		Store:      st,
		Log:        log,
		T:          i18n.TR,
		cache:      &titleCache{ttl: 15 * time.Minute, items: map[string]cachedTitle{}},
	}
	pages, err := parseTemplates(s.funcs())
	if err != nil {
		return nil, err
	}
	s.pages = pages
	return s, nil
}

// Handler returns the site's routes wrapped in common middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheStatic(http.FileServerFS(static))))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	mux.HandleFunc("GET /{$}", s.searchPage)
	mux.HandleFunc("GET /arsiv", s.archivePage)
	mux.HandleFunc("GET /kategoriler", s.categoriesPage)
	mux.HandleFunc("GET /kategoriler/yeni", s.newCategoryPage)
	mux.HandleFunc("POST /kategoriler", s.createCategory)
	mux.HandleFunc("GET /kategoriler/{id}", s.categoryPage)
	mux.HandleFunc("GET /kategoriler/{id}/duzenle", s.editCategoryPage)
	mux.HandleFunc("POST /kategoriler/{id}", s.updateCategory)
	mux.HandleFunc("GET /kategoriler/{id}/sil", s.deleteCategoryPage)
	mux.HandleFunc("POST /kategoriler/{id}/sil", s.deleteCategory)
	mux.HandleFunc("POST /kategoriler/{id}/cikar", s.removeFromCategory)
	mux.HandleFunc("GET /icerik/{id}", s.titlePage)
	mux.HandleFunc("POST /icerik/{id}/kayit", s.saveRating)
	mux.HandleFunc("POST /icerik/{id}/kategoriler", s.saveTitleCategories)
	mux.HandleFunc("GET /hakkinda", s.aboutPage)
	mux.HandleFunc("POST /tema", s.setTheme)
	mux.HandleFunc("/", s.notFound)
	return s.middleware(mux)
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data:; script-src 'self'; style-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		// No sign-in yet, but refuse cross-site form posts all the same.
		if r.Method == http.MethodPost && !sameOrigin(r) {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
		defer func() {
			if v := recover(); v != nil {
				s.Log.Error("panic", "err", v, "path", r.URL.Path)
				s.serverError(w, r, errors.New("panic"))
			}
		}()
		start := time.Now()
		next.ServeHTTP(w, r)
		s.Log.Debug("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	})
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

// ---------- title cache ----------

// titleCache keeps recently fetched source titles so a detail page and the form post
// that follows it do not both hit TMDB/AniList.
type titleCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]cachedTitle
}

type cachedTitle struct {
	t   domain.Title
	exp time.Time
}

func (c *titleCache) get(id string) (domain.Title, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[id]
	if !ok || time.Now().After(it.exp) {
		return domain.Title{}, false
	}
	return it.t, true
}

func (c *titleCache) put(t domain.Title) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) > 1000 {
		clear(c.items)
	}
	c.items[t.ID] = cachedTitle{t: t, exp: time.Now().Add(c.ttl)}
}

var errTitleNotFound = errors.New("title not found")

// loadTitle returns a title from the cache, its source, or — when the source is down
// or has dropped it — the locally stored copy. stale is true when the stored copy was
// used because the source failed.
func (s *Server) loadTitle(ctx context.Context, id string) (t domain.Title, stale bool, err error) {
	if t, ok := s.cache.get(id); ok {
		return t, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	fresh, srcErr := s.Search.GetTitle(ctx, id)
	if srcErr == nil && fresh != nil {
		s.cache.put(*fresh)
		if stored, _ := s.Store.GetTitle(ctx, id); stored != nil {
			_ = s.Store.UpsertTitle(ctx, *fresh) // keep the archive copy current
		}
		return *fresh, false, nil
	}
	stored, err := s.Store.GetTitle(ctx, id)
	if err != nil {
		return t, false, err
	}
	if stored != nil {
		return *stored, srcErr != nil, nil
	}
	if srcErr != nil {
		s.Log.Warn("title source failed", "id", id, "err", srcErr)
		return t, false, srcErr
	}
	return t, false, errTitleNotFound
}

// ---------- helpers ----------

func safeBack(raw, fallback string) string {
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") && !strings.HasPrefix(raw, "/\\") {
		return raw
	}
	return fallback
}

func titleURL(id string) string { return "/icerik/" + url.PathEscape(id) }

func categoryURL(id string) string { return "/kategoriler/" + url.PathEscape(id) }
