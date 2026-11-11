package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kkdai/song-lingo/backend/internal/config"
	"github.com/kkdai/song-lingo/backend/internal/progress"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

const songID = "abcdefghijk"

// newTestServer serves a data dir holding one Japanese song with one cached clip.
func newTestServer(t *testing.T) (*Server, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir:  dir,
		Teachers: map[string]config.Teacher{"ja": {LanguageCode: "ja-JP"}},
		Styles:   map[string]string{"normal": "n", "slow": "s"},
		Speeds:   []string{"normal", "slow"},
	}
	reading := "きみ"
	song := &songs.File{Language: "ja", Source: "audio", Lines: []songs.Line{
		{Start: "00:01", End: "00:02", Text: "君", Reading: &reading},
		{Start: "00:03", End: "00:04", Text: "僕"},
	}}
	store := songs.Store{Dir: dir}
	for _, p := range []string{store.TranscriptPath(songID), store.AnnotatedPath(songID)} {
		if err := songs.WriteFile(p, song); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(store.AudioDir(songID), 0o755)
	os.WriteFile(filepath.Join(store.AudioDir(songID), songs.ClipName("君", "slow")), []byte("RIFF"), 0o644)
	os.MkdirAll(filepath.Join(dir, "progress"), 0o755)
	os.WriteFile(filepath.Join(dir, "progress", songID+".json"),
		[]byte(`{"version":1,"lastLine":1,"lastStudiedAt":"2026-01-01T00:00:00.000Z","lines":{"僕":{"learned":true}}}`), 0o644)
	return New(cfg, progress.NewFileStore(filepath.Join(dir, "progress"))), cfg
}

func do(t *testing.T, s *Server, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if body != "" {
		req = httptest.NewRequest(method, path, stringsReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestListSongs(t *testing.T) {
	s, _ := newTestServer(t)
	rec := do(t, s, "GET", "/api/songs", "")
	var got []songSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	// The old text-keyed entry is read under its hash.
	if p := got[0].Progress; p.Learned != 1 || p.Total != 2 || p.LastLine != 1 || !got[0].HasTeacher {
		t.Errorf("summary = %+v", got[0])
	}
}

func TestGetSong(t *testing.T) {
	s, _ := newTestServer(t)
	rec := do(t, s, "GET", "/api/songs/"+songID, "")
	var got struct {
		Song     songs.Song
		Progress struct{ Lines map[string]any }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	l := got.Song.Lines[0]
	if l.ProgressKey != songs.TextHash("君") || l.Audio["normal"] != "/api/audio/"+songID+"/"+songs.ClipName("君", "normal") {
		t.Errorf("line = %+v", l)
	}
	if len(l.AudioCached) != 1 || l.AudioCached[0] != "slow" {
		t.Errorf("audioCached = %v", l.AudioCached)
	}
	if _, ok := got.Progress.Lines[songs.TextHash("僕")]; !ok {
		t.Errorf("progress = %+v", got.Progress)
	}

	if rec := do(t, s, "GET", "/api/songs/nope", ""); rec.Code != 404 {
		t.Errorf("unknown id: %d", rec.Code)
	}
}

// failingProgress fails the progress read of the listed songs.
type failingProgress struct {
	progress.Store
	fail map[string]bool
}

func (f failingProgress) Read(ctx context.Context, id string) (*progress.Progress, error) {
	if f.fail[id] {
		return nil, errors.New("firestore down")
	}
	return f.Store.Read(ctx, id)
}

func TestListSongsOrderAndFailingProgress(t *testing.T) {
	s, _ := newTestServer(t)
	// Songs never studied keep their directory order, after the studied one (abcdefghijk).
	extra := []string{"zzzzzzzzzz1", "aaaaaaaaaa1", "mmmmmmmmmm1", "bbbbbbbbbb1", "cccccccccc1", "dddddddddd1",
		"eeeeeeeeee1", "ffffffffff1", "gggggggggg1", "hhhhhhhhhh1", "iiiiiiiiii1"}
	for _, id := range extra {
		if err := songs.WriteFile(s.songs.AnnotatedPath(id), &songs.File{Language: "ja", Lines: []songs.Line{{Text: id}}}); err != nil {
			t.Fatal(err)
		}
	}
	s.progress = failingProgress{Store: s.progress, fail: map[string]bool{songID: true, "mmmmmmmmmm1": true}}

	rec := do(t, s, "GET", "/api/songs", "")
	var got []songSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || len(got) != len(extra)+1 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	want := append([]string(nil), extra...)
	want = append(want, songID)
	sort.Strings(want) // the directory order; nobody has progress, so no one is "studied"
	for i, g := range got {
		if g.ID != want[i] {
			t.Fatalf("order = %v, want %v", ids(got), want)
		}
	}
	for _, g := range got {
		if g.ID == songID && (g.Progress.Learned != 0 || g.Progress.Total != 2) {
			t.Errorf("failed progress read: %+v", g.Progress)
		}
	}
}

func ids(list []songSummary) []string {
	var out []string
	for _, g := range list {
		out = append(out, g.ID)
	}
	return out
}
