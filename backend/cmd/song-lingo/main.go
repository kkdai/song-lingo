// Command song-lingo is the Song Lingo back end: the HTTP API (`serve`) and the song pipeline
// as command-line tools.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/config"
	"github.com/kkdai/song-lingo/backend/internal/server"
)

const usage = `Usage:
  song-lingo [serve]                          start the API server on 127.0.0.1:$BACKEND_PORT (default 8081)
  song-lingo transcribe <youtube_url>         lyrics and timestamps        → output/<id>.json
  song-lingo annotate <output/<id>.json>      romanization, translation…   → output/<id>.annotated.json
  song-lingo speak <output/<id>.annotated.json> [--workers 4]
                                              all teacher clips of a song   → output/audio/<id>/
`

func main() {
	log.SetFlags(0)
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	switch cmd {
	case "serve":
		err = serve(cfg)
	case "transcribe":
		err = transcribeCmd(cfg, os.Args[2:])
	case "annotate":
		err = annotateCmd(cfg, os.Args[2:])
	case "speak":
		err = speakCmd(cfg, os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func serve(cfg *config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8081"
	}
	progressStore, err := server.NewProgressStore(ctx, cfg)
	if err != nil {
		return err
	}
	srv := &http.Server{
		// Localhost only: requests reach the API through Next.js, which checks access first.
		Addr:              net.JoinHostPort("127.0.0.1", port),
		Handler:           server.New(cfg, progressStore).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Printf("song-lingo API on http://%s (data: %s)", srv.Addr, cfg.DataDir)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
