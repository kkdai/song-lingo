package tts

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/config"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	cfg := &config.Config{DataDir: t.TempDir(), Speeds: []string{"normal", "slow"}}
	return NewService(cfg, songs.Store{Dir: cfg.DataDir}, nil)
}

func TestRecordFailurePrunesExpired(t *testing.T) {
	s := newTestService(t)
	s.failures["old"] = failure{err: &Error{Status: 502}, until: time.Now().Add(-time.Second)}
	s.recordFailure("new", &Error{Status: 502, Message: "boom"})
	if _, ok := s.failures["old"]; ok {
		t.Error("expired failure not pruned")
	}
	if f, ok := s.failures["new"]; !ok || f.err.Message != "boom" {
		t.Errorf("new failure missing: %+v", f)
	}
}

func TestLastError(t *testing.T) {
	s := newTestService(t)
	id, file := "dQw4w9WgXcQ", "0123456789abcdef_slow.wav"
	path := s.Songs.AudioDir(id) + "/" + file
	if _, ok := s.LastError(id, file); ok {
		t.Fatal("unexpected error")
	}
	s.recordFailure(path, &Error{Status: 502, Message: "boom"})
	if e, ok := s.LastError(id, file); !ok || e.Message != "boom" {
		t.Errorf("LastError = %v, %v", e, ok)
	}
	s.failures[path] = failure{err: &Error{Status: 502}, until: time.Now().Add(-time.Second)}
	if _, ok := s.LastError(id, file); ok {
		t.Error("expired error reported")
	}
	if _, ok := s.failures[path]; ok {
		t.Error("expired error kept")
	}
}

func TestQuotaFailureLastsUntilRetryAt(t *testing.T) {
	s := newTestService(t)
	s.recordFailure("p", &Error{Status: 429, RetryAt: time.Now().Add(time.Hour)})
	if f := s.failures["p"]; time.Until(f.until) < 30*time.Minute {
		t.Errorf("quota failure expires too early: %v", time.Until(f.until))
	}
}

func TestWithSlotBoundsConcurrency(t *testing.T) {
	s := newTestService(t)
	var running, peak atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.withSlot(context.Background(), func() ([]byte, error) {
				n := running.Add(1)
				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				running.Add(-1)
				return nil, nil
			})
		}()
	}
	wg.Wait()
	if got := peak.Load(); got > MaxConcurrentGenerations || got < 2 {
		t.Errorf("peak concurrency = %d, want 2..%d", got, MaxConcurrentGenerations)
	}
}

func TestWithSlotRespectsContext(t *testing.T) {
	s := newTestService(t)
	for range MaxConcurrentGenerations {
		s.slots <- struct{}{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	_, err := s.withSlot(ctx, func() ([]byte, error) { called = true; return nil, nil })
	if err != context.Canceled || called {
		t.Errorf("err = %v, called = %v", err, called)
	}
}

func TestVoicesCachedInMemory(t *testing.T) {
	s := newTestService(t)
	if err := s.storeVoice("ja", "voice-1"); err != nil {
		t.Fatal(err)
	}
	// A fresh service reads the file once; later reads come from memory.
	s2 := NewService(s.Cfg, s.Songs, nil)
	if id, err := s2.TeacherVoice(context.Background(), "ja"); err != nil || id != "voice-1" {
		t.Fatalf("voice = %q, %v", id, err)
	}
	if err := songs.WriteAtomic(s2.voicesFile(), []byte(`{"ja":"changed"}`)); err != nil {
		t.Fatal(err)
	}
	if id, _ := s2.TeacherVoice(context.Background(), "ja"); id != "voice-1" {
		t.Errorf("voice re-read from disk: %q", id)
	}
}

func TestLastErrorFallsBackToQuota(t *testing.T) {
	s := newTestService(t)
	id := "dQw4w9WgXcQ"
	s.recordFailure(s.Songs.AudioDir(id)+"/0123456789abcdef_slow.wav", &Error{Status: 429, Message: "quota", RetryAt: time.Now().Add(time.Hour)})
	if e, ok := s.LastError(id, "fedcba9876543210_normal.wav"); !ok || e.Status != 429 {
		t.Errorf("clip B during quota block: %v, %v", e, ok)
	}
	s.quota.RetryAt = time.Now().Add(-time.Second)
	if e, ok := s.LastError(id, "fedcba9876543210_normal.wav"); ok {
		t.Errorf("expired quota still reported: %v", e)
	}
}
