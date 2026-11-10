package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// fakeTTS stands in for the Gemini voices and interactions endpoints.
func fakeTTS(t *testing.T, s *Server, interactions func(w http.ResponseWriter)) *atomic.Int32 {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "k" {
			t.Errorf("missing key")
		}
		switch r.URL.Path {
		case "/voices":
			json.NewEncoder(w).Encode(map[string]any{"id": "voice-1"})
		case "/interactions":
			calls.Add(1)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if !strings.Contains(mustJSON(body), `"voice":"voice-1"`) {
				t.Errorf("body = %v", body)
			}
			interactions(w)
		}
	}))
	t.Cleanup(srv.Close)
	s.tts.API.Base = srv.URL
	s.tts.API.APIKey = func() (string, error) { return "k", nil }
	return &calls
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func audioOK(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(map[string]any{"steps": []any{map[string]any{
		"type": "model_output", "content": []any{map[string]any{"type": "audio", "data": base64.StdEncoding.EncodeToString([]byte("PCMDATA!"))}},
	}}})
}

func TestAudioGeneratesOnceAndCaches(t *testing.T) {
	s, cfg := newTestServer(t)
	calls := fakeTTS(t, s, audioOK)
	path := "/api/audio/" + songID + "/" + songs.ClipName("君", "normal")

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := do(t, s, "GET", path, ""); rec.Code != 200 || rec.Body.Len() != 52 {
				t.Errorf("%d %d", rec.Code, rec.Body.Len())
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Errorf("generated %d times", calls.Load())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "audio", songID, songs.ClipName("君", "normal"))); err != nil {
		t.Error("clip not cached")
	}
	raw, _ := os.ReadFile(filepath.Join(cfg.DataDir, "voices.json"))
	if !strings.Contains(string(raw), `"ja": "voice-1"`) {
		t.Errorf("voices.json = %s", raw)
	}

	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Range", "bytes=44-")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 206 || rec.Body.String() != "PCMDATA!" || rec.Header().Get("Content-Range") != "bytes 44-51/52" {
		t.Errorf("range: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
	if calls.Load() != 1 {
		t.Error("cached clip regenerated")
	}
}

func TestAudioFailuresAreNotRetried(t *testing.T) {
	s, _ := newTestServer(t)
	calls := fakeTTS(t, s, func(w http.ResponseWriter) {
		w.WriteHeader(500)
		w.Write([]byte(`[{"error":{"message":"boom"}}]`))
	})
	path := "/api/audio/" + songID + "/" + songs.ClipName("僕", "slow")
	for range 2 {
		if rec := do(t, s, "GET", path, ""); rec.Code != 502 || !strings.Contains(rec.Body.String(), "boom") {
			t.Errorf("%d %s", rec.Code, rec.Body)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("retried: %d calls", calls.Load())
	}
	if rec := do(t, s, "GET", path+"?error=1", ""); rec.Code != 502 || !strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("?error: %d %s", rec.Code, rec.Body)
	}
}

func TestAudioQuotaBlocksEveryClip(t *testing.T) {
	s, _ := newTestServer(t)
	calls := fakeTTS(t, s, func(w http.ResponseWriter) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"per day"}}`))
	})
	for _, text := range []string{"君", "僕"} {
		rec := do(t, s, "GET", "/api/audio/"+songID+"/"+songs.ClipName(text, "normal"), "")
		if rec.Code != 429 || !strings.Contains(rec.Body.String(), "TTS 額度已用完") {
			t.Errorf("%d %s", rec.Code, rec.Body)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("%d calls", calls.Load())
	}
}

func TestAudioRejectsBadNames(t *testing.T) {
	s, _ := newTestServer(t)
	for _, p := range []string{"/api/audio/" + songID + "/x.wav", "/api/audio/bad/" + songs.ClipName("君", "normal"),
		"/api/audio/" + songID + "/" + songs.ClipName("unknown", "normal")} {
		if rec := do(t, s, "GET", p, ""); rec.Code != 404 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
	if rec := do(t, s, "GET", "/api/audio/"+songID+"/x.wav?error=1", ""); rec.Code != 404 {
		t.Errorf("?error on bad name: %d", rec.Code)
	}
}
