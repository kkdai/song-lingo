package progress

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestWithDefaultsMergesOldKeys(t *testing.T) {
	var p Progress
	json.Unmarshal([]byte(`{"lines":{
		"君":{"shadow":[{"at":"2026-01-02","score":50,"missed":[]}]},
		"`+LineKey("君")+`":{"learned":true,"shadow":[{"at":"2026-01-01","score":80,"missed":["君"]}]}}}`), &p)
	got := withDefaults(&p)
	e := got.Lines[LineKey("君")]
	if len(got.Lines) != 1 || !e.Learned || len(e.Shadow) != 2 || e.Shadow[0].Score != 80 || got.Version != 1 {
		t.Errorf("got %+v %+v", got, e)
	}
}

func TestAddAttemptKeepsLatest(t *testing.T) {
	p := Empty()
	for i := range MaxAttempts + 5 {
		p.AddAttempt("x", ShadowAttempt{At: fmt.Sprint(i), Score: i})
	}
	s := p.Lines[LineKey("x")].Shadow
	if len(s) != MaxAttempts || s[0].Score != 5 || s[0].Missed == nil {
		t.Errorf("got %d attempts, first %+v", len(s), s[0])
	}
}

func TestFileStoreUpdateIsSerialized(t *testing.T) {
	s := NewFileStore(t.TempDir())
	ctx := context.Background()
	done := make(chan struct{})
	for i := range 20 {
		go func() {
			s.Update(ctx, "v", func(p *Progress) { p.AddAttempt(fmt.Sprint(i), ShadowAttempt{}) })
			done <- struct{}{}
		}()
	}
	for range 20 {
		<-done
	}
	p, _ := s.Read(ctx, "v")
	if len(p.Lines) != 20 {
		t.Errorf("lost updates: %d lines", len(p.Lines))
	}
}
