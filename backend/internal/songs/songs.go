// Package songs reads and writes the song files in the data directory:
// <id>.json (the transcript) and <id>.annotated.json (what the study page shows).
package songs

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var ErrNotFound = errors.New("not found")

var videoID = regexp.MustCompile(`^[\w-]{11}$`)

func IsVideoID(id string) bool { return videoID.MatchString(id) }

type Token struct {
	Surface   string `json:"surface"`
	Reading   string `json:"reading"`
	Pos       string `json:"pos"`
	MeaningZh string `json:"meaning_zh"`
}

type Line struct {
	Start     string  `json:"start"`
	End       string  `json:"end"`
	Text      string  `json:"text"`
	Reading   *string `json:"reading"`
	Section   *string `json:"section"`
	Uncertain bool    `json:"uncertain"`

	// Added by annotation.
	TranslationZh    *string `json:"translation_zh,omitempty"`
	Tokens           []Token `json:"tokens,omitempty"`
	Romanization     *string `json:"romanization,omitempty"`
	GrammarNote      *string `json:"grammar_note,omitempty"`
	PronunciationTip *string `json:"pronunciation_tip,omitempty"`
	NeedsReview      *bool   `json:"needs_review,omitempty"`

	// Set by the review UI.
	Reviewed            *bool  `json:"reviewed,omitempty"`
	TranslationZhManual string `json:"translation_zh_manual,omitempty"`
	// Text or reading was edited; tokens/romanization are outdated until re-annotated.
	Stale bool `json:"stale,omitempty"`

	// Only in API responses, never stored.
	Audio       map[string]string `json:"audio,omitempty"`
	AudioCached []string          `json:"audioCached,omitempty"`
	ProgressKey string            `json:"progressKey,omitempty"`
}

// ReadingOrEmpty is the line's reading, or "" when it has none.
func (l *Line) ReadingOrEmpty() string {
	if l.Reading == nil {
		return ""
	}
	return *l.Reading
}

// File is the content of a transcript or annotated song file.
type File struct {
	Language    string  `json:"language"`
	TitleGuess  *string `json:"title_guess"`
	ArtistGuess *string `json:"artist_guess"`
	Source      string  `json:"source"`
	Lines       []Line  `json:"lines"`
}

type Song struct {
	ID string `json:"id"`
	File
}

// TextHash keys teacher clips and study progress by a line's content, so editing a lyric only
// invalidates that line and repeated choruses share one entry.
func TextHash(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

func ClipName(text, speed string) string { return TextHash(text) + "_" + speed + ".wav" }

// UniqueTexts returns each distinct line text once, in order of first appearance.
func UniqueTexts(lines []Line) []string {
	seen := map[string]bool{}
	var texts []string
	for _, l := range lines {
		if !seen[l.Text] {
			seen[l.Text] = true
			texts = append(texts, l.Text)
		}
	}
	return texts
}

type Store struct{ Dir string }

func (s Store) TranscriptPath(id string) string { return filepath.Join(s.Dir, id+".json") }
func (s Store) AnnotatedPath(id string) string  { return filepath.Join(s.Dir, id+".annotated.json") }
func (s Store) AudioDir(id string) string       { return filepath.Join(s.Dir, "audio", id) }

// ReadFile reads a song file; a missing file is ErrNotFound.
func ReadFile(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// WriteFile writes a song file atomically (tmp + rename).
func WriteFile(path string, f *File) error {
	return WriteAtomic(path, MarshalIndent(f))
}

// MarshalIndent formats JSON like the existing files: two-space indent, non-ASCII kept as is.
func MarshalIndent(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		panic(err) // only plain data structures are encoded here
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// WriteAtomic writes data to path via a unique temp file in the same directory plus a rename,
// creating the parent directory if needed. The temp file is removed on failure.
func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(name, 0o644)
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}

// Read returns an annotated song, or ErrNotFound.
func (s Store) Read(id string) (*Song, error) {
	if !IsVideoID(id) {
		return nil, ErrNotFound
	}
	f, err := ReadFile(s.AnnotatedPath(id))
	if err != nil {
		return nil, err
	}
	return &Song{ID: id, File: *f}, nil
}

// IDs lists the songs that have been annotated.
func (s Store) IDs() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".annotated.json")
		if ok && IsVideoID(id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// CachedClips lists the teacher clips already generated for a song.
func (s Store) CachedClips(id string) map[string]bool {
	cached := map[string]bool{}
	entries, _ := os.ReadDir(s.AudioDir(id))
	for _, e := range entries {
		cached[e.Name()] = true
	}
	return cached
}

var songLocks sync.Map // video ID -> *sync.Mutex

// Lock takes the song's lock and returns the function that releases it. Everything that
// read-modify-writes a song's files (review edits, re-annotation) holds it around that step.
// Don't hold it across slow calls such as Gemini requests.
func Lock(id string) (unlock func()) {
	m, _ := songLocks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
