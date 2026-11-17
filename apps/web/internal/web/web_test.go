package web

import (
	"bytes"
	"context"
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
}

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
	return &harness{t: t, srv: srv, h: srv.Handler(), anilist: anilist}
}

func (h *harness) do(method, target string, form url.Values) *httptest.ResponseRecorder {
	h.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.com")
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
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
	h.get("/?q=frieren", 200, `class="score-badge"`)

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
