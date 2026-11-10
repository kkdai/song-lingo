package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/genai"

	"github.com/kkdai/song-lingo/backend/internal/config"
	"github.com/kkdai/song-lingo/backend/internal/pipeline"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// parseArgs parses flags placed before or after the positional arguments.
func parseArgs(fs *flag.FlagSet, args []string, positional int) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) > 0 {
			pos = append(pos, args[0])
			args = args[1:]
		}
	}
	if len(pos) != positional {
		fs.Usage()
		return nil, errors.New("參數數量不正確")
	}
	return pos, nil
}

func geminiClient(ctx context.Context, cfg *config.Config) (*genai.Client, error) {
	key, err := cfg.APIKey()
	if err != nil {
		return nil, err
	}
	return pipeline.NewClient(ctx, key)
}

func transcribeCmd(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("transcribe", flag.ExitOnError)
	model := fs.String("model", pipeline.DefaultModel, "Gemini model")
	out := fs.String("out", cfg.DataDir, "output directory")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: song-lingo transcribe <youtube_url> [--model M] [--out DIR]") }
	pos, err := parseArgs(fs, args, 1)
	if err != nil {
		return err
	}
	id, ok := pipeline.ParseVideoID(pos[0])
	if !ok {
		return fmt.Errorf("不是 YouTube 網址：%s", pos[0])
	}
	ctx, cancel := context.WithTimeout(context.Background(), pipeline.StepTimeout)
	defer cancel()
	client, err := geminiClient(ctx, cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Transcribing %s with %s ...\n", id, *model)
	transcript, err := pipeline.Transcribe(ctx, client, *model, id)
	if err != nil {
		return err
	}
	path := filepath.Join(*out, id+".json")
	if err := songs.WriteFile(path, transcript); err != nil {
		return err
	}
	uncertain := 0
	for _, l := range transcript.Lines {
		if l.Uncertain {
			uncertain++
		}
	}
	fmt.Printf("language: %s  source: %s\n", transcript.Language, transcript.Source)
	fmt.Printf("guess: %s - %s\n", deref(transcript.ArtistGuess), deref(transcript.TitleGuess))
	fmt.Printf("lines: %d  uncertain: %d\nsaved: %s\n", len(transcript.Lines), uncertain, path)
	return nil
}

func annotateCmd(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("annotate", flag.ExitOnError)
	model := fs.String("model", pipeline.DefaultModel, "Gemini model")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: song-lingo annotate <transcript.json> [--model M]") }
	pos, err := parseArgs(fs, args, 1)
	if err != nil {
		return err
	}
	transcript, err := songs.ReadFile(pos[0])
	if err != nil {
		return fmt.Errorf("%s: %w", pos[0], err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), pipeline.StepTimeout)
	defer cancel()
	client, err := geminiClient(ctx, cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Annotating %d unique lines (%d total, language=%s) ...\n",
		len(songs.UniqueTexts(transcript.Lines)), len(transcript.Lines), transcript.Language)
	annotated, result, err := pipeline.Annotate(ctx, client, *model, transcript)
	if err != nil {
		return err
	}
	path := strings.TrimSuffix(pos[0], ".json") + ".annotated.json"
	if err := songs.WriteFile(path, annotated); err != nil {
		return err
	}
	fmt.Printf("%s\nsaved: %s\n", result, path)
	return nil
}

func deref(s *string) string {
	if s == nil {
		return "?"
	}
	return *s
}
