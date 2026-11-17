package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"filmarsivi/web/internal/domain"
	"filmarsivi/web/internal/store"
)

func sp(s string) *string { return &s }
func ip(i int) *int       { return &i }

type fakeSource struct {
	titles map[string]domain.Title // by source id
	err    error
}

func (f *fakeSource) Search(_ context.Context, q string) ([]domain.Title, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []domain.Title
	for _, t := range f.titles {
		if strings.Contains(strings.ToLower(t.Name), strings.ToLower(q)) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeSource) GetByID(_ context.Context, id string) (*domain.Title, error) {
	if f.err != nil {
		return nil, f.err
	}
	if t, ok := f.titles[id]; ok {
		return &t, nil
	}
	return nil, nil
}

var frieren = domain.Title{
	ID: "anilist:154587", Source: domain.SourceAniList, SourceID: "154587", Kind: domain.KindAnime,
	Name: "Frieren: Beyond Journey's End", OriginalName: sp("Sousou no Frieren"), Year: ip(2023),
	Overview: sp("Elf mage.\n\nSecond paragraph."), Genres: []string{"Adventure", "Drama"}, Format: sp("TV"), EpisodeCount: ip(28),
}

type harness struct {
	t       *testing.T
	srv     *Server
	h       http.Handler
	anilist *fakeSource
	cookies map[string]string // a tiny cookie jar
}

const testCSRF = "test-csrf-token-0123456789abcdef0123456789"

func newHarness(t *testing.T) *harness {
	anilist := &fakeSource{titles: map[string]domain.Title{"154587": frieren}}
	st, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv, err := New(&domain.SearchService{AniList: anilist}, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, srv: srv, h: srv.Handler(), anilist: anilist, cookies: map[string]string{csrfCookie: testCSRF}}
}

func (h *harness) do(method, target string, form url.Values) *httptest.ResponseRecorder {
	h.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	if form != nil {
		if _, explicit := form["csrf"]; !explicit {
			form.Set("csrf", h.cookies[csrfCookie])
		}
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.com")
	}
	for k, v := range h.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(h.cookies, c.Name)
		} else {
			h.cookies[c.Name] = c.Value
		}
	}
	return rec
}

func (h *harness) get(target string, wantStatus int, wantText ...string) string {
	h.t.Helper()
	rec := h.do("GET", target, nil)
	if rec.Code != wantStatus {
		h.t.Fatalf("GET %s: status %d, want %d\n%s", target, rec.Code, wantStatus, rec.Body)
	}
	body := rec.Body.String()
	for _, w := range wantText {
		if !strings.Contains(body, w) {
			h.t.Fatalf("GET %s: missing %q", target, w)
		}
	}
	return body
}

func (h *harness) post(target string, form url.Values, wantStatus int) *httptest.ResponseRecorder {
	h.t.Helper()
	rec := h.do("POST", target, form)
	if rec.Code != wantStatus {
		h.t.Fatalf("POST %s: status %d, want %d\n%s", target, rec.Code, wantStatus, rec.Body)
	}
	return rec
}

func TestPagesRender(t *testing.T) {
	h := newHarness(t)
	h.get("/", 200, "Ne izledin, ne izleyeceksin?", "Şu an sadece animeler aranıyor")
	h.get("/?q=f", 200, "En az 2 harf yaz")
	h.get("/?q=frieren", 200, "Frieren: Beyond Journey&#39;s End", "AniList", "/icerik/anilist:154587")
	h.get("/?q=zzzz", 200, "“zzzz” için sonuç yok")
	h.get("/arsiv", 200, "Arşivin boş", "Aramaya git")
	h.get("/kategoriler", 200, "İlk kategorini oluştur")
	h.get("/kategoriler/yeni", 200, `name="name"`, `value="violet"`, `aria-label="Pembe"`)
	h.get("/hakkinda", 200, "Bu ürün TMDB API&#39;sini kullanır", "This product uses the TMDB API but is not endorsed or certified by TMDB.", "TMDB anahtarı yok")
	h.get("/icerik/anilist:154587", 200, "Sousou no Frieren", "28 bölüm", "Second paragraph.", "Henüz kategorin yok.")
	h.get("/icerik/anilist:999", 404, "İçerik bulunamadı.")
	h.get("/icerik/tmdb:movie:1", 404) // TMDB disabled and nothing stored
	h.get("/kategoriler/yok", 404, "Kategori bulunamadı.")
	h.get("/nope", 404, "Sayfa bulunamadı")
	h.get("/static/tokens.css", 200, "--color-accent")
	h.get("/static/app.css", 200, "aspect-ratio: 2 / 3")
}

