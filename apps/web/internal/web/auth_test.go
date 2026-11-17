package web

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"filmarsivi/web/internal/auth"
	"filmarsivi/web/internal/store"
)

// fakeOIDC is a minimal OpenID provider: discovery, JWKS and a token endpoint that
// checks PKCE and returns RS256-signed ID tokens. No request ever goes to Google.
type fakeOIDC struct {
	t        *testing.T
	srv      *httptest.Server
	key      *rsa.PrivateKey
	clientID string

	mu    sync.Mutex
	codes map[string]pendingCode
}

type pendingCode struct {
	challenge string
	nonce     string
	claims    map[string]any
}

func newFakeOIDC(t *testing.T, clientID string) *fakeOIDC {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeOIDC{t: t, key: key, clientID: clientID, codes: map[string]pendingCode{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize",
			"token_endpoint": f.srv.URL + "/token", "jwks_uri": f.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", f.token)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOIDC) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	f.mu.Lock()
	pc, ok := f.codes[r.PostForm.Get("code")]
	delete(f.codes, r.PostForm.Get("code"))
	f.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != pc.challenge {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	now := time.Now()
	claims := map[string]any{"iss": f.srv.URL, "aud": f.clientID, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "nonce": pc.nonce}
	for k, v := range pc.claims {
		claims[k] = v
	}
	payload, _ := json.Marshal(claims)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		f.t.Fatal(err)
	}
	jws, _ := signer.Sign(payload)
	idToken, _ := jws.CompactSerialize()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": idToken})
}

// authorize plays the Google consent screen: it reads the authorization URL our
// server redirected to and issues a code for the given account.
func (f *fakeOIDC) authorize(authURL string, claims map[string]any) (state, code string) {
	u, err := url.Parse(authURL)
	if err != nil || !strings.HasPrefix(authURL, f.srv.URL+"/authorize") {
		f.t.Fatalf("unexpected authorization URL %q", authURL)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("state") == "" || q.Get("client_id") != f.clientID {
		f.t.Fatalf("missing PKCE/nonce/state: %s", authURL)
	}
	if q.Get("redirect_uri") != "http://example.com"+auth.CallbackPath {
		f.t.Fatalf("redirect_uri %q", q.Get("redirect_uri"))
	}
	code = auth.RandomToken()
	f.mu.Lock()
	f.codes[code] = pendingCode{challenge: q.Get("code_challenge"), nonce: q.Get("nonce"), claims: claims}
	f.mu.Unlock()
	return q.Get("state"), code
}

func account(sub, email string) map[string]any {
	return map[string]any{"sub": sub, "email": email, "email_verified": true, "name": strings.Split(email, "@")[0], "picture": ""}
}

type authHarness struct {
	*harness
	idp *fakeOIDC
}

func newAuthHarness(t *testing.T, mutate func(*auth.Config)) *authHarness {
	h := newHarness(t)
	idp := newFakeOIDC(t, "client-123")
	cfg := auth.Config{ClientID: "client-123", ClientSecret: "secret", BaseURL: "http://example.com", Issuer: idp.srv.URL}
	if mutate != nil {
		mutate(&cfg)
	}
	h.srv.Auth = auth.New(cfg, h.srv.Store.(*store.SQLite))
	h.h = h.srv.Handler()
	return &authHarness{harness: h, idp: idp}
}

// login runs the whole browser flow and returns the callback response.
func (a *authHarness) login(claims map[string]any) *httptest.ResponseRecorder {
	a.t.Helper()
	start := a.do("GET", "/auth/google?sonra=/arsiv", nil)
	if start.Code != http.StatusFound {
		a.t.Fatalf("login start: %d %s", start.Code, start.Body)
	}
	state, code := a.idp.authorize(start.Header().Get("Location"), claims)
	return a.do("GET", auth.CallbackPath+"?state="+url.QueryEscape(state)+"&code="+url.QueryEscape(code), nil)
}

// asNewBrowser forgets all cookies (another person on another device).
func (a *authHarness) asNewBrowser() map[string]string {
	old := a.cookies
	a.cookies = map[string]string{csrfCookie: testCSRF}
	return old
}

func TestLoginRequiredAndSucceeds(t *testing.T) {
	a := newAuthHarness(t, nil)
	if rec := a.do("GET", "/arsiv", nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/giris?sonra=%2Farsiv" {
		t.Fatalf("anonymous visitor not sent to login: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	a.get("/giris", 200, "Google ile oturum açın", "/auth/google")
	a.get("/hakkinda", 200, "Google ile giriş açık")
	if rec := a.do("POST", "/kategoriler", url.Values{"name": {"x"}, "color": {"red"}}); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/giris") {
		t.Fatal("anonymous POST must not write", rec.Code)
	}

	rec := a.login(account("sub-alice", "Alice@Example.com"))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/arsiv" {
		t.Fatalf("callback: %d %s %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	if a.cookies[sessionCookie] == "" {
		t.Fatal("no session cookie")
	}
	var sess *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			sess = c
		}
	}
	if !sess.HttpOnly || sess.SameSite != http.SameSiteLaxMode || sess.MaxAge < 29*24*3600 || sess.Secure {
		t.Fatalf("session cookie flags: %+v", sess)
	}
	a.get("/arsiv", 200, "Arşivin boş", "alice@example.com", "Çıkış yap")

	// state cookie is single-use: replaying the callback fails
	a.cookies[stateCookie] = "whatever"
	if rec := a.do("GET", auth.CallbackPath+"?state=whatever&code=x", nil); rec.Code != http.StatusBadRequest {
		t.Fatal("replayed callback accepted", rec.Code)
	}

	// logout ends the session server-side too
	token := a.cookies[sessionCookie]
	a.post("/auth/cikis", url.Values{}, http.StatusSeeOther)
	a.cookies[sessionCookie] = token // even if the browser kept the old cookie
	if rec := a.do("GET", "/arsiv", nil); rec.Code != http.StatusSeeOther {
		t.Fatal("session still valid after logout")
	}
}

func TestSecureCookiesOnHTTPS(t *testing.T) {
	a := newAuthHarness(t, func(c *auth.Config) { c.BaseURL = "https://film.example.com" })
	rec := a.do("GET", "/giris", nil)
	for _, c := range rec.Result().Cookies() {
		if !c.Secure {
			t.Fatalf("cookie %s not Secure on HTTPS", c.Name)
		}
	}
}

func TestLoginStateMismatch(t *testing.T) {
	a := newAuthHarness(t, nil)
	start := a.do("GET", "/auth/google", nil)
	_, code := a.idp.authorize(start.Header().Get("Location"), account("sub-alice", "alice@example.com"))

	// attacker's state, or a callback in a browser that never started the login
	rec := a.do("GET", auth.CallbackPath+"?state=someone-elses-state&code="+code, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Giriş yarıda kaldı") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if a.cookies[sessionCookie] != "" {
		t.Fatal("session created despite state mismatch")
	}
}

func TestLoginNotAllowedAndUnverified(t *testing.T) {
	a := newAuthHarness(t, func(c *auth.Config) { c.AllowedEmail = auth.ParseEmails(" alice@example.com , ") })
	rec := a.login(account("sub-bob", "bob@example.com"))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Bu uygulama davetle çalışıyor") {
		t.Fatalf("%d", rec.Code)
	}
	if a.cookies[sessionCookie] != "" {
		t.Fatal("uninvited user got a session")
	}
	unverified := account("sub-alice", "alice@example.com")
	unverified["email_verified"] = false
	if rec := a.login(unverified); rec.Code != http.StatusForbidden || a.cookies[sessionCookie] != "" {
		t.Fatal("unverified email accepted")
	}
	if rec := a.login(account("sub-alice", "ALICE@example.com")); rec.Code != http.StatusSeeOther {
		t.Fatal("invited user (case-insensitive) refused", rec.Code)
	}
}

func TestSessionExpires(t *testing.T) {
	a := newAuthHarness(t, nil)
	a.login(account("sub-alice", "alice@example.com"))
	a.get("/arsiv", 200)
	a.srv.Auth.Now = func() time.Time { return time.Now().Add(auth.SessionTTL + time.Hour) }
	if rec := a.do("GET", "/arsiv", nil); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/giris") {
		t.Fatal("expired session still works", rec.Code)
	}
	if a.cookies[sessionCookie] != "" {
		t.Fatal("expired session cookie not cleared")
	}
}

