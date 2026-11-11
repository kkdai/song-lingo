package tts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// BatchAttempts is how many times GenerateAll tries a clip before giving up on it.
const BatchAttempts = 3

// BatchResult summarizes a GenerateAll run.
type BatchResult struct {
	Unique, Total, Cached, Generated int
	// One message per clip that was not generated (failed, or skipped after a stop).
	Failures []string
	// The daily quota ran out: the rest of the clips were skipped.
	DailyQuota bool
}

// GenerateAll generates every clip of a song that is not in audioDir yet, with the given number of
// parallel requests (the Gemini TTS quota is per request, so this costs one request per clip). It
// stops early when ctx ends or the daily quota is used up; finished clips are kept. logf, if not
// nil, receives progress lines (from several goroutines, so it must be safe for concurrent use). The error is only for failing before any clip (the teacher voice).
func (s *Service) GenerateAll(ctx context.Context, song *songs.File, audioDir string, workers int, logf func(format string, args ...any)) (BatchResult, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if _, err := s.TeacherVoice(ctx, song.Language); err != nil {
		return BatchResult{}, ttsMessage(err)
	}

	type job struct{ path, text, speed string }
	var todo []job
	texts := songs.UniqueTexts(song.Lines)
	result := BatchResult{Unique: len(texts)}
	for _, text := range texts {
		for _, speed := range s.Cfg.Speeds {
			result.Total++
			path := filepath.Join(audioDir, songs.ClipName(text, speed))
			if _, err := os.Stat(path); err != nil {
				todo = append(todo, job{path, text, speed})
			}
		}
	}
	result.Cached = result.Total - len(todo)
	logf("%d unique lines -> %d clips (%d cached, %d to generate)", result.Unique, result.Total, result.Cached, len(todo))

	var (
		done  atomic.Int32
		stop  atomic.Bool // the daily quota is used up
		once  sync.Once
		mu    sync.Mutex
		wg    sync.WaitGroup
		queue = make(chan job)
	)
	fail := func(msg string) {
		mu.Lock()
		result.Failures = append(result.Failures, msg)
		mu.Unlock()
	}
	for range max(workers, 1) {
		wg.Go(func() {
			for j := range queue {
				name := filepath.Base(j.path)
				if err := s.generateWithRetry(ctx, song.Language, j.text, j.speed, j.path, &stop, &once, logf); err != nil {
					fail(name + ": " + err.Error())
					continue
				}
				logf("[%d/%d] %s", done.Add(1), len(todo), name)
			}
		})
	}
feed:
	for _, j := range todo {
		select {
		case queue <- j:
		case <-ctx.Done():
			break feed
		}
	}
	close(queue)
	wg.Wait()

	result.Generated = int(done.Load())
	result.DailyQuota = stop.Load()
	if skipped := len(todo) - result.Generated - len(result.Failures); skipped > 0 && ctx.Err() != nil {
		fail(fmt.Sprintf("%d clips skipped: %v", skipped, ctx.Err()))
	}
	return result, nil
}

var errDailyQuota = errors.New("skipped (daily quota exhausted)")

// generateWithRetry generates and saves one clip, retrying with a growing pause. A daily quota
// won't come back by retrying: it sets stop so that every worker gives up.
func (s *Service) generateWithRetry(ctx context.Context, language, text, speed, path string, stop *atomic.Bool, once *sync.Once, logf func(string, ...any)) error {
	name := filepath.Base(path)
	for attempt := 1; ; attempt++ {
		if stop.Load() {
			return errDailyQuota
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		audio, err := s.synthesizeText(ctx, language, text, speed)
		if err == nil {
			return songs.WriteAtomic(path, audio)
		}
		var e *Error
		if errors.As(err, &e) && e.Status == 429 && e.Daily {
			once.Do(func() { logf("Daily quota exhausted: %s", e.Message) })
			stop.Store(true)
			return errDailyQuota
		}
		if attempt == BatchAttempts {
			return err
		}
		logf("retry %d/%d %s: %v", attempt, BatchAttempts-1, name, err)
		select {
		case <-time.After(time.Duration(1<<attempt) * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
