package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/kkdai/song-lingo/backend/internal/songs"
)

func ptr[T any](v T) *T { return &v }

func TestParseVideoID(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=1": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                    "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":      "dQw4w9WgXcQ",
		"https://www.youtube.com/live/dQw4w9WgXcQ":        "dQw4w9WgXcQ",
		" dQw4w9WgXcQ ":       "dQw4w9WgXcQ",
		"https://example.com": "",
	} {
		if got, _ := ParseVideoID(in); got != want {
			t.Errorf("ParseVideoID(%q) = %q", in, got)
		}
	}
}

func TestApplyNotes(t *testing.T) {
	transcript := &songs.File{Language: "ja", Lines: []songs.Line{
		{Text: "君の名は", Reading: ptr("きみのなは")},
		{Text: "東京", Reading: ptr("ひがしきょう"), Stale: true, TranslationZhManual: "東京啊", Reviewed: ptr(true)},
		{Text: "君の名は", Reading: ptr("きみのなは"), Uncertain: true},
	}}
	notes := []lineNote{
		{Index: 0, TranslationZh: "你的名字", Tokens: []songs.Token{
			{Surface: "君", Reading: "きみ"}, {Surface: "の", Reading: "の", Pos: "particle"},
			{Surface: "名", Reading: "な"}, {Surface: "は", Reading: "は", Pos: "particle"}}},
		{Index: 1, TranslationZh: "東京", Tokens: []songs.Token{{Surface: "東京", Reading: "とうきょう"}}},
	}
	out, result := applyNotes(transcript, songs.UniqueTexts(transcript.Lines), notes)

	a, b, c := out.Lines[0], out.Lines[1], out.Lines[2]
	if *a.Romanization != "kimi no na wa" || *a.NeedsReview || *a.TranslationZh != "你的名字" || *a.GrammarNote != "" {
		t.Errorf("line 0 = %+v", a)
	}
	// Reviewed lines keep the manual translation and aren't flagged even though the readings differ.
	if *b.TranslationZh != "東京啊" || *b.NeedsReview || b.Stale {
		t.Errorf("line 1 = %+v", b)
	}
	if !c.Uncertain || *c.Romanization != "kimi no na wa" {
		t.Errorf("line 2 = %+v", c)
	}
	if transcript.Lines[1].Stale != true {
		t.Error("transcript was modified")
	}
	want := "finish_reason: \nannotated: 2/2  missing indexes: []\nlines where tokens don't cover the text: 0\nlines needing review: 0"
	if result.String() != want {
		t.Errorf("summary = %q", result.String())
	}
}

func TestApplyNotesFlagsReadingMismatch(t *testing.T) {
	transcript := &songs.File{Language: "ja", Lines: []songs.Line{{Text: "東京", Reading: ptr("ひがしきょう")}, {Text: "x"}}}
	out, result := applyNotes(transcript, []string{"東京", "x"}, []lineNote{{Index: 0, Tokens: []songs.Token{{Surface: "東", Reading: "とうきょう"}}}})
	if !*out.Lines[0].NeedsReview || result.NeedsReview != 1 || result.TokenMismatch != 1 || len(result.Missing) != 1 {
		t.Errorf("line = %+v, result = %+v", out.Lines[0], result)
	}
	if out.Lines[1].TranslationZh != nil {
		t.Error("missing line got annotations")
	}
}

// fakeGemini answers generateContent with the given JSON texts in turn.
func fakeGemini(t *testing.T, answers ...string) (func(ctx context.Context) (*genai.Client, error), *[]string) {
	var mu sync.Mutex
	var prompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		prompts = append(prompts, string(body))
		answer := answers[0]
		answers = answers[1:]
		mu.Unlock()
		if !strings.HasSuffix(r.URL.Path, "/models/"+DefaultModel+":generateContent") {
			t.Errorf("path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": answer}}},
			"finishReason": "STOP",
		}}})
	}))
	t.Cleanup(srv.Close)
	return func(ctx context.Context) (*genai.Client, error) {
		return genai.NewClient(ctx, &genai.ClientConfig{APIKey: "test", Backend: genai.BackendGeminiAPI, HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL}})
	}, &prompts
}