func TestUsersCannotSeeOrChangeEachOthersData(t *testing.T) {
	a := newAuthHarness(t, nil)
	a.login(account("sub-alice", "alice@example.com"))
	a.post("/icerik/anilist:154587/kayit", url.Values{"score": {"9"}, "note": {"alice'in notu"}}, http.StatusSeeOther)
	catURL := a.post("/kategoriler", url.Values{"name": {"Alice listesi"}, "color": {"teal"}}, http.StatusSeeOther).Header().Get("Location")
	catID := strings.TrimPrefix(catURL, "/kategoriler/")
	a.post("/icerik/anilist:154587/kategoriler", url.Values{"kategori": {catID}}, http.StatusSeeOther)
	alice := a.asNewBrowser()

	a.login(account("sub-bob", "bob@example.com"))
	a.get("/arsiv", 200, "Arşivin boş")
	a.get("/kategoriler", 200, "İlk kategorini oluştur")
	if body := a.get("/icerik/anilist:154587", 200); strings.Contains(body, "alice&#39;in notu") || strings.Contains(body, `value="9" checked`) || strings.Contains(body, "Alice listesi") {
		t.Fatal("bob sees alice's rating or category")
	}
	a.get(catURL, 404)
	a.get(catURL+"/duzenle", 404)
	a.post(catURL, url.Values{"name": {"ele geçirildi"}, "color": {"red"}}, 404)
	a.post(catURL+"/cikar", url.Values{"icerik": {"anilist:154587"}}, 404)
	a.post(catURL+"/sil", url.Values{}, 404)
	// adding alice's category id to bob's title is ignored
	a.post("/icerik/anilist:154587/kategoriler", url.Values{"kategori": {catID}}, http.StatusSeeOther)
	// bob may use the same category name: names are unique per user
	a.post("/kategoriler", url.Values{"name": {"Alice listesi"}, "color": {"red"}}, http.StatusSeeOther)

	a.cookies = alice
	a.get("/arsiv", 200, "Frieren")
	a.get(catURL, 200, "Alice listesi", "1 içerik", "cat-teal")
}