func TestSearchFailureShowsRetry(t *testing.T) {
	h := newHarness(t)
	h.anilist.err = errors.New("down")
	h.get("/?q=frieren", http.StatusBadGateway, "Arama yapılamadı", "Tekrar dene")
}

func TestRatingFlow(t *testing.T) {
	h := newHarness(t)
	rec := h.post("/icerik/anilist:154587/kayit", url.Values{"score": {"9"}, "status": {"watched"}, "note": {"  çok iyi  "}}, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/icerik/anilist:154587?kaydedildi=puan") {
		t.Fatal(loc)
	}
	body := h.get("/icerik/anilist:154587?kaydedildi=puan", 200, "Kaydedildi", ">çok iyi</textarea>")
	if !strings.Contains(body, `value="9" checked`) || !strings.Contains(body, `value="watched" checked`) {
		t.Fatal("saved score/status not preselected")
	}
	h.get("/arsiv", 200, "Frieren", `class="score-badge"`)
	h.get("/arsiv?durum=watched&sirala=score", 200, "Frieren")
	h.get("/arsiv?durum=planned", 200, "Bu filtrede içerik yok.")
	h.get("/?q=frieren", 200, `class="row-score"`, `class="rows"`)

	// invalid score is rejected with the Turkish message and keeps input
	rec = h.post("/icerik/anilist:154587/kayit", url.Values{"score": {"11"}, "note": {"x"}}, http.StatusUnprocessableEntity)
	if !strings.Contains(rec.Body.String(), "Puan 1 ile 10 arasında olmalı.") {
		t.Fatal(rec.Body.String())
	}
	rec = h.post("/icerik/anilist:154587/kayit", url.Values{"note": {strings.Repeat("a", 1001)}}, http.StatusUnprocessableEntity)
	if !strings.Contains(rec.Body.String(), "Not en fazla 1000 karakter olabilir.") {
		t.Fatal("note limit")
	}

	// clearing everything removes the record
	h.post("/icerik/anilist:154587/kayit", url.Values{"score": {""}, "status": {""}, "note": {""}}, http.StatusSeeOther)
	h.get("/arsiv", 200, "Arşivin boş")

	// archive keeps working from the stored copy when the source is down
	h.post("/icerik/anilist:154587/kayit", url.Values{"score": {"7"}}, http.StatusSeeOther)
	h.srv.cache = &titleCache{items: map[string]cachedTitle{}}
	h.anilist.err = errors.New("down")
	h.get("/icerik/anilist:154587", 200, "Bu içerik yüklenemedi.", "Sousou no Frieren")
}