func TestAddSong(t *testing.T) {
	client, prompts := fakeGemini(t,
		`{"language":"ja","title_guess":"歌","artist_guess":null,"source":"audio","lines":[
			{"start":"00:01","end":"00:02","text":"東京","reading":"とうきょう","section":null,"uncertain":false},
			{"start":"00:03","end":"00:04","text":"東京","reading":"とうきょう","section":"chorus","uncertain":false}]}`,
		`{"lines":[{"index":0,"translation_zh":"東京","tokens":[{"surface":"東京","reading":"とうきょう","pos":"noun","meaning_zh":"東京"}],"romanization":"","grammar_note":"","pronunciation_tip":"長音"}]}`,
	)
	store := songs.Store{Dir: t.TempDir()}
	r := NewRunner(store, DefaultModel, client)
	job, err := r.AddSong("https://youtu.be/dQw4w9WgXcQ")
	if err != nil || job.Stage != StageTranscribing || job.StartedAt == 0 {
		t.Fatalf("job = %+v, %v", job, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.Status("dQw4w9WgXcQ").Stage != StageDone {
		if s := r.Status("dQw4w9WgXcQ"); s.Stage == StageError || time.Now().After(deadline) {
			t.Fatalf("status = %+v", s)
		}
		time.Sleep(10 * time.Millisecond)
	}
	song, err := store.Read("dQw4w9WgXcQ")
	if err != nil || len(song.Lines) != 2 || *song.Lines[1].Romanization != "toukyou" || *song.Lines[1].PronunciationTip != "長音" {
		t.Fatalf("song = %+v, %v", song, err)
	}
	if !strings.Contains((*prompts)[0], "youtube.com/watch?v=dQw4w9WgXcQ") || !strings.Contains((*prompts)[1], `0: 東京`) ||
		strings.Contains((*prompts)[1], `1: 東京`) {
		t.Errorf("prompts = %v", *prompts)
	}
	// Adding it again is a no-op.
	if job, _ := r.AddSong("dQw4w9WgXcQ"); job.Stage != StageDone {
		t.Errorf("again: %+v", job)
	}
}

func TestReannotateKeepsEdits(t *testing.T) {
	client, _ := fakeGemini(t, `{"lines":[{"index":0,"translation_zh":"新","tokens":[]}]}`)
	store := songs.Store{Dir: t.TempDir()}
	songs.WriteFile(store.TranscriptPath("dQw4w9WgXcQ"), &songs.File{Language: "en", Lines: []songs.Line{
		{Text: "hello", TranslationZhManual: "你好"}}})
	r := NewRunner(store, DefaultModel, client)
	summary, err := r.Reannotate("dQw4w9WgXcQ")
	if err != nil || summary != "finish_reason: STOP\nannotated: 1/1  missing indexes: []" {
		t.Fatalf("%q %v", summary, err)
	}
	song, _ := store.Read("dQw4w9WgXcQ")
	if *song.Lines[0].TranslationZh != "你好" || song.Lines[0].NeedsReview != nil {
		t.Errorf("line = %+v", song.Lines[0])
	}
	if _, err := r.Reannotate("zzzzzzzzzzz"); err == nil || err.(*Error).Status != 404 {
		t.Errorf("missing song: %v", err)
	}
}

func TestSaveAnnotation(t *testing.T) {
	r := &Runner{Songs: songs.Store{Dir: t.TempDir()}}
	id := "dQw4w9WgXcQ"
	transcript := &songs.File{Language: "ja", Lines: []songs.Line{{Text: "a"}}}
	if err := songs.WriteFile(r.Songs.TranscriptPath(id), transcript); err != nil {
		t.Fatal(err)
	}
	annotated := &songs.File{Language: "ja", Lines: []songs.Line{{Text: "a", TranslationZh: ptr("A")}}}

	// Edited meanwhile: nothing is written.
	edited := &songs.File{Language: "ja", Lines: []songs.Line{{Text: "b"}}}
	if err := songs.WriteFile(r.Songs.TranscriptPath(id), edited); err != nil {
		t.Fatal(err)
	}
	if err := r.saveAnnotation(id, transcript, annotated); err == nil {
		t.Fatal("expected a conflict error")
	}
	if _, err := os.Stat(r.Songs.AnnotatedPath(id)); !os.IsNotExist(err) {
		t.Errorf("annotated file written despite conflict (err=%v)", err)
	}

	// Unchanged: written.
	if err := r.saveAnnotation(id, edited, annotated); err != nil {
		t.Fatal(err)
	}
	got, err := songs.ReadFile(r.Songs.AnnotatedPath(id))
	if err != nil || got.Lines[0].TranslationZh == nil || *got.Lines[0].TranslationZh != "A" {
		t.Errorf("annotated = %+v, err = %v", got, err)
	}
}

func TestShutdownCancelsJobs(t *testing.T) {
	started := make(chan struct{})
	client := func(ctx context.Context) (*genai.Client, error) {
		close(started)
		<-ctx.Done() // a job stuck on the network until it is cancelled
		return nil, ctx.Err()
	}
	r := NewRunner(songs.Store{Dir: t.TempDir()}, DefaultModel, client)
	if _, err := r.AddSong("dQw4w9WgXcQ"); err != nil {
		t.Fatal(err)
	}
	<-started
	begin := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.Cancel()
	r.Wait(ctx)
	if time.Since(begin) > 2*time.Second || ctx.Err() != nil {
		t.Errorf("shutdown took %v", time.Since(begin))
	}
	if st := r.Status("dQw4w9WgXcQ"); st == nil || st.Stage != StageError {
		t.Errorf("status = %+v, want error", st)
	}
}

func TestAddSongTwiceStartsOneJob(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	client := func(ctx context.Context) (*genai.Client, error) {
		calls.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, errors.New("stop")
	}
	r := NewRunner(songs.Store{Dir: t.TempDir()}, DefaultModel, client)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); r.AddSong("dQw4w9WgXcQ") }()
	}
	wg.Wait()
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.Wait(ctx)
	if calls.Load() != 1 {
		t.Errorf("jobs started = %d, want 1", calls.Load())
	}
}
