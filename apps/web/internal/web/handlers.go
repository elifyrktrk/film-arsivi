package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"filmarsivi/web/internal/domain"
)

// ---------- search ----------

type searchResultView struct {
	Title domain.Title
	Score *int
}

type searchData struct {
	Query   string
	Short   bool // fewer than 2 characters
	Failed  bool
	Results []searchResultView
	Sources domain.SourceStates
}

func (s *Server) searchPage(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	d := searchData{Query: q}
	title := s.T.Tabs.Search
	status := http.StatusOK
	if len([]rune(q)) < 2 {
		d.Short = true
	} else {
		title = q + " · " + s.T.Tabs.Search
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		res, err := s.Search.Search(ctx, q)
		d.Sources = res.Sources
		if err != nil {
			s.Log.Warn("search failed", "err", err)
			d.Failed = true
			status = http.StatusBadGateway
		}
		for _, t := range res.Results {
			v := searchResultView{Title: t}
			if rt, _ := s.Ratings.Get(r.Context(), LocalUser, t.ID); rt != nil {
				v.Score = rt.Score
			}
			d.Results = append(d.Results, v)
		}
	}
	s.render(w, r, status, "search", s.page(r, "search", title, d))
}

// ---------- archive ----------

type archiveData struct {
	Entries     []domain.ArchiveEntry
	Status      string
	Sort        string
	Total       int // entries regardless of filter, to tell "empty" from "empty filter"
	StatusLinks []filterLink
	SortLinks   []filterLink
}

type filterLink struct {
	Label, URL string
	Active     bool
}

func archiveURL(status, sort string) string {
	v := url.Values{}
	if status != "" {
		v.Set("durum", status)
	}
	if sort != "" && sort != string(domain.SortRecent) {
		v.Set("sirala", sort)
	}
	if len(v) == 0 {
		return "/arsiv"
	}
	return "/arsiv?" + v.Encode()
}

func (s *Server) archivePage(w http.ResponseWriter, r *http.Request) {
	q := domain.ArchiveQuery{Sort: domain.SortRecent}
	statusParam := r.URL.Query().Get("durum")
	if st, ok := domain.ParseWatchStatus(statusParam); ok {
		q.Status = st
	} else {
		statusParam = ""
	}
	switch sp := domain.ArchiveSort(r.URL.Query().Get("sirala")); sp {
	case domain.SortScore, domain.SortName:
		q.Sort = sp
	}
	entries, err := s.Store.ListArchive(r.Context(), LocalUser, q)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	total := len(entries)
	if q.Status != "" {
		all, err := s.Store.ListArchive(r.Context(), LocalUser, domain.ArchiveQuery{})
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		total = len(all)
	}
	d := archiveData{Entries: entries, Status: statusParam, Sort: string(q.Sort), Total: total}
	d.StatusLinks = append(d.StatusLinks, filterLink{s.T.Archive.All, archiveURL("", d.Sort), statusParam == ""})
	for _, st := range domain.WatchStatuses {
		d.StatusLinks = append(d.StatusLinks, filterLink{s.T.Status[string(st)], archiveURL(string(st), d.Sort), statusParam == string(st)})
	}
	for _, so := range []struct {
		v     domain.ArchiveSort
		label string
	}{{domain.SortRecent, s.T.Archive.SortRecent}, {domain.SortScore, s.T.Archive.SortScore}, {domain.SortName, s.T.Archive.SortName}} {
		d.SortLinks = append(d.SortLinks, filterLink{so.label, archiveURL(statusParam, string(so.v)), q.Sort == so.v})
	}
	s.render(w, r, http.StatusOK, "archive", s.page(r, "archive", s.T.Archive.Title, d))
}

// ---------- title detail ----------

type detailData struct {
	Title        domain.Title
	Stale        bool
	Rating       *domain.Rating
	Form         ratingForm
	Categories   []domain.CategoryWithCount
	InCategories []string
	Saved        string // "rating" | "categories"
	Back         string
}

type ratingForm struct {
	Score  *int
	Status string
	Note   string
	Error  string
}

func (s *Server) titlePage(w http.ResponseWriter, r *http.Request) {
	s.renderTitle(w, r, http.StatusOK, nil)
}