func TestCategoryFlow(t *testing.T) {
	h := newHarness(t)
	rec := h.post("/kategoriler", url.Values{"name": {"  Pazar   akşamı "}, "color": {"teal"}, "description": {"rahat"}}, http.StatusSeeOther)
	catURL := rec.Header().Get("Location")
	if !strings.HasPrefix(catURL, "/kategoriler/") {
		t.Fatal(catURL)
	}
	h.get(catURL, 200, "Pazar akşamı", "rahat", "Bu kategori boş.")
	h.get("/kategoriler", 200, "Pazar akşamı", "Boş", "cat-teal")

	// duplicate name (case and spacing insensitive, Turkish İ/i)
	rec = h.post("/kategoriler", url.Values{"name": {"PAZAR AKŞAMI"}, "color": {"red"}}, http.StatusUnprocessableEntity)
	if !strings.Contains(rec.Body.String(), "Bu adda bir kategorin zaten var.") || !strings.Contains(rec.Body.String(), `value="PAZAR AKŞAMI"`) {
		t.Fatal("duplicate not rejected with input kept")
	}
	h.post("/kategoriler", url.Values{"name": {" "}, "color": {"red"}}, http.StatusUnprocessableEntity)
	h.post("/kategoriler", url.Values{"name": {"x"}, "color": {"gold"}}, http.StatusUnprocessableEntity)

	// add from the title page, then remove from the category page
	catID := strings.TrimPrefix(catURL, "/kategoriler/")
	h.post("/icerik/anilist:154587/kategoriler", url.Values{"kategori": {catID}}, http.StatusSeeOther)
	h.get("/icerik/anilist:154587?kaydedildi=kategoriler", 200, "Kategoriler güncellendi", `value="`+catID+`" checked`)
	h.get(catURL, 200, "Frieren", "1 içerik", "Çıkar")
	h.post(catURL+"/cikar", url.Values{"icerik": {"anilist:154587"}}, http.StatusSeeOther)
	h.get(catURL, 200, "Bu kategori boş.")

	// create-and-add from a title page
	rec = h.post("/kategoriler", url.Values{"name": {"Anime"}, "color": {"pink"}, "icerik": {"anilist:154587"}}, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/icerik/anilist:154587") {
		t.Fatal(loc)
	}

	// edit
	h.get(catURL+"/duzenle", 200, "Kategoriyi düzenle", `value="Pazar akşamı"`)
	h.post(catURL, url.Values{"name": {"pazar akşamı"}, "color": {"amber"}}, http.StatusSeeOther)
	h.get(catURL, 200, "pazar akşamı", "cat-amber")

	// delete with confirmation page
	h.get(catURL+"/sil", 200, "Kategori silinsin mi?")
	h.post(catURL+"/sil", url.Values{}, http.StatusSeeOther)
	h.get(catURL, 404)
	h.get("/kategoriler", 200, "Anime")
}

