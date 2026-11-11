package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// batchService is a Service whose "ja" voice is known and whose interactions endpoint is handler.
func batchService(t *testing.T, handler http.HandlerFunc) (*Service, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	s := newTestService(t)
	s.Cfg.Styles = map[string]string{"normal": "n", "slow": "s"}
	s.API = &API{Base: srv.URL, APIKey: func() (string, error) { return "k", nil }}
	if err := s.storeVoice("ja", "voice-1"); err != nil {
		t.Fatal(err)
	}
	return s, &calls
}

func audioResponse(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{"steps": []any{map[string]any{
		"type": "model_output", "content": []any{map[string]any{"type": "audio", "data": base64.StdEncoding.EncodeToString([]byte("PCM"))}},
	}}})
}

func TestGenerateAllSkipsCachedClips(t *testing.T) {
	s, calls := batchService(t, audioResponse)
	song := &songs.File{Language: "ja", Lines: []songs.Line{{Text: "a"}, {Text: "b"}, {Text: "a"}}}
	dir := t.TempDir()
	cached := filepath.Join(dir, songs.ClipName("a", "slow"))
	os.WriteFile(cached, []byte("RIFF"), 0o644)

	var logs atomic.Int32
	res, err := s.GenerateAll(context.Background(), song, dir, 2, func(f string, a ...any) { logs.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	// 2 unique texts x 2 speeds = 4 clips, one cached.
	if res.Unique != 2 || res.Total != 4 || res.Cached != 1 || res.Generated != 3 || len(res.Failures) != 0 || calls.Load() != 3 {
		t.Errorf("result = %+v, calls = %d", res, calls.Load())
	}
	if got, _ := os.ReadFile(cached); string(got) != "RIFF" {
		t.Error("cached clip overwritten")
	}
	for _, text := range []string{"a", "b"} {
		for _, speed := range []string{"normal", "slow"} {
			if _, err := os.Stat(filepath.Join(dir, songs.ClipName(text, speed))); err != nil {
				t.Errorf("missing clip %s/%s", text, speed)
			}
		}
	}
	if logs.Load() == 0 {
		t.Error("no progress logged")
	}
}

func TestGenerateAllStopsOnDailyQuota(t *testing.T) {
	s, calls := batchService(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"quota exceeded per day"}}`))
	})
	var lines []songs.Line
	for _, text := range []string{"a", "b", "c", "d", "e", "f"} {
		lines = append(lines, songs.Line{Text: text})
	}
	res, err := s.GenerateAll(context.Background(), &songs.File{Language: "ja", Lines: lines}, t.TempDir(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DailyQuota || res.Generated != 0 || len(res.Failures) != 12 || calls.Load() != 1 {
		t.Errorf("result = %+v, calls = %d", res, calls.Load())
	}
}

func TestGenerateAllStopsWhenContextEnds(t *testing.T) {
	s, calls := batchService(t, audioResponse)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := s.GenerateAll(ctx, &songs.File{Language: "ja", Lines: []songs.Line{{Text: "a"}}}, t.TempDir(), 2, nil)
	if err != nil || res.Generated != 0 || len(res.Failures) == 0 || calls.Load() != 0 {
		t.Errorf("result = %+v, err = %v, calls = %d", res, err, calls.Load())
	}
}
