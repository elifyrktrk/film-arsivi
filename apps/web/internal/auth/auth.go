// Package auth implements "Sign in with Google" as a server-side OpenID Connect
// Authorization Code flow with PKCE, state and nonce, plus server-side sessions.
// All crypto comes from golang.org/x/oauth2, github.com/coreos/go-oidc/v3 and the
// standard library; nothing here implements its own primitives.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	GoogleIssuer = "https://accounts.google.com"
	CallbackPath = "/auth/google/callback"
	SessionTTL   = 30 * 24 * time.Hour
	FlowTTL      = 10 * time.Minute
)

// Config comes from the environment. Auth is enabled only when ClientID and
// ClientSecret are both set.
type Config struct {
	ClientID     string
	ClientSecret string
	BaseURL      string   // e.g. http://localhost:8090 or https://film.example.com
	Issuer       string   // default GoogleIssuer; tests point it at a fake provider
	AllowedEmail []string // empty: anyone with a verified Google account
	OwnerEmail   string   // receives data created before sign-in existed
}

func (c Config) Enabled() bool { return c.ClientID != "" && c.ClientSecret != "" }

// SecureCookies is true when the site is served over HTTPS.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.BaseURL, "https://") }

// RedirectURL is the callback registered in Google Cloud Console.
func (c Config) RedirectURL() string { return strings.TrimRight(c.BaseURL, "/") + CallbackPath }

// Allowed reports whether a verified email may sign in.
func (c Config) Allowed(email string) bool {
	if len(c.AllowedEmail) == 0 {
		return true
	}
	for _, a := range c.AllowedEmail {
		if strings.EqualFold(strings.TrimSpace(a), email) {
			return true
		}
	}
	return false
}

