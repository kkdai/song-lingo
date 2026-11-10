package server

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/gemini"
	"github.com/kkdai/song-lingo/backend/internal/progress"
	"github.com/kkdai/song-lingo/backend/internal/shadow"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

const maxRecordingBytes = 5 * 1024 * 1024

// Browser recording formats → a MIME type gemini-3.5-transcribe accepts.
var recordingFormats = map[string]string{
	"audio/webm":  "audio/webm",
	"audio/ogg":   "audio/ogg",
	"audio/mp4":   "audio/m4a", // Safari / iOS
	"audio/m4a":   "audio/m4a",
	"audio/x-m4a": "audio/m4a",
	"audio/aac":   "audio/aac",
	"audio/wav":   "audio/wav",
}

// shadowGate allows one scoring at a time, and no calls at all while the transcribe quota is used
// up: a double-tap or a retry storm must not be billed twice.
type shadowGate struct {
	mu         sync.Mutex
	busy       bool
	quotaUntil time.Time
}

func (g *shadowGate) enter() (status int, message string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if wait := time.Until(g.quotaUntil); wait > 0 {
		return 429, fmt.Sprintf("跟讀評分的額度已用完，大約 %d 分鐘後再試。", int(math.Ceil(wait.Minutes())))
	}
	if g.busy {
		return 429, "上一次跟讀還在評分中，請稍等。"
	}
	g.busy = true
	return 0, ""
}

func (g *shadowGate) leave(quotaFor time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.busy = false
	if quotaFor > 0 {
		g.quotaUntil = time.Now().Add(quotaFor)
	}
}

// shadowLine scores a shadowing attempt: the learner's recording of one line is transcribed and
// compared with that line. The recording stays in memory only for the call; nothing is logged or
// stored except the score and missed words, which go into the song's study progress.
func (s *Server) shadowLine(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	index, ok := lineIndex(r)
	if !songs.IsVideoID(id) || !ok {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	if status, message := s.shadowGate.enter(); status != 0 {
		fail(w, status, message)
		return
	}
	var quotaFor time.Duration
	defer func() { s.shadowGate.leave(quotaFor) }()

	song, err := s.songs.Read(id)
	if err != nil || index >= len(song.Lines) {
		fail(w, http.StatusNotFound, "找不到這一句。")
		return
	}
	line := song.Lines[index]

	r.Body = http.MaxBytesReader(w, r.Body, maxRecordingBytes+1024*1024)
	file, header, err := r.FormFile("audio")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, http.StatusRequestEntityTooLarge, "錄音太長了，請只念這一句。")
		} else {
			fail(w, http.StatusBadRequest, "沒有收到錄音。")
		}
		return
	}
	defer file.Close()
	if header.Size < 1000 {
		fail(w, http.StatusUnprocessableEntity, "錄音太短了，請按住按鈕念完整句再放開。")
		return
	}
	if header.Size > maxRecordingBytes {
		fail(w, http.StatusRequestEntityTooLarge, "錄音太長了，請只念這一句。")
		return
	}
	contentType := header.Header.Get("Content-Type")
	mimeType := recordingFormats[strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))]
	if mimeType == "" {
		if contentType == "" {
			contentType = "未知"
		}
		fail(w, http.StatusUnsupportedMediaType, fmt.Sprintf("不支援這種錄音格式（%s）。", contentType))
		return
	}
	audio, err := io.ReadAll(file)
	if err != nil {
		fail(w, http.StatusBadRequest, "沒有收到錄音。")
		return
	}

	transcript, err := s.shadow.Transcribe(r.Context(), audio, mimeType, song.Language)
	if err != nil {
		var e *gemini.Error
		if errors.As(err, &e) && e.Status == 429 {
			quotaFor = max(time.Until(e.RetryAt), time.Minute)
			fail(w, http.StatusTooManyRequests, "跟讀評分的額度已用完，請稍後再試。")
			return
		}
		log.Printf("[shadow] transcription failed for %s: %v", id, err)
		fail(w, http.StatusBadGateway, "評分失敗："+firstLine(err.Error(), 300))
		return
	}
	if strings.TrimSpace(transcript) == "" {
		fail(w, http.StatusUnprocessableEntity, "沒有聽到聲音，請靠近麥克風再念一次。")
		return
	}

	expected := shadow.Expected{Language: song.Language, Text: line.Text, Reading: line.ReadingOrEmpty()}
	for _, t := range line.Tokens {
		expected.Tokens = append(expected.Tokens, shadow.Token{Surface: t.Surface, Reading: t.Reading})
	}
	result := shadow.Score(expected, transcript)
	attempt := progress.ShadowAttempt{At: progress.Now(), Score: result.Score, Missed: []string{}}
	for _, word := range result.Words {
		if word.Status != "ok" {
			attempt.Missed = append(attempt.Missed, word.Surface)
		}
	}
	// The transcription is already paid for: if saving fails, still return the score (marked
	// unsaved) rather than an error that would make the learner record and pay again.
	saved := true
	if _, err := s.progress.Update(r.Context(), id, func(p *progress.Progress) {
		p.LastLine = index
		p.AddAttempt(line.Text, attempt)
	}); err != nil {
		saved = false
		log.Printf("[shadow] progress not saved for %s: %v", id, err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transcript": result.Transcript,
		"score":      result.Score,
		"words":      result.Words,
		"attempt":    attempt,
		"saved":      saved,
	})
}

func firstLine(s string, limit int) string {
	s, _, _ = strings.Cut(s, "\n")
	if r := []rune(s); len(r) > limit {
		return string(r[:limit])
	}
	return s
}
