package tts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/config"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// A failed generation may still have been billed, so repeated clicks or the browser's parallel
// range requests don't retry it: the error is replayed for this long instead.
const failureTTL = 60 * time.Second

// Service returns teacher clips, generating and caching each on first request.
type Service struct {
	Cfg   *config.Config
	Songs songs.Store
	API   *API

	clipFile *regexp.Regexp

	mu        sync.Mutex
	voiceLock sync.Mutex
	pending   map[string]*call
	failures  map[string]failure
	quota     *Error
}

type call struct {
	done  chan struct{}
	audio []byte
	err   error
}

type failure struct {
	err   *Error
	until time.Time
}

func NewService(cfg *config.Config, store songs.Store, api *API) *Service {
	return &Service{
		Cfg:      cfg,
		Songs:    store,
		API:      api,
		clipFile: ClipPattern(cfg.Speeds),
		pending:  map[string]*call{},
		failures: map[string]failure{},
	}
}

// ClipPattern matches clip file names: <16 hex chars of the text hash>_<speed>.wav.
func ClipPattern(speeds []string) *regexp.Regexp {
	quoted := make([]string, len(speeds))
	for i, s := range speeds {
		quoted[i] = regexp.QuoteMeta(s)
	}
	return regexp.MustCompile(`^([0-9a-f]{16})_(` + strings.Join(quoted, "|") + `)\.wav$`)
}

func (s *Service) IsClipName(file string) bool { return s.clipFile.MatchString(file) }

func (s *Service) voicesFile() string { return filepath.Join(s.Cfg.DataDir, "voices.json") }

func (s *Service) readVoices() (map[string]string, error) {
	voices := map[string]string{}
	raw, err := os.ReadFile(s.voicesFile())
	if errors.Is(err, fs.ErrNotExist) {
		return voices, nil
	}
	if err != nil {
		return nil, err
	}
	return voices, json.Unmarshal(raw, &voices)
}

// TeacherVoice returns the language's teacher voice, designing it on first use (cached in
// voices.json). Only one voice is designed at a time.
func (s *Service) TeacherVoice(ctx context.Context, language string) (string, error) {
	s.voiceLock.Lock()
	defer s.voiceLock.Unlock()
	voices, err := s.readVoices()
	if err != nil {
		return "", err
	}
	if id := voices[language]; id != "" {
		return id, nil
	}
	teacher, ok := s.Cfg.Teachers[language]
	if !ok {
		return "", &Error{Status: 400, Message: fmt.Sprintf("還沒有「%s」的老師聲音設定。", language)}
	}
	created, err := s.API.Call(ctx, "POST", "voices", map[string]any{
		"voice": map[string]any{
			"type":          "prompted",
			"display_name":  teacher.DisplayName,
			"gender":        "female",
			"language_code": teacher.LanguageCode,
			"model":         s.Cfg.TTSModel,
			"prompted":      map[string]any{"input": teacher.Description},
		},
		"store": true,
	})
	if err != nil {
		return "", err
	}
	id, _ := created["id"].(string)
	if id == "" {
		return "", &Error{Status: 502, Message: "Gemini 沒有回傳聲音 ID。"}
	}
	voices[language] = id
	if err := os.MkdirAll(s.Cfg.DataDir, 0o755); err != nil {
		return "", err
	}
	if err := songs.WriteAtomic(s.voicesFile(), songs.MarshalIndent(voices)); err != nil {
		return "", err
	}
	return id, nil
}

// Clip returns a teacher clip, generating and caching it on first request.
func (s *Service) Clip(ctx context.Context, videoID, file string) ([]byte, error) {
	m := s.clipFile.FindStringSubmatch(file)
	if m == nil || !songs.IsVideoID(videoID) {
		return nil, &Error{Status: 404, Message: "Not found"}
	}
	path := filepath.Join(s.Songs.AudioDir(videoID), file)
	if audio, err := os.ReadFile(path); err == nil {
		return audio, nil
	}

	s.mu.Lock()
	if s.quota != nil && time.Now().Before(s.quota.RetryAt) {
		err := s.quota
		s.mu.Unlock()
		return nil, err
	}
	if f, ok := s.failures[path]; ok && time.Now().Before(f.until) {
		s.mu.Unlock()
		return nil, f.err
	}
	// Safari fires several range requests at once; they share one generation.
	c := s.pending[path]
	if c == nil {
		c = &call{done: make(chan struct{})}
		s.pending[path] = c
		go s.generate(c, videoID, path, m[1], m[2])
	}
	s.mu.Unlock()

	select {
	case <-c.done:
		return c.audio, c.err
	case <-ctx.Done():
		// The generation keeps going and is cached for the next request.
		return nil, ctx.Err()
	}
}

// generate runs detached from the request, so a closed tab doesn't waste a paid generation.
func (s *Service) generate(c *call, videoID, path, hash, speed string) {
	ctx := context.Background()
	c.audio, c.err = s.synthesizeClip(ctx, videoID, path, hash, speed)

	s.mu.Lock()
	var e *Error
	if errors.As(c.err, &e) {
		if e.Status == 429 {
			s.quota = e
		} else {
			s.failures[path] = failure{err: e, until: time.Now().Add(failureTTL)}
		}
	}
	delete(s.pending, path)
	s.mu.Unlock()
	close(c.done)
}

func (s *Service) synthesizeClip(ctx context.Context, videoID, path, hash, speed string) ([]byte, error) {
	song, err := s.Songs.Read(videoID)
	if err != nil {
		return nil, &Error{Status: 404, Message: "Not found"}
	}
	var text string
	for _, l := range song.Lines {
		if songs.TextHash(l.Text) == hash {
			text = l.Text
			break
		}
	}
	if text == "" {
		return nil, &Error{Status: 404, Message: "Not found"}
	}
	voice, err := s.TeacherVoice(ctx, song.Language)
	if err != nil {
		return nil, ttsMessage(err)
	}
	audio, err := Synthesize(ctx, s.API, s.Cfg.TTSModel, voice, text, s.Cfg.Styles[speed])
	if err != nil {
		return nil, ttsMessage(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := songs.WriteAtomic(path, audio); err != nil {
		return nil, err
	}
	return audio, nil
}

// ttsMessage names the TTS quota in quota errors, as the study page shows them as is.
func ttsMessage(err error) error {
	var e *Error
	if errors.As(err, &e) && e.Status == 429 {
		copied := *e
		copied.Message = "TTS " + e.Message
		return &copied
	}
	return err
}
