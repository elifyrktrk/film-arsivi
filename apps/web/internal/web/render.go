package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"filmarsivi/web/internal/domain"
	"filmarsivi/web/internal/i18n"
)

// Page is what every template receives.
type Page struct {
	T           i18n.Strings
	Title       string
	Nav         string // search | archive | categories | about
	Theme       string // system | light | dark
	Path        string // current path + query, for "back" links
	TMDBEnabled bool
	Flash       string
	Error       string
	Data        any
}

func parseTemplates(funcs template.FuncMap) (map[string]*template.Template, error) {
	names, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	pages := map[string]*template.Template{}
	for _, n := range names {
		base := path.Base(n)
		if base == "layout.html" || base == "partials.html" {
			continue
		}
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/partials.html", n)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		pages[strings.TrimSuffix(base, ".html")] = t
	}
	return pages, nil
}

func (s *Server) funcs() template.FuncMap {
	return template.FuncMap{
		"kindLabel":   func(k domain.MediaKind) string { return s.T.Kind[string(k)] },
		"sourceLabel": func(src domain.Source) string { return s.T.Source[string(src)] },
		"statusLabel": func(st any) string {
			switch v := st.(type) {
			case domain.WatchStatus:
				return s.T.Status[string(v)]
			case *domain.WatchStatus:
				if v != nil {
					return s.T.Status[string(*v)]
				}
			}
			return ""
		},
		"titleURL":    titleURL,
		"categoryURL": categoryURL,
		"initials": func(name string) string {
			var out []rune
			for _, w := range strings.Fields(name) {
				r, _ := utf8.DecodeRuneInString(w)
				if r != utf8.RuneError && len(out) < 2 {
					out = append(out, r)
				}
			}
			return strings.ToUpper(string(out))
		},
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, fmt.Errorf("dict: odd number of arguments")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"community": s.communityFor,
		"scores":    func() []int { return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} },
		"statuses":  func() []domain.WatchStatus { return domain.WatchStatuses },
		"colors":    func() []domain.CategoryColor { return domain.CategoryColors },
		"join":      strings.Join,
		"deref": func(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		},
		"eqp": func(p *int, v int) bool { return p != nil && *p == v },
		"has": func(list []string, v string) bool {
			for _, x := range list {
				if x == v {
					return true
				}
			}
			return false
		},
		"paragraphs": func(s *string) []string {
			if s == nil {
				return nil
			}
			var out []string
			for _, p := range strings.Split(*s, "\n\n") {
				if p = strings.TrimSpace(p); p != "" {
					out = append(out, p)
				}
			}
			return out
		},
		"limit": func(n int, ts []domain.Title) []domain.Title {
			if len(ts) > n {
				return ts[:n]
			}
			return ts
		},
	}
}

func themeOf(r *http.Request) string {
	if c, err := r.Cookie("tema"); err == nil && (c.Value == "light" || c.Value == "dark") {
		return c.Value
	}
	return "system"
}

func (s *Server) page(r *http.Request, nav, title string, data any) Page {
	return Page{
		T: s.T, Title: title, Nav: nav, Theme: themeOf(r), Path: r.URL.RequestURI(),
		TMDBEnabled: s.Search.TMDBEnabled(), Data: data,
	}
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name string, p Page) {
	t, ok := s.pages[name]
	if !ok {
		s.serverError(w, r, fmt.Errorf("no template %q", name))
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout.html", p); err != nil {
		s.Log.Error("render", "page", name, "err", err)
		http.Error(w, s.T.Web.ServerError, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

type messageData struct {
	Heading, Body, LinkURL, LinkText string
}

func (s *Server) message(w http.ResponseWriter, r *http.Request, status int, m messageData) {
	s.render(w, r, status, "message", s.page(r, "", m.Heading, m))
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.message(w, r, http.StatusNotFound, messageData{
		Heading: s.T.Web.NotFoundTitle, Body: s.T.Web.NotFoundBody, LinkURL: "/", LinkText: s.T.Archive.GoSearch,
	})
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("server error", "path", r.URL.Path, "err", err)
	s.message(w, r, http.StatusInternalServerError, messageData{
		Heading: s.T.Web.ServerError, LinkURL: r.URL.RequestURI(), LinkText: s.T.Search.Retry,
	})
}

// errorText turns a domain error into its Turkish message.
func (s *Server) errorText(err error) string {
	if code, ok := domain.CodeOf(err); ok {
		return s.T.Error(string(code))
	}
	return s.T.Errors["generic"]
}
