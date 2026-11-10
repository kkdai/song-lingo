package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kkdai/song-lingo/backend/internal/pipeline"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

func pipelineError(w http.ResponseWriter, r *http.Request, err error) {
	var pe *pipeline.Error
	if errors.As(err, &pe) {
		fail(w, pe.Status, pe.Message)
		return
	}
	internalError(w, r, err)
}

// addSong starts adding a song from a YouTube URL; poll /api/songs/{id}/status for progress.
func (s *Server) addSong(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL *string `json:"url"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.URL == nil {
		fail(w, http.StatusBadRequest, "請提供 YouTube 網址。")
		return
	}
	job, err := s.pipeline.AddSong(*body.URL)
	if err != nil {
		pipelineError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) songStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var job *pipeline.JobStatus
	if songs.IsVideoID(id) {
		job = s.pipeline.Status(id)
	}
	if job == nil {
		fail(w, http.StatusNotFound, "沒有這首歌的處理紀錄。")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) reannotate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !songs.IsVideoID(id) {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	summary, err := s.pipeline.Reannotate(id)
	if err != nil {
		pipelineError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"summary": summary})
}
