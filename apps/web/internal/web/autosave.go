package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"filmarsivi/web/internal/domain"
)

// ratingJSON is what the autosave endpoint answers to fetch requests.
type ratingJSON struct {
	Score  *int    `json:"score"`
	Status *string `json:"status"`
	Note   string  `json:"note"`
	Error  string  `json:"error,omitempty"`
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// updateField saves one field of the user's record right away (tap to save).
//
//	alan    score | status | note
//	deger   the new value ("" clears it)
//	tersine 1: if the field already has this value, clear it instead (tap again to undo)
//	geri    where to go afterwards for plain (no-JS) form posts
//
// fetch requests (Accept: application/json) get the saved record as JSON; plain
// form posts are redirected, so the bookmark buttons work without JavaScript.
func (s *Server) updateField(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	asJSON := wantsJSON(r)
	fail := func(status int, msg string) {
		if asJSON {
			writeJSON(w, status, ratingJSON{Error: msg})
			return
		}
		s.renderTitle(w, r, status, &ratingForm{Error: msg})
	}
	field, val, toggle := r.PostForm.Get("alan"), strings.TrimSpace(r.PostForm.Get("deger")), r.PostForm.Get("tersine") == "1"

	t, _, err := s.loadTitle(r.Context(), id)
	if errors.Is(err, errTitleNotFound) {
		fail(http.StatusNotFound, s.T.Errors["title_not_found"])
		return
	}
	if err != nil {
		fail(http.StatusBadGateway, s.T.Detail.LoadError)
		return
	}
	current, err := s.Ratings.Get(r.Context(), s.uid(r), id)
	if err != nil {
		s.Log.Error("autosave", "err", err)
		fail(http.StatusInternalServerError, s.T.Web.SaveFailed)
		return
	}

	var patch domain.RatingPatch
	switch field {
	case "score":
		score, err := domain.ParseScore(val)
		if err != nil {
			fail(http.StatusUnprocessableEntity, s.errorText(err))
			return
		}
		if toggle && score != nil && current != nil && current.Score != nil && *current.Score == *score {
			score = nil
		}
		patch.SetScore, patch.Score = true, score
	case "status":
		var st *domain.WatchStatus
		if v, ok := domain.ParseWatchStatus(val); ok {
			st = &v
		}
		if toggle && st != nil && current != nil && current.Status != nil && *current.Status == *st {
			st = nil
		}
		patch.SetStatus, patch.Status = true, st
	case "note":
		patch.SetNote, patch.Note = true, r.PostForm.Get("deger")
	default:
		fail(http.StatusBadRequest, s.T.Errors["generic"])
		return
	}

	saved, err := s.Ratings.Update(r.Context(), s.uid(r), t, patch)
	if err != nil {
		if _, ok := domain.CodeOf(err); ok {
			fail(http.StatusUnprocessableEntity, s.errorText(err))
			return
		}
		s.Log.Error("autosave", "err", err)
		fail(http.StatusInternalServerError, s.T.Web.SaveFailed)
		return
	}
	if !asJSON {
		http.Redirect(w, r, safeBack(r.PostForm.Get("geri"), titleURL(id)+"?kaydedildi=puan#puan"), http.StatusSeeOther)
		return
	}
	out := ratingJSON{}
	if saved != nil {
		out.Score, out.Note = saved.Score, saved.Note
		if saved.Status != nil {
			st := string(*saved.Status)
			out.Status = &st
		}
	}
	writeJSON(w, http.StatusOK, out)
}
