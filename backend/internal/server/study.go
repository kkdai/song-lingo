package server

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/kkdai/song-lingo/backend/internal/progress"
	"github.com/kkdai/song-lingo/backend/internal/review"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// lineIndex parses the {index} path segment; only plain digits are accepted.
func lineIndex(r *http.Request) (int, bool) {
	raw := r.PathValue("index")
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	i, err := strconv.Atoi(raw)
	return i, err == nil
}

// asIndex accepts a JSON number that is a whole number in [0, n).
func asIndex(raw json.RawMessage, n int) (int, bool) {
	var f float64
	if review.IsNull(raw) || json.Unmarshal(raw, &f) != nil || f != math.Trunc(f) || f < 0 || f >= float64(n) {
		return 0, false
	}
	return int(f), true
}

// patchProgress saves study progress: `lastLine` (where to resume) and/or
// `learned: { index, value }`. Learned status is stored by the line's text, so every repeat of a
// chorus line follows.
func (s *Server) patchProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !songs.IsVideoID(id) {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	song, err := s.songs.Read(id)
	if errors.Is(err, songs.ErrNotFound) {
		fail(w, http.StatusNotFound, "找不到這首歌。")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}

	body, ok := review.DecodeObject(r.Body)
	if !ok {
		fail(w, http.StatusBadRequest, "請求格式不正確。")
		return
	}
	rawLastLine, hasLastLine := body["lastLine"]
	lastLine := 0
	if hasLastLine {
		if lastLine, ok = asIndex(rawLastLine, len(song.Lines)); !ok {
			fail(w, http.StatusBadRequest, "lastLine 格式不正確。")
			return
		}
	}
	var learned *struct {
		index int
		value bool
	}
	if rawLearned, present := body["learned"]; present {
		var raw struct {
			Index json.RawMessage `json:"index"`
			Value *bool           `json:"value"`
		}
		_ = json.Unmarshal(rawLearned, &raw)
		index, ok := asIndex(raw.Index, len(song.Lines))
		if !ok || raw.Value == nil {
			fail(w, http.StatusBadRequest, "learned 格式不正確。")
			return
		}
		learned = &struct {
			index int
			value bool
		}{index, *raw.Value}
	}

	_, err = s.progress.Update(r.Context(), id, func(p *progress.Progress) {
		if hasLastLine {
			p.LastLine = lastLine
		}
		if learned != nil {
			p.SetLearned(song.Lines[learned.index].Text, learned.value)
		}
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// patchLine applies a review edit (the ✏️ form on the study page).
func (s *Server) patchLine(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	index, ok := lineIndex(r)
	if !songs.IsVideoID(id) || !ok {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	edit, err := review.ParseEdit(r.Body)
	if err == nil {
		var updated int
		updated, err = s.review.EditLine(r.Context(), id, index, edit)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]int{"updated": updated})
			return
		}
	}
	var re *review.Error
	if errors.As(err, &re) {
		fail(w, re.Status, re.Message)
		return
	}
	internalError(w, r, err)
}