// renderTitle shows the detail page; form is non-nil when re-showing rejected input.
func (s *Server) renderTitle(w http.ResponseWriter, r *http.Request, status int, form *ratingForm) {
	id := r.PathValue("id")
	t, stale, err := s.loadTitle(r.Context(), id)
	if errors.Is(err, errTitleNotFound) {
		s.message(w, r, http.StatusNotFound, messageData{Heading: s.T.Errors["title_not_found"], LinkURL: "/", LinkText: s.T.Archive.GoSearch})
		return
	}
	if err != nil {
		s.message(w, r, http.StatusBadGateway, messageData{Heading: s.T.Detail.LoadError, Body: s.T.Search.Error, LinkURL: r.URL.Path, LinkText: s.T.Search.Retry})
		return
	}
	ctx := r.Context()
	rating, err := s.Ratings.Get(ctx, LocalUser, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	cats, err := s.Categories.List(ctx, LocalUser)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	in, err := s.Categories.CategoryIDsForTitle(ctx, LocalUser, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := detailData{Title: t, Stale: stale, Rating: rating, Categories: cats, InCategories: in,
		Saved: r.URL.Query().Get("kaydedildi"), Back: safeBack(r.URL.Query().Get("geri"), "")}
	if form != nil {
		d.Form = *form
	} else if rating != nil {
		d.Form = ratingForm{Score: rating.Score, Note: rating.Note}
		if rating.Status != nil {
			d.Form.Status = string(*rating.Status)
		}
	}
	s.render(w, r, status, "title", s.page(r, "", t.Name, d))
}

// saveRating saves score, status and note in one go (one form, works without JS).
func (s *Server) saveRating(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	form := ratingForm{Status: r.PostForm.Get("status"), Note: r.PostForm.Get("note")}
	patch := domain.RatingPatch{SetScore: true, SetNote: true, Note: form.Note, SetStatus: true}
	score, err := domain.ParseScore(r.PostForm.Get("score"))
	if err != nil {
		form.Error = s.errorText(err)
		s.renderTitle(w, r, http.StatusUnprocessableEntity, &form)
		return
	}
	patch.Score, form.Score = score, score
	if st, ok := domain.ParseWatchStatus(form.Status); ok {
		patch.Status = &st
	}
	t, _, err := s.loadTitle(r.Context(), id)
	if err != nil {
		s.renderTitle(w, r, http.StatusBadGateway, &form)
		return
	}
	if _, err := s.Ratings.Update(r.Context(), LocalUser, t, patch); err != nil {
		if _, ok := domain.CodeOf(err); ok {
			form.Error = s.errorText(err)
			s.renderTitle(w, r, http.StatusUnprocessableEntity, &form)
			return
		}
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, titleURL(id)+"?kaydedildi=puan#puan", http.StatusSeeOther)
}

func (s *Server) saveTitleCategories(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t, _, err := s.loadTitle(r.Context(), id)
	if err != nil {
		s.renderTitle(w, r, http.StatusBadGateway, nil)
		return
	}
	if err := s.Categories.SetCategoriesForTitle(r.Context(), LocalUser, t, r.PostForm["kategori"]); err != nil {
		if _, ok := domain.CodeOf(err); !ok {
			s.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, titleURL(id)+"?kaydedildi=kategoriler#kategoriler", http.StatusSeeOther)
}

// ---------- categories ----------

func (s *Server) categoriesPage(w http.ResponseWriter, r *http.Request) {
	cats, err := s.Categories.List(r.Context(), LocalUser)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	p := s.page(r, "categories", s.T.Categories.Title, cats)
	if r.URL.Query().Get("silindi") != "" {
		p.Flash = s.T.Web.Saved
	}
	s.render(w, r, http.StatusOK, "categories", p)
}

type categoryFormData struct {
	Category *domain.Category // nil when creating
	Input    domain.CategoryInput
	Error    string
	AddTitle *domain.Title // create-and-add flow from a title page
}

func (s *Server) newCategoryPage(w http.ResponseWriter, r *http.Request) {
	d := categoryFormData{Input: domain.CategoryInput{Color: domain.CategoryColors[0]}}
	if id := r.URL.Query().Get("icerik"); id != "" {
		if t, _, err := s.loadTitle(r.Context(), id); err == nil {
			d.AddTitle = &t
		}
	}
	s.render(w, r, http.StatusOK, "category_form", s.page(r, "categories", s.T.Categories.New, d))
}

func categoryInputFrom(r *http.Request) domain.CategoryInput {
	return domain.CategoryInput{
		Name:        r.PostForm.Get("name"),
		Color:       domain.CategoryColor(r.PostForm.Get("color")),
		Description: r.PostForm.Get("description"),
	}
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	in := categoryInputFrom(r)
	var addTitle *domain.Title
	if id := r.PostForm.Get("icerik"); id != "" {
		if t, _, err := s.loadTitle(r.Context(), id); err == nil {
			addTitle = &t
		}
	}
	c, err := s.Categories.Create(r.Context(), LocalUser, in)
	if err != nil {
		if _, ok := domain.CodeOf(err); !ok {
			s.serverError(w, r, err)
			return
		}
		d := categoryFormData{Input: in, Error: s.errorText(err), AddTitle: addTitle}
		s.render(w, r, http.StatusUnprocessableEntity, "category_form", s.page(r, "categories", s.T.Categories.New, d))
		return
	}
	if addTitle != nil {
		if err := s.Categories.AddTitle(r.Context(), LocalUser, c.ID, *addTitle); err != nil {
			s.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, titleURL(addTitle.ID)+"?kaydedildi=kategoriler#kategoriler", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, categoryURL(c.ID), http.StatusSeeOther)
}

// requireCategory loads the category in the path or answers 404.
func (s *Server) requireCategory(w http.ResponseWriter, r *http.Request) *domain.Category {
	c, err := s.Categories.Get(r.Context(), LocalUser, r.PathValue("id"))
	if err != nil {
		s.serverError(w, r, err)
		return nil
	}
	if c == nil {
		s.message(w, r, http.StatusNotFound, messageData{Heading: s.T.Errors["category_not_found"], LinkURL: "/kategoriler", LinkText: s.T.Categories.Title})
		return nil
	}
	return c
}

type categoryData struct {
	Category domain.Category
	Items    []domain.CategoryItemWithTitle
	Scores   map[string]*int
}

func (s *Server) categoryPage(w http.ResponseWriter, r *http.Request) {
	c := s.requireCategory(w, r)
	if c == nil {
		return
	}
	items, err := s.Categories.ListItems(r.Context(), LocalUser, c.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := categoryData{Category: *c, Items: items, Scores: map[string]*int{}}
	for _, it := range items {
		if rt, _ := s.Ratings.Get(r.Context(), LocalUser, it.TitleID); rt != nil {
			d.Scores[it.TitleID] = rt.Score
		}
	}
	p := s.page(r, "categories", c.Name, d)
	if r.URL.Query().Get("kaydedildi") != "" {
		p.Flash = s.T.Web.Saved
	}
	s.render(w, r, http.StatusOK, "category", p)
}

func (s *Server) editCategoryPage(w http.ResponseWriter, r *http.Request) {
	c := s.requireCategory(w, r)
	if c == nil {
		return
	}
	d := categoryFormData{Category: c, Input: domain.CategoryInput{Name: c.Name, Color: c.Color, Description: c.Description}}
	s.render(w, r, http.StatusOK, "category_form", s.page(r, "categories", s.T.Categories.Edit, d))
}

func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	c := s.requireCategory(w, r)
	if c == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	in := categoryInputFrom(r)
	if _, err := s.Categories.Update(r.Context(), LocalUser, c.ID, in); err != nil {
		if _, ok := domain.CodeOf(err); !ok {
			s.serverError(w, r, err)
			return
		}
		d := categoryFormData{Category: c, Input: in, Error: s.errorText(err)}
		s.render(w, r, http.StatusUnprocessableEntity, "category_form", s.page(r, "categories", s.T.Categories.Edit, d))
		return
	}
	http.Redirect(w, r, categoryURL(c.ID)+"?kaydedildi=1", http.StatusSeeOther)
}

func (s *Server) deleteCategoryPage(w http.ResponseWriter, r *http.Request) {
	c := s.requireCategory(w, r)
	if c == nil {
		return
	}
	s.render(w, r, http.StatusOK, "category_delete", s.page(r, "categories", s.T.Web.DeleteTitle, c))
}

func (s *Server) deleteCategory(w http.ResponseWriter, r *http.Request) {
	c := s.requireCategory(w, r)
	if c == nil {
		return
	}
	if err := s.Categories.Remove(r.Context(), LocalUser, c.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/kategoriler?silindi=1", http.StatusSeeOther)
}

func (s *Server) removeFromCategory(w http.ResponseWriter, r *http.Request) {
	c := s.requireCategory(w, r)
	if c == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Categories.RemoveTitle(r.Context(), LocalUser, c.ID, r.PostForm.Get("icerik")); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, categoryURL(c.ID), http.StatusSeeOther)
}

// ---------- about & theme ----------

func (s *Server) aboutPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "about", s.page(r, "about", s.T.About.Title, nil))
}

func (s *Server) setTheme(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	v := r.PostForm.Get("tema")
	c := &http.Cookie{Name: "tema", Value: v, Path: "/", MaxAge: 365 * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteLaxMode}
	if v != "light" && v != "dark" {
		c.Value, c.MaxAge = "", -1
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, safeBack(r.PostForm.Get("geri"), "/"), http.StatusSeeOther)
}
