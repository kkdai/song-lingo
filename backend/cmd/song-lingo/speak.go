package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/config"
	"github.com/kkdai/song-lingo/backend/internal/songs"
	"github.com/kkdai/song-lingo/backend/internal/tts"
)

const speakAttempts = 3

// speakCmd pre-generates every teacher clip of a song. The web app also generates clips on demand
// the first time a line is played, so this only warms the cache (each clip costs one TTS request).
func speakCmd(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("speak", flag.ExitOnError)
	workers := fs.Int("workers", 4, "parallel requests")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: song-lingo speak <output/<id>.annotated.json> [--workers 4]") }
	pos, err := parseArgs(fs, args, 1)
	if err != nil {
		return err
	}
	song, err := songs.ReadFile(pos[0])
	if err != nil {
		return fmt.Errorf("%s: %w", pos[0], err)
	}
	videoID := strings.SplitN(filepath.Base(pos[0]), ".", 2)[0]
	audioDir := filepath.Join(filepath.Dir(pos[0]), "audio", videoID)
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		return err
	}

	ctx := context.Background()
	service := tts.NewService(cfg, songs.Store{Dir: cfg.DataDir}, &tts.API{Base: tts.DefaultAPIBase, APIKey: cfg.APIKey})
	voice, err := service.TeacherVoice(ctx, song.Language)
	if err != nil {
		return err
	}

	type job struct{ path, text, style string }
	var todo []job
	texts := songs.UniqueTexts(song.Lines)
	total := 0
	for _, text := range texts {
		for _, speed := range cfg.Speeds {
			total++
			path := filepath.Join(audioDir, songs.ClipName(text, speed))
			if _, err := os.Stat(path); err != nil {
				todo = append(todo, job{path, text, cfg.Styles[speed]})
			}
		}
	}
	fmt.Printf("%d unique lines -> %d clips (%d cached, %d to generate)\n", len(texts), total, total-len(todo), len(todo))

	var (
		done          atomic.Int32
		quotaOnce     sync.Once
		quotaExceeded atomic.Bool
		mu            sync.Mutex
		failures      []string
		wg            sync.WaitGroup
	)
	jobs := make(chan job)
	for range max(*workers, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if msg := speakOne(ctx, service, voice, j.path, j.text, j.style, &quotaExceeded, &quotaOnce); msg != "" {
					mu.Lock()
					failures = append(failures, msg)
					mu.Unlock()
					continue
				}
				fmt.Printf("[%d/%d] %s\n", done.Add(1), len(todo), filepath.Base(j.path))
			}
		}()
	}
	for _, j := range todo {
		jobs <- j
	}
	close(jobs)
	wg.Wait()

	fmt.Printf("generated: %d  failed: %d\n", done.Load(), len(failures))
	for _, f := range failures[:min(len(failures), 5)] {
		fmt.Printf("  %s\n", f)
	}
	switch {
	case quotaExceeded.Load():
		fmt.Println("Stopped early: daily TTS quota exhausted. Re-run after it resets; finished clips are kept.")
	case len(failures) > 0:
		fmt.Println("Re-run to retry failed clips.")
	}
	fmt.Printf("audio dir: %s\n", audioDir)
	return nil
}

// speakOne generates one clip with retries; it returns a failure message, or "" on success.
func speakOne(ctx context.Context, service *tts.Service, voice, path, text, style string, quotaExceeded *atomic.Bool, quotaOnce *sync.Once) string {
	name := filepath.Base(path)
	for attempt := 1; attempt <= speakAttempts; attempt++ {
		if quotaExceeded.Load() {
			return name + ": skipped (daily quota exhausted)"
		}
		audio, err := tts.Synthesize(ctx, service.API, service.Cfg.TTSModel, voice, text, style)
		if err == nil {
			if err := songs.WriteAtomic(path, audio); err != nil {
				return name + ": " + err.Error()
			}
			return ""
		}
		var e *tts.Error
		// A daily quota won't come back by retrying; stop every worker.
		if errors.As(err, &e) && e.Status == 429 && strings.Contains(e.Message, "per day") {
			quotaOnce.Do(func() { fmt.Printf("Daily quota exhausted: %s\n", e.Message) })
			quotaExceeded.Store(true)
			return name + ": skipped (daily quota exhausted)"
		}
		if attempt == speakAttempts {
			return name + ": " + err.Error()
		}
		fmt.Printf("retry %d/%d %s: %v\n", attempt, speakAttempts-1, name, err)
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
	return ""
}