func TestThemeAndSafety(t *testing.T) {
	h := newHarness(t)
	rec := h.post("/tema", url.Values{"tema": {"dark"}, "geri": {"//evil.example"}}, http.StatusSeeOther)
	if rec.Header().Get("Location") != "/" {
		t.Fatal("open redirect", rec.Header().Get("Location"))
	}
	if c := rec.Result().Cookies(); len(c) != 1 || c[0].Value != "dark" {
		t.Fatal(c)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "tema", Value: "dark"})
	out := httptest.NewRecorder()
	h.h.ServeHTTP(out, req)
	if !strings.Contains(out.Body.String(), `data-theme="dark"`) {
		t.Fatal("theme attribute missing")
	}

	req = httptest.NewRequest("POST", "/kategoriler", strings.NewReader("name=x&color=red"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	out = httptest.NewRecorder()
	h.h.ServeHTTP(out, req)
	if out.Code != http.StatusForbidden {
		t.Fatal("cross-origin post accepted", out.Code)
	}
}

// TestTokensCopyIsFresh fails when packages/tokens changed and `go generate ./...`
// was not run, so the web never drifts from the shared design tokens.
func TestTokensCopyIsFresh(t *testing.T) {
	src, err := os.ReadFile("../../../../packages/tokens/dist/tokens.css")
	if err != nil {
		t.Skip("packages/tokens not found next to apps/web")
	}
	copy, err := staticFS.ReadFile("static/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, copy) {
		t.Fatal("static/tokens.css is stale: run `go generate ./...` in apps/web")
	}
}

func TestPWAFiles(t *testing.T) {
	h := newHarness(t)
	h.get("/", 200, `rel="manifest"`, `rel="apple-touch-icon"`, `apple-mobile-web-app-capable`, `viewport-fit=cover`)
	m := h.get("/static/manifest.webmanifest", 200, `"display": "standalone"`, `"name": "Film Arşivi"`)
	if ct := h.do("GET", "/static/manifest.webmanifest", nil).Header().Get("Content-Type"); ct != "application/manifest+json" {
		t.Fatal("manifest content type", ct)
	}
	tokens, _ := staticFS.ReadFile("static/tokens.css")
	for _, c := range []string{"#f6f6f9", "#ffffff"} { // background_color, theme_color
		if !strings.Contains(m, c) || !strings.Contains(string(tokens), c) {
			t.Fatalf("manifest color %s must come from tokens.css", c)
		}
	}
	for _, p := range []string{"/static/apple-touch-icon.png", "/static/icon-192.png", "/static/icon-512.png"} {
		rec := h.do("GET", p, nil)
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
			t.Fatal(p, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

func TestCommunityScoreFormatting(t *testing.T) {
	cases := map[*int]string{ip(89): "8,9", ip(80): "8,0", ip(7): "0,7", nil: "", ip(0): ""}
	for in, want := range cases {
		if got := AniListScore(in); got != want {
			t.Errorf("AniListScore(%v) = %q, want %q", in, got, want)
		}
	}
	for n, want := range map[int]string{850: "850", 2093: "2,1 B", 45000: "45 B", 1000: "1 B", 2100345: "2,1 Mn", 12_000_000: "12 Mn"} {
		if got := VotesTR(n); got != want {
			t.Errorf("VotesTR(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestCommunityScoresOnPages(t *testing.T) {
	h := newHarness(t)
	matrix := domain.Title{ID: "tmdb:movie:603", Source: domain.SourceTMDB, SourceID: "movie:603", Kind: domain.KindMovie,
		Name: "Matrix", Year: ip(1999), Genres: []string{}, IMDbID: sp("tt0133093")}
	unmatched := domain.Title{ID: "tmdb:movie:2", Source: domain.SourceTMDB, SourceID: "movie:2", Kind: domain.KindMovie,
		Name: "Eşleşmeyen", Genres: []string{}} // no imdb id
	noRating := domain.Title{ID: "tmdb:movie:3", Source: domain.SourceTMDB, SourceID: "movie:3", Kind: domain.KindMovie,
		Name: "Puansız", Genres: []string{}, IMDbID: sp("tt9999999")} // id known, not in dataset
	h.srv.Search.TMDB = &fakeSource{titles: map[string]domain.Title{"movie:603": matrix, "movie:2": unmatched, "movie:3": noRating}}
	st := h.srv.Store.(interface {
		ReplaceIMDbRatings(context.Context, func(func(domain.IMDbRating) error) error) error
	})
	st.ReplaceIMDbRatings(context.Background(), func(add func(domain.IMDbRating) error) error {
		return add(domain.IMDbRating{TConst: "tt0133093", Rating: 8.7, Votes: 2100345})
	})

	h.get("/icerik/tmdb:movie:603", 200, `cbadge-imdb`, `<span class="cbadge-val">8,7</span>`, "2,1 Mn oy", "Puan ver")
	for _, id := range []string{"tmdb:movie:2", "tmdb:movie:3"} {
		if body := h.get("/icerik/"+id, 200); strings.Contains(body, "IMDb ") || strings.Contains(body, `class="cbadge`) {
			t.Fatalf("%s: no community score expected", id)
		}
	}

	// anime: AniList average, no key needed
	f := frieren
	f.AniListScore = ip(89)
	h.anilist.titles["154587"] = f
	h.get("/icerik/anilist:154587", 200, "AniList 8,9")
	h.get("/?q=frieren", 200, `cbadge-anilist`, `<span class="cbadge-val">8,9</span>`)

	// the user's own score comes first
	h.post("/icerik/tmdb:movie:603/kayit", url.Values{"score": {"9"}}, http.StatusSeeOther)
	body := h.get("/icerik/tmdb:movie:603", 200)
	if mine, comm := strings.Index(body, `class="my-score"`), strings.Index(body, `class="community-score"`); mine < 0 || comm < mine {
		t.Fatal("own score must come before the community score")
	}
	h.get("/arsiv", 200, `title="IMDb 8,7"`)

	// without TMDB there is no IMDb match: nothing shown, quietly
	h.srv.Search.TMDB = nil
	h.srv.IMDb = nil
	if body := h.get("/arsiv", 200); strings.Contains(body, `title="IMDb 8,7"`) {
		t.Fatal("IMDb score without lookup")
	}
}

type limited struct{}

func (limited) Error() string     { return "429" }
func (limited) RateLimited() bool { return true }

func TestLiveSearchFragmentAndRateLimit(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequest("GET", "/?q=frieren", nil)
	req.Header.Set("X-Partial", "1")
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "<html") || strings.Contains(body, "searchbar") || !strings.Contains(body, "/icerik/anilist:154587") {
		t.Fatalf("fragment: %d %s", rec.Code, body)
	}
	h.get("/?q=frieren", 200, `id="sonuclar"`, "data-live-search")

	h.anilist.err = limited{}
	h.get("/?q=frieren", http.StatusBadGateway, "Biraz yavaşlayalım", "AniList kısa bir mola istedi")
}

func TestGridListToggleIsRemembered(t *testing.T) {
	h := newHarness(t)
	h.post("/icerik/anilist:154587/kayit", url.Values{"score": {"9"}}, http.StatusSeeOther)
	cat := h.post("/kategoriler", url.Values{"name": {"Liste"}, "color": {"teal"}}, http.StatusSeeOther).Header().Get("Location")
	h.post("/icerik/anilist:154587/kategoriler", url.Values{"kategori": {strings.TrimPrefix(cat, "/kategoriler/")}}, http.StatusSeeOther)

	h.get("/arsiv", 200, `<ul class="grid">`, `class="score-badge"`, `gorunum=liste`)
	h.get("/arsiv?durum=watched&gorunum=liste", 200) // switch keeps other params
	if h.cookies["gorunum"] != "liste" {
		t.Fatal("view not remembered")
	}
	body := h.get("/arsiv", 200, `<ul class="rows">`, `class="row-score"`)
	if strings.Contains(body, `<ul class="grid">`) {
		t.Fatal("still grid")
	}
	// category detail uses the same component and remembered view, with a remove button per item
	h.get(cat, 200, `<ul class="rows">`, `/cikar`, "?geri=")
	h.get("/arsiv?gorunum=izgara", 200, `<ul class="grid">`)
	h.get(cat, 200, `<ul class="grid">`, `/cikar`)
}

// postJSON sends a form like app.js does (Accept: application/json).
func (h *harness) postJSON(target string, form url.Values) (*httptest.ResponseRecorder, ratingJSON) {
	h.t.Helper()
	if _, explicit := form["csrf"]; !explicit {
		form.Set("csrf", h.cookies[csrfCookie])
	}
	req := httptest.NewRequest("POST", target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	for k, v := range h.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	var out ratingJSON
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestTapToSave(t *testing.T) {
	h := newHarness(t)
	u := "/icerik/anilist:154587/guncelle"
	field := func(alan, deger string, toggle bool) url.Values {
		v := url.Values{"alan": {alan}, "deger": {deger}}
		if toggle {
			v.Set("tersine", "1")
		}
		return v
	}

	rec, r := h.postJSON(u, field("score", "9", true))
	if rec.Code != 200 || r.Score == nil || *r.Score != 9 {
		t.Fatalf("score: %d %s", rec.Code, rec.Body)
	}
	_, r = h.postJSON(u, field("status", "planned", true))
	if r.Status == nil || *r.Status != "planned" || r.Score == nil {
		t.Fatalf("status must not touch the score: %+v", r)
	}
	h.get("/arsiv?durum=planned", 200, "Frieren")

	// tapping the selected value again clears it
	_, r = h.postJSON(u, field("score", "9", true))
	if r.Score != nil || r.Status == nil {
		t.Fatalf("second tap should clear only the score: %+v", r)
	}
	_, r = h.postJSON(u, field("status", "planned", true))
	if r.Status != nil {
		t.Fatalf("second tap should clear the status: %+v", r)
	}
	h.get("/arsiv", 200, "Arşivin boş") // nothing left: the record is gone

	// a different value replaces instead of clearing
	h.postJSON(u, field("score", "7", true))
	_, r = h.postJSON(u, field("score", "8", true))
	if r.Score == nil || *r.Score != 8 {
		t.Fatalf("%+v", r)
	}

	// note: trimmed, saved on its own
	_, r = h.postJSON(u, field("note", "  sakin ve güzel  ", false))
	if r.Note != "sakin ve güzel" || r.Score == nil {
		t.Fatalf("%+v", r)
	}

	// validation errors come back as JSON with the Turkish message
	rec, r = h.postJSON(u, field("score", "11", false))
	if rec.Code != http.StatusUnprocessableEntity || r.Error != "Puan 1 ile 10 arasında olmalı." {
		t.Fatalf("%d %+v", rec.Code, r)
	}
	rec, _ = h.postJSON(u, field("nope", "x", false))
	if rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code)
	}
	rec, r = h.postJSON("/icerik/anilist:999/guncelle", field("score", "5", false))
	if rec.Code != http.StatusNotFound || r.Error == "" {
		t.Fatal(rec.Code)
	}
}

func TestTapToSaveNeedsCSRF(t *testing.T) {
	h := newHarness(t)
	rec, _ := h.postJSON("/icerik/anilist:154587/guncelle", url.Values{"alan": {"score"}, "deger": {"9"}, "csrf": {""}})
	if rec.Code != http.StatusForbidden {
		t.Fatal("POST without CSRF token accepted", rec.Code)
	}
	rec, _ = h.postJSON("/icerik/anilist:154587/guncelle", url.Values{"alan": {"score"}, "deger": {"9"}, "csrf": {"forged-token-forged-token-forged-token"}})
	if rec.Code != http.StatusForbidden {
		t.Fatal("forged CSRF token accepted", rec.Code)
	}
	h.get("/arsiv", 200, "Arşivin boş")
}

func TestBookmarkWithoutJS(t *testing.T) {
	h := newHarness(t)
	body := h.get("/?q=frieren", 200, `class="bookmark"`, `aria-pressed="false"`, `name="tersine" value="1"`, `<span class="sr">İzleyeceğim</span>`)
	if !strings.Contains(body, `action="/icerik/anilist:154587/guncelle"`) {
		t.Fatal("no bookmark form on search rows")
	}
	// plain form post (no JS): toggles and goes back to the search
	rec := h.post("/icerik/anilist:154587/guncelle", url.Values{"alan": {"status"}, "deger": {"planned"}, "tersine": {"1"}, "geri": {"/?q=frieren"}}, http.StatusSeeOther)
	if rec.Header().Get("Location") != "/?q=frieren" {
		t.Fatal(rec.Header().Get("Location"))
	}
	h.get("/?q=frieren", 200, `aria-pressed="true"`)
	h.get("/icerik/anilist:154587", 200, `data-autosave=`, `<noscript><button type="submit" class="button">Kaydet</button></noscript>`, `class="bookmark with-text"`, `aria-pressed="true"`)
	h.post("/icerik/anilist:154587/guncelle", url.Values{"alan": {"status"}, "deger": {"planned"}, "tersine": {"1"}, "geri": {"//evil.example"}}, http.StatusSeeOther)
	h.get("/?q=frieren", 200, `aria-pressed="false"`)
}
