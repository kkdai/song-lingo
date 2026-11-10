package server

import (
	"bytes"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/songs"
	"github.com/kkdai/song-lingo/backend/internal/tts"
)

// The last generation failure per clip, so the page can explain an <audio> error (media
// elements never expose the response body) without triggering another TTS request.
type recentErrors struct {
	mu     sync.Mutex
	errors map[string]recentError
}

type recentError struct {
	status  int
	message string
	at      time.Time
}

const recentErrorTTL = 60 * time.Second

func (r *recentErrors) set(key string, e *recentError) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.errors == nil {
		r.errors = map[string]recentError{}
	}
	if e == nil {
		delete(r.errors, key)
	} else {
		r.errors[key] = *e
	}
}

func (r *recentErrors) get(key string) (recentError, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.errors[key]
	return e, ok && time.Since(e.at) < recentErrorTTL
}

// audio serves a teacher clip, generating it on first request. `?error` instead reports why the
// last attempt failed.
func (s *Server) audio(w http.ResponseWriter, r *http.Request) {
	id, file := r.PathValue("id"), r.PathValue("file")
	if !songs.IsVideoID(id) || !s.tts.IsClipName(file) {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	key := id + "/" + file

	if r.URL.Query().Has("error") {
		if e, ok := s.audioErrors.get(key); ok {
			fail(w, e.status, e.message)
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
		s.audioErrors.set(key, &recentError{status: e.Status, message: e.Message, at: time.Now()})
		fail(w, e.Status, e.Message)
		return
	}
	s.audioErrors.set(key, nil)

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	// ServeContent answers byte-range requests, which Safari needs to play media.
	http.ServeContent(w, r, file, time.Time{}, bytes.NewReader(data))
}
