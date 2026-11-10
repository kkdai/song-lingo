package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kkdai/song-lingo/backend/internal/config"
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
	return New(cfg, DefaultProgressStore(cfg)), cfg
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