// ParseEmails splits a comma-separated ALLOWED_EMAILS value.
func ParseEmails(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

// Identity is what we keep from a verified ID token.
type Identity struct {
	Subject string // Google's stable user id ("sub")
	Email   string
	Name    string
	Picture string
}

// User is a signed-in person.
type User struct {
	ID      string
	Email   string
	Name    string
	Picture string
}

// Flow is the server-side half of one login attempt, keyed by state.
type Flow struct {
	State     string
	Nonce     string
	Verifier  string // PKCE code verifier
	Next      string // where to go after login
	CreatedAt time.Time
}

// Store persists users, sessions and in-flight logins (implemented by store.SQLite).
type Store interface {
	SaveOAuthFlow(ctx context.Context, f Flow) error
	// TakeOAuthFlow returns and deletes the flow (one-time use); nil if unknown.
	TakeOAuthFlow(ctx context.Context, state string) (*Flow, error)
	UpsertGoogleUser(ctx context.Context, id Identity) (User, error)
	CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error
	// SessionUser returns the user and expiry for a session; nil if unknown.
	SessionUser(ctx context.Context, tokenHash string) (*User, time.Time, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteExpired(ctx context.Context, now time.Time) error
	// ClaimLegacyData moves data stored before sign-in existed (user "local") to
	// userID, once. Returns true when something was claimed now.
	ClaimLegacyData(ctx context.Context, legacyUserID, userID string) (bool, error)
}

var (
	ErrStateMismatch = errors.New("auth: state mismatch")
	ErrFlowExpired   = errors.New("auth: login attempt expired")
	ErrNotAllowed    = errors.New("auth: email not allowed")
	ErrUnverified    = errors.New("auth: email not verified")
)

// Service runs the login flow.
type Service struct {
	Config Config
	Store  Store
	HTTP   *http.Client
	Now    func() time.Time

	mu       sync.Mutex
	provider *oidc.Provider
}

func New(cfg Config, st Store) *Service {
	if cfg.Issuer == "" {
		cfg.Issuer = GoogleIssuer
	}
	return &Service{Config: cfg, Store: st, HTTP: &http.Client{Timeout: 10 * time.Second}, Now: time.Now}
}

// oidcProvider runs discovery lazily, so the app starts even when Google is
// unreachable, and retries on the next login.
func (s *Service) oidcProvider(ctx context.Context) (*oidc.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider != nil {
		return s.provider, nil
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, s.HTTP), s.Config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	s.provider = p
	return p, nil
}

func (s *Service) oauthConfig(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.Config.ClientID,
		ClientSecret: s.Config.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.Config.RedirectURL(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

// RandomToken returns 32 random bytes, base64url encoded.
func RandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is how session tokens are stored, so a leaked database holds no
// usable sessions.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Begin starts a login: stores state, nonce and PKCE verifier, and returns the
// state (for the browser-binding cookie) and Google's authorization URL.
func (s *Service) Begin(ctx context.Context, next string) (state, authURL string, err error) {
	p, err := s.oidcProvider(ctx)
	if err != nil {
		return "", "", err
	}
	f := Flow{State: RandomToken(), Nonce: RandomToken(), Verifier: oauth2.GenerateVerifier(), Next: next, CreatedAt: s.Now()}
	if err := s.Store.SaveOAuthFlow(ctx, f); err != nil {
		return "", "", err
	}
	u := s.oauthConfig(p).AuthCodeURL(f.State,
		oidc.Nonce(f.Nonce),
		oauth2.S256ChallengeOption(f.Verifier),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)
	return f.State, u, nil
}

// Result is a finished login.
type Result struct {
	User    User
	Token   string // raw session token for the cookie
	Expires time.Time
	Next    string
	Claimed bool // legacy data was moved to this user
}

// Finish completes the callback. cookieState is the state bound to this browser
// at Begin; it must equal the state Google sent back.
func (s *Service) Finish(ctx context.Context, cookieState string, q url.Values) (*Result, error) {
	state := q.Get("state")
	if state == "" || cookieState == "" || state != cookieState {
		return nil, ErrStateMismatch
	}
	f, err := s.Store.TakeOAuthFlow(ctx, state)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, ErrStateMismatch
	}
	if s.Now().Sub(f.CreatedAt) > FlowTTL {
		return nil, ErrFlowExpired
	}
	if e := q.Get("error"); e != "" {
		return nil, fmt.Errorf("auth: provider returned %q", e)
	}
	p, err := s.oidcProvider(ctx)
	if err != nil {
		return nil, err
	}
	octx := oidc.ClientContext(ctx, s.HTTP)
	tok, err := s.oauthConfig(p).Exchange(octx, q.Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return nil, fmt.Errorf("auth: code exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return nil, errors.New("auth: no id_token in response")
	}
	idt, err := p.VerifierContext(octx, &oidc.Config{ClientID: s.Config.ClientID, Now: s.Now}).Verify(octx, raw)
	if err != nil {
		return nil, fmt.Errorf("auth: id token: %w", err)
	}
	if idt.Nonce != f.Nonce {
		return nil, errors.New("auth: nonce mismatch")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idt.Claims(&claims); err != nil {
		return nil, err
	}
	if !claims.EmailVerified || claims.Email == "" {
		return nil, ErrUnverified
	}
	if !s.Config.Allowed(claims.Email) {
		return nil, ErrNotAllowed
	}
	user, err := s.Store.UpsertGoogleUser(ctx, Identity{Subject: idt.Subject, Email: strings.ToLower(claims.Email), Name: claims.Name, Picture: claims.Picture})
	if err != nil {
		return nil, err
	}
	res := &Result{User: user, Next: f.Next}
	if s.Config.OwnerEmail != "" && strings.EqualFold(s.Config.OwnerEmail, claims.Email) {
		if res.Claimed, err = s.Store.ClaimLegacyData(ctx, LegacyUserID, user.ID); err != nil {
			return nil, err
		}
	}
	res.Token, res.Expires = RandomToken(), s.Now().Add(SessionTTL)
	if err := s.Store.CreateSession(ctx, HashToken(res.Token), user.ID, res.Expires); err != nil {
		return nil, err
	}
	_ = s.Store.DeleteExpired(ctx, s.Now())
	return res, nil
}

// LegacyUserID owns data created in single-user mode (no sign-in configured).
const LegacyUserID = "local"

// Lookup returns the signed-in user for a raw session token, or nil when the
// session is unknown or expired.
func (s *Service) Lookup(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	u, exp, err := s.Store.SessionUser(ctx, HashToken(token))
	if err != nil || u == nil {
		return nil, err
	}
	if !s.Now().Before(exp) {
		_ = s.Store.DeleteSession(ctx, HashToken(token))
		return nil, nil
	}
	return u, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.Store.DeleteSession(ctx, HashToken(token))
}
