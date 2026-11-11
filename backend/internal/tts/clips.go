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

// MaxConcurrentGenerations bounds the paid TTS calls in flight at once.
const MaxConcurrentGenerations = 4

// Service returns teacher clips, generating and caching each on first request.
type Service struct {
	Cfg   *config.Config
	Songs songs.Store
	API   *API

	clipFile *regexp.Regexp

	mu       sync.Mutex
	pending  map[string]*call
	failures map[string]failure // the last failure per clip path, until it expires
	quota    *Error
	// Generations in flight; a slot is a token in the channel.
	slots chan struct{}

	// Teacher voices by language, loaded from voices.json on first use. voicesMu guards the map
	// and the file; langLocks serialize creating one language's voice without blocking others.
	voicesMu    sync.Mutex
	voices      map[string]string
	langLocksMu sync.Mutex
	langLocks   map[string]*sync.Mutex
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
		Cfg:       cfg,
		Songs:     store,
		API:       api,
		clipFile:  ClipPattern(cfg.Speeds),
		pending:   map[string]*call{},
		failures:  map[string]failure{},
		slots:     make(chan struct{}, MaxConcurrentGenerations),
		langLocks: map[string]*sync.Mutex{},
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

// loadVoicesLocked reads voices.json once; voicesMu must be held.
func (s *Service) loadVoicesLocked() error {
	if s.voices != nil {
		return nil
	}
	voices := map[string]string{}
	raw, err := os.ReadFile(s.voicesFile())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(raw, &voices); err != nil {
			return err
		}
	}
	s.voices = voices
	return nil
}

func (s *Service) cachedVoice(language string) (string, error) {
	s.voicesMu.Lock()
	defer s.voicesMu.Unlock()
	if err := s.loadVoicesLocked(); err != nil {
		return "", err
	}
	return s.voices[language], nil
}

func (s *Service) storeVoice(language, id string) error {
	s.voicesMu.Lock()
	defer s.voicesMu.Unlock()
	if err := s.loadVoicesLocked(); err != nil {
		return err
	}
	s.voices[language] = id
	return songs.WriteAtomic(s.voicesFile(), songs.MarshalIndent(s.voices))
}

func (s *Service) langLock(language string) *sync.Mutex {
	s.langLocksMu.Lock()
	defer s.langLocksMu.Unlock()
	l := s.langLocks[language]
	if l == nil {
		l = &sync.Mutex{}
		s.langLocks[language] = l
	}
	return l
}

// TeacherVoice returns the language's teacher voice, designing it on first use (cached in memory
// and in voices.json). A language's voice is designed once at a time; other languages and
// already-known voices don't wait for it.
func (s *Service) TeacherVoice(ctx context.Context, language string) (string, error) {
	if id, err := s.cachedVoice(language); err != nil || id != "" {
		return id, err
	}
	lock := s.langLock(language)
	lock.Lock()
	defer lock.Unlock()
	if id, err := s.cachedVoice(language); err != nil || id != "" {
		return id, err
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
	if err := s.storeVoice(language, id); err != nil {
		return "", err
	}
	return id, nil
}

// LastError reports why the last generation of a clip failed, while that failure is still
// remembered. The study page asks, as media elements never expose the response body.
func (s *Service) LastError(videoID, file string) (*Error, bool) {
	path := filepath.Join(s.Songs.AudioDir(videoID), file)
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.failures[path]; ok {
		if time.Now().Before(f.until) {
			return f.err, true
		}
		delete(s.failures, path)
	}
	// During a quota block Clip refuses every clip, including ones never attempted.
	if s.quota != nil && time.Now().Before(s.quota.RetryAt) {
		return s.quota, true
	}
	return nil, false
}

// recordFailure remembers a failed generation, and drops the expired entries.
func (s *Service) recordFailure(path string, e *Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for p, f := range s.failures {
		if !now.Before(f.until) {
			delete(s.failures, p)
		}
	}
	until := now.Add(failureTTL)
	if e.Status == 429 {
		// A quota error is remembered as long as the quota block lasts, not just failureTTL.
		s.quota = e
		until = e.RetryAt
	}
	s.failures[path] = failure{err: e, until: until}
}

// withSlot runs fn once one of the MaxConcurrentGenerations slots is free, or fails with ctx's
// error if ctx ends first.
func (s *Service) withSlot(ctx context.Context, fn func() ([]byte, error)) ([]byte, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
		return fn()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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
	if s.quota != nil && !time.Now().Before(s.quota.RetryAt) {
		s.quota = nil
	}
	if s.quota != nil {
		err := s.quota
		s.mu.Unlock()
		return nil, err
	}
	if f, ok := s.failures[path]; ok {
		if time.Now().Before(f.until) {
			s.mu.Unlock()
			return nil, f.err
		}
		delete(s.failures, path)
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
	c.audio, c.err = s.withSlot(ctx, func() ([]byte, error) {
		return s.synthesizeClip(ctx, videoID, path, hash, speed)
	})

	var e *Error
	if errors.As(c.err, &e) {
		s.recordFailure(path, e)
	}
	s.mu.Lock()
	if c.err == nil {
		delete(s.failures, path)
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
