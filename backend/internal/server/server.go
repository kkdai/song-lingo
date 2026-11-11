// Package server is the HTTP API behind the Next.js front end, which forwards /api/* here.
//
// It listens on localhost only: access control (the IAP check in web/proxy.ts) runs in Next.js
// before a request is forwarded.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/kkdai/song-lingo/backend/internal/config"
	"github.com/kkdai/song-lingo/backend/internal/pipeline"
	"github.com/kkdai/song-lingo/backend/internal/progress"
	"github.com/kkdai/song-lingo/backend/internal/review"
	"github.com/kkdai/song-lingo/backend/internal/shadow"
	"github.com/kkdai/song-lingo/backend/internal/songs"
	"github.com/kkdai/song-lingo/backend/internal/tts"
)

type Server struct {
	cfg      *config.Config
	songs    songs.Store
	progress progress.Store
	review   *review.Service
	pipeline *pipeline.Runner
	tts      *tts.Service
	shadow   transcriber

	shadowGate shadowGate
}

// transcriber writes down what the learner said in a recording (shadow.Transcriber).
type transcriber interface {
	Transcribe(ctx context.Context, audio []byte, mimeType, language string) (string, error)
}

func New(cfg *config.Config, progressStore progress.Store) *Server {
	store := songs.Store{Dir: cfg.DataDir}
	geminiClient := pipeline.LazyClient(cfg.APIKey)
	rest := &tts.API{Base: tts.DefaultAPIBase, APIKey: cfg.APIKey}
	return &Server{
		cfg:      cfg,
		songs:    store,
		progress: progressStore,
		review:   &review.Service{Songs: store, Progress: progressStore},
		pipeline: pipeline.NewRunner(store, pipeline.DefaultModel, geminiClient),
		tts:      tts.NewService(cfg, store, rest),
		shadow:   &shadow.Transcriber{Client: geminiClient, API: rest},
	}
}

// NewProgressStore uses Firestore when FIRESTORE_DATABASE is set (Cloud Run), else JSON files
// under <data dir>/progress.
func NewProgressStore(ctx context.Context, cfg *config.Config) (progress.Store, error) {
	if db := os.Getenv("FIRESTORE_DATABASE"); db != "" {
		return progress.NewFirestoreStore(ctx, db)
	}
	return progress.NewFileStore(filepath.Join(cfg.DataDir, "progress")), nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/songs", s.listSongs)
	mux.HandleFunc("POST /api/songs", s.addSong)
	mux.HandleFunc("GET /api/songs/{id}", s.getSong)
	mux.HandleFunc("GET /api/songs/{id}/status", s.songStatus)
	mux.HandleFunc("POST /api/songs/{id}/reannotate", s.reannotate)
	mux.HandleFunc("PATCH /api/songs/{id}/progress", s.patchProgress)
	mux.HandleFunc("GET /api/audio/{id}/{file}", s.audio)
	mux.HandleFunc("PATCH /api/songs/{id}/lines/{index}", s.patchLine)
	mux.HandleFunc("POST /api/songs/{id}/lines/{index}/shadow", s.shadowLine)
	return logErrors(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

// internalError logs the cause and answers 500 without exposing it.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("[api] %s %s: %v", r.Method, r.URL.Path, err)
	fail(w, http.StatusInternalServerError, "伺服器發生錯誤。")
}

func logErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Printf("[api] panic in %s %s: %v", r.Method, r.URL.Path, v)
				fail(w, http.StatusInternalServerError, "伺服器發生錯誤。")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type songSummary struct {
	ID          string           `json:"id"`
	Language    string           `json:"language"`
	TitleGuess  *string          `json:"title_guess"`
	ArtistGuess *string          `json:"artist_guess"`
	LineCount   int              `json:"lineCount"`
	HasTeacher  bool             `json:"hasTeacher"`
	Progress    progress.Summary `json:"progress"`
}

func (s *Server) listSongs(w http.ResponseWriter, r *http.Request) {
	ids, err := s.songs.IDs()
	if err != nil {
		internalError(w, r, err)
		return
	}
	summaries := []songSummary{}
	for _, id := range ids {
		song, err := s.songs.Read(id)
		if err != nil {
			continue // a half-written or broken file shouldn't hide the other songs
		}
		p, err := s.progress.Read(r.Context(), id)
		if err != nil {
			internalError(w, r, err)
			return
		}
		summaries = append(summaries, songSummary{
			ID:          id,
			Language:    song.Language,
			TitleGuess:  song.TitleGuess,
			ArtistGuess: song.ArtistGuess,
			LineCount:   len(song.Lines),
			HasTeacher:  s.cfg.HasTeacher(song.Language),
			Progress:    progress.Summarize(p, song.Lines),
		})
	}
	// Most recently studied first; songs never opened keep their order at the end.
	studied := func(i int) string {
		if at := summaries[i].Progress.LastStudiedAt; at != nil {
			return *at
		}
		return ""
	}
	sort.SliceStable(summaries, func(i, j int) bool { return studied(i) > studied(j) })
	writeJSON(w, http.StatusOK, summaries)
}

// getSong returns a song for the study page, with teacher clip URLs and the learner's progress.
func (s *Server) getSong(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	song, err := s.songs.Read(id)
	if errors.Is(err, songs.ErrNotFound) {
		fail(w, http.StatusNotFound, "找不到這首歌。")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	p, err := s.progress.Read(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}

	hasTeacher := s.cfg.HasTeacher(song.Language)
	cached := map[string]bool{}
	if hasTeacher {
		cached = s.songs.CachedClips(id)
	}
	for i := range song.Lines {
		line := &song.Lines[i]
		line.ProgressKey = progress.LineKey(line.Text)
		if !hasTeacher {
			continue
		}
		line.Audio = map[string]string{}
		line.AudioCached = []string{}
		for _, speed := range s.cfg.Speeds {
			name := songs.ClipName(line.Text, speed)
			line.Audio[speed] = "/api/audio/" + id + "/" + name
			if cached[name] {
				line.AudioCached = append(line.AudioCached, speed)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"song": song, "progress": p})
}