func TestOwnerReceivesSingleUserData(t *testing.T) {
	h := newHarness(t) // single-user mode first
	h.post("/icerik/anilist:154587/kayit", url.Values{"score": {"8"}}, http.StatusSeeOther)
	h.post("/kategoriler", url.Values{"name": {"Eski liste"}, "color": {"amber"}}, http.StatusSeeOther)

	idp := newFakeOIDC(t, "client-123")
	h.srv.Auth = auth.New(auth.Config{ClientID: "client-123", ClientSecret: "s", BaseURL: "http://example.com", Issuer: idp.srv.URL, OwnerEmail: "owner@example.com"}, h.srv.Store.(*store.SQLite))
	h.h = h.srv.Handler()
	a := &authHarness{harness: h, idp: idp}

	a.login(account("sub-guest", "guest@example.com"))
	a.get("/arsiv", 200, "Arşivin boş")
	a.asNewBrowser()
	a.login(account("sub-owner", "Owner@example.com"))
	a.get("/arsiv", 200, "Frieren")
	a.get("/kategoriler", 200, "Eski liste")
	// claimed once: a second owner login does not move anything again
	if claimed, _ := h.srv.Store.(*store.SQLite).ClaimLegacyData(t.Context(), auth.LegacyUserID, "someone"); claimed {
		t.Fatal("legacy data claimed twice")
	}
}

func TestCSRFTokenRequired(t *testing.T) {
	h := newHarness(t)
	rec := h.do("POST", "/kategoriler", url.Values{"name": {"x"}, "color": {"red"}, "csrf": {""}})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Form süresi doldu") {
		t.Fatal(rec.Code)
	}
	rec = h.do("POST", "/kategoriler", url.Values{"name": {"x"}, "color": {"red"}, "csrf": {"wrong-token-wrong-token-wrong-token-xx"}})
	if rec.Code != http.StatusForbidden {
		t.Fatal(rec.Code)
	}
	h.get("/kategoriler", 200, "İlk kategorini oluştur")
	if body := h.get("/icerik/anilist:154587", 200); strings.Count(body, `name="csrf" value="`+testCSRF+`"`) < 2 { // theme + rating forms
		t.Fatal("forms carry no CSRF token")
	}
}

func TestTapToSaveIsPerUser(t *testing.T) {
	a := newAuthHarness(t, nil)
	a.login(account("sub-alice", "alice@example.com"))
	if rec, r := a.postJSON("/icerik/anilist:154587/guncelle", url.Values{"alan": {"status"}, "deger": {"planned"}, "tersine": {"1"}}); rec.Code != 200 || r.Status == nil {
		t.Fatal(rec.Code, rec.Body)
	}
	a.asNewBrowser()
	// signed out: the save is refused (redirected to the login page), nothing written
	if rec, _ := a.postJSON("/icerik/anilist:154587/guncelle", url.Values{"alan": {"score"}, "deger": {"3"}}); rec.Code != http.StatusSeeOther {
		t.Fatal("anonymous autosave accepted", rec.Code)
	}
	a.login(account("sub-bob", "bob@example.com"))
	a.get("/arsiv", 200, "Arşivin boş")
	// bob's tap starts from bob's (empty) record: planned for him, alice's untouched
	_, r := a.postJSON("/icerik/anilist:154587/guncelle", url.Values{"alan": {"status"}, "deger": {"planned"}, "tersine": {"1"}})
	if r.Status == nil || *r.Status != "planned" {
		t.Fatalf("%+v", r)
	}
}
