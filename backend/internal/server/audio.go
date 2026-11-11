package server

import (
	"bytes"
	"errors"
	"net/http"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/songs"
	"github.com/kkdai/song-lingo/backend/internal/tts"
)

// audio serves a teacher clip, generating it on first request. `?error` instead reports why the
// last attempt failed.
func (s *Server) audio(w http.ResponseWriter, r *http.Request) {
	id, file := r.PathValue("id"), r.PathValue("file")
	if !songs.IsVideoID(id) || !s.tts.IsClipName(file) {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	if r.URL.Query().Has("error") {
		if e, ok := s.tts.LastError(id, file); ok {
			fail(w, e.Status, e.Message)
		} else {
			writeJSON(w, http.StatusOK, map[string]any{"error": nil})
		}
		return
	}

	data, err := s.tts.Clip(r.Context(), id, file)
	if err != nil {
		var e *tts.Error
		if !errors.As(err, &e) {
			if r.Context().Err() != nil {
				return // the browser gave up; the clip is still generated and cached
			}
			internalError(w, r, err)
			return
		}
		fail(w, e.Status, e.Message)
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	// ServeContent answers byte-range requests, which Safari needs to play media.
	http.ServeContent(w, r, file, time.Time{}, bytes.NewReader(data))
}
