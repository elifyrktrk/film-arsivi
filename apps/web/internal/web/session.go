package web

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"filmarsivi/web/internal/auth"
)

const (
	sessionCookie = "oturum"
	csrfCookie    = "csrf"
	stateCookie   = "oauth_state"
)

type ctxKey int

const (
	userKey ctxKey = iota
	csrfKey
)

// uid is the data owner for this request: the signed-in user, or the single
// local user when sign-in is not configured.
func (s *Server) uid(r *http.Request) string {
	if u, ok := r.Context().Value(userKey).(*auth.User); ok && u != nil {
		return u.ID
	}
	return LocalUser
}

func userOf(r *http.Request) *auth.User {
	u, _ := r.Context().Value(userKey).(*auth.User)
	return u
}

func csrfOf(r *http.Request) string {
	v, _ := r.Context().Value(csrfKey).(string)
	return v
}

func (s *Server) secureCookies() bool { return s.Auth != nil && s.Auth.Config.SecureCookies() }

func (s *Server) cookie(name, value string, maxAge int, path string) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: path, MaxAge: maxAge, HttpOnly: true,
		Secure: s.secureCookies(), SameSite: http.SameSiteLaxMode}
}

// publicPath is reachable without signing in.
func publicPath(p string) bool {
	return p == "/giris" || p == "/hakkinda" || p == "/healthz" || p == "/tema" ||
		strings.HasPrefix(p, "/auth/") || strings.HasPrefix(p, "/static/")
}

// csrfAndAuth checks the double-submit CSRF token on every POST, then (when sign-in
// is on) loads the session and sends anonymous visitors to the login page.
func (s *Server) csrfAndAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) >= 32 {
			token = c.Value
		} else {
			token = auth.RandomToken()
			http.SetCookie(w, s.cookie(csrfCookie, token, 0, "/"))
		}
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			sent := r.PostFormValue("csrf")
			if sent == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
				back := "/"
				if ref, err := url.Parse(r.Referer()); err == nil && ref.Host == r.Host {
					back = safeBack(ref.RequestURI(), "/")
				}
				s.message(w, r.WithContext(context.WithValue(r.Context(), csrfKey, token)), http.StatusForbidden, messageData{
					Heading: s.T.Web.CSRFTitle, Body: s.T.Web.CSRFBody, LinkURL: back, LinkText: s.T.Web.Back})
				return
			}
		}
		ctx := context.WithValue(r.Context(), csrfKey, token)
		if s.Auth != nil {
			if c, err := r.Cookie(sessionCookie); err == nil {
				u, err := s.Auth.Lookup(ctx, c.Value)
				if err != nil {
					s.serverError(w, r.WithContext(ctx), err)
					return
				}
				if u != nil {
					ctx = context.WithValue(ctx, userKey, u)
				} else {
					http.SetCookie(w, s.cookie(sessionCookie, "", -1, "/"))
				}
			}
			if userOf(r.WithContext(ctx)) == nil && !publicPath(r.URL.Path) {
				target := "/giris"
				if r.Method == http.MethodGet && r.URL.Path != "/" {
					target += "?sonra=" + url.QueryEscape(r.URL.RequestURI())
				}
				http.Redirect(w, r, target, http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ---------- handlers ----------

type loginData struct {
	Next  string
	Error string
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	next := safeBack(r.URL.Query().Get("sonra"), "/")
	if s.Auth == nil || userOf(r) != nil {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "login", s.page(r, "", s.T.Web.LoginTitle, loginData{Next: next}))
}

func (s *Server) loginError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	s.render(w, r, status, "login", s.page(r, "", s.T.Web.LoginTitle, loginData{Next: "/", Error: msg}))
}

func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	state, authURL, err := s.Auth.Begin(r.Context(), safeBack(r.URL.Query().Get("sonra"), "/"))
	if err != nil {
		s.Log.Error("login start", "err", err)
		s.loginError(w, r, http.StatusBadGateway, s.T.Web.LoginUnavailable)
		return
	}
	// Binds this login attempt to this browser (login CSRF protection).
	http.SetCookie(w, s.cookie(stateCookie, state, int(auth.FlowTTL/time.Second), "/auth/"))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cookieState := ""
	if c, err := r.Cookie(stateCookie); err == nil {
		cookieState = c.Value
	}
	http.SetCookie(w, s.cookie(stateCookie, "", -1, "/auth/"))
	res, err := s.Auth.Finish(r.Context(), cookieState, r.URL.Query())
	switch {
	case errors.Is(err, auth.ErrNotAllowed):
		s.render(w, r, http.StatusForbidden, "invite", s.page(r, "", s.T.Web.InviteTitle, nil))
		return
	case errors.Is(err, auth.ErrStateMismatch), errors.Is(err, auth.ErrFlowExpired):
		s.loginError(w, r, http.StatusBadRequest, s.T.Web.LoginExpired)
		return
	case errors.Is(err, auth.ErrUnverified):
		s.loginError(w, r, http.StatusForbidden, s.T.Web.LoginUnverified)
		return
	case err != nil:
		s.Log.Warn("login failed", "err", err)
		s.loginError(w, r, http.StatusBadGateway, s.T.Web.LoginFailed)
		return
	}
	if res.Claimed {
		s.Log.Info("legacy data moved to owner", "user", res.User.ID)
	}
	c := s.cookie(sessionCookie, res.Token, int(time.Until(res.Expires)/time.Second), "/")
	http.SetCookie(w, c)
	// New CSRF token for the new session.
	http.SetCookie(w, s.cookie(csrfCookie, auth.RandomToken(), 0, "/"))
	http.Redirect(w, r, safeBack(res.Next, "/"), http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if s.Auth != nil {
		if c, err := r.Cookie(sessionCookie); err == nil {
			if err := s.Auth.Logout(r.Context(), c.Value); err != nil {
				s.serverError(w, r, err)
				return
			}
		}
	}
	http.SetCookie(w, s.cookie(sessionCookie, "", -1, "/"))
	http.Redirect(w, r, "/giris", http.StatusSeeOther)
}
