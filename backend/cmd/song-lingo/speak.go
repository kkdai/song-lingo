package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kkdai/song-lingo/backend/internal/config"
	"github.com/kkdai/song-lingo/backend/internal/songs"
	"github.com/kkdai/song-lingo/backend/internal/tts"
)

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

	// Ctrl-C stops the run; the clips finished so far are kept.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service := tts.NewService(cfg, songs.Store{Dir: cfg.DataDir}, &tts.API{Base: tts.DefaultAPIBase, APIKey: cfg.APIKey})
	result, err := service.GenerateAll(ctx, song, audioDir, *workers, func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
	})
	if err != nil {
		return err
	}

	fmt.Printf("generated: %d  failed: %d\n", result.Generated, len(result.Failures))
	for _, f := range result.Failures[:min(len(result.Failures), 5)] {
		fmt.Printf("  %s\n", f)
	}
	switch {
	case result.DailyQuota:
		fmt.Println("Stopped early: daily TTS quota exhausted. Re-run after it resets; finished clips are kept.")
	case ctx.Err() != nil:
		fmt.Println("Interrupted. Re-run to generate the rest; finished clips are kept.")
	case len(result.Failures) > 0:
		fmt.Println("Re-run to retry failed clips.")
	}
	fmt.Printf("audio dir: %s\n", audioDir)
	return nil
}
