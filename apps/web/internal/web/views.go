package web

import (
	"net/http"
	"net/url"

	"filmarsivi/web/internal/domain"
)

// itemView is one title in a list or grid (search, archive, category).
type itemView struct {
	Title domain.Title
	Score *int   // the user's own score, if any
	Href  string // detail link (may carry ?geri=)
}

const (
	viewGrid = "izgara"
	viewList = "liste"
)

// viewMode picks grid or list for the archive and category pages. ?gorunum= switches
// and is remembered in a cookie; the default is the grid.
func (s *Server) viewMode(w http.ResponseWriter, r *http.Request) string {
	if v := r.URL.Query().Get("gorunum"); v == viewGrid || v == viewList {
		http.SetCookie(w, &http.Cookie{Name: "gorunum", Value: v, Path: "/", MaxAge: 365 * 24 * 3600,
			HttpOnly: true, Secure: s.secureCookies(), SameSite: http.SameSiteLaxMode})
		return v
	}
	if c, err := r.Cookie("gorunum"); err == nil && c.Value == viewList {
		return viewList
	}
	return viewGrid
}

type viewToggle struct {
	Mode             string
	GridURL, ListURL string
}

// toggleFor builds the grid/list links for the current URL, keeping its other params.
func toggleFor(r *http.Request, mode string) viewToggle {
	link := func(v string) string {
		q := r.URL.Query()
		q.Set("gorunum", v)
		return (&url.URL{Path: r.URL.Path, RawQuery: q.Encode()}).String()
	}
	return viewToggle{Mode: mode, GridURL: link(viewGrid), ListURL: link(viewList)}
}
