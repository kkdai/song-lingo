package server

import (
	"context"
	"strings"
	"testing"

	"github.com/kkdai/song-lingo/backend/internal/progress"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

func TestPatchProgress(t *testing.T) {
	s, cfg := newTestServer(t)
	path := "/api/songs/" + songID + "/progress"
	if rec := do(t, s, "PATCH", path, `{"lastLine":0,"learned":{"index":0,"value":true}}`); rec.Code != 204 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	p, _ := s.progress.Read(context.Background(), songID)
	if p.LastLine != 0 || !p.Lines[songs.TextHash("君")].Learned || p.LastStudiedAt == nil {
		t.Errorf("progress = %+v", p)
	}
	// The migrated entry was written back under its hash.
	if !p.Lines[songs.TextHash("僕")].Learned {
		t.Errorf("old entry lost: %+v", p.Lines)
	}

	do(t, s, "PATCH", path, `{"learned":{"index":0,"value":false}}`)
	if p, _ := s.progress.Read(context.Background(), songID); p.Lines[songs.TextHash("君")].Learned {
		t.Error("learned not cleared")
	}

	for _, body := range []string{`null`, `[]`, `{"lastLine":2}`, `{"lastLine":0.5}`, `{"lastLine":null}`, `{"learned":{"index":0}}`, `{"learned":null}`, `{"lastLine":0,"learned":{"index":0,"value":null}}`} {
		if rec := do(t, s, "PATCH", path, body); rec.Code != 400 {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	_ = cfg
}

func TestPatchLine(t *testing.T) {
	s, _ := newTestServer(t)
	path := "/api/songs/" + songID + "/lines/0"
	do(t, s, "PATCH", "/api/songs/"+songID+"/progress", `{"learned":{"index":0,"value":true}}`)

	rec := do(t, s, "PATCH", path, `{"text":" 君よ ","reading":"","translation_zh":"你啊","reviewed":true}`)
	if rec.Code != 200 || rec.Body.String() != "{\"updated\":1}\n" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	a, _ := songs.ReadFile(s.songs.AnnotatedPath(songID))
	tr, _ := songs.ReadFile(s.songs.TranscriptPath(songID))
	l := a.Lines[0]
	if l.Text != "君よ" || !l.Stale || l.Reading != nil || *l.TranslationZh != "你啊" || l.TranslationZhManual != "你啊" ||
		!*l.Reviewed || *l.NeedsReview || l.Uncertain {
		t.Errorf("annotated = %+v", l)
	}
	if tl := tr.Lines[0]; tl.Text != "君よ" || tl.TranslationZhManual != "你啊" || !*tl.Reviewed || tl.Stale {
		t.Errorf("transcript = %+v", tl)
	}
	p, _ := s.progress.Read(context.Background(), songID)
	if !p.Lines[progress.LineKey("君よ")].Learned || !p.Lines[progress.LineKey("君")].Learned {
		t.Errorf("progress not carried over: %+v", p.Lines)
	}

	// Clearing the translation drops the manual override.
	do(t, s, "PATCH", path, `{"translation_zh":""}`)
	if a, _ := songs.ReadFile(s.songs.AnnotatedPath(songID)); a.Lines[0].TranslationZhManual != "" {
		t.Error("manual translation kept")
	}

	for body, code := range map[string]int{`{"text":"  "}`: 400, `{"reviewed":"yes"}`: 400, `{"reviewed":null}`: 400, `{"text":null}`: 400, `{"text":1}`: 400, `x`: 400} {
		if rec := do(t, s, "PATCH", path, body); rec.Code != code {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	if rec := do(t, s, "PATCH", "/api/songs/"+songID+"/lines/9", `{}`); rec.Code != 404 {
		t.Errorf("bad index: %d", rec.Code)
	}
}

func TestPatchLineApplyToSame(t *testing.T) {
	s, _ := newTestServer(t)
	for _, p := range []string{s.songs.TranscriptPath(songID), s.songs.AnnotatedPath(songID)} {
		f, _ := songs.ReadFile(p)
		f.Lines = append(f.Lines, f.Lines[0])
		songs.WriteFile(p, f)
	}
	rec := do(t, s, "PATCH", "/api/songs/"+songID+"/lines/0", `{"text":"君だ","applyToSame":true}`)
	if rec.Body.String() != "{\"updated\":2}\n" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestOversizedJSONBodyIs413(t *testing.T) {
	s, _ := newTestServer(t)
	big := `{"text":"` + strings.Repeat("a", maxJSONBody) + `"}`
	for _, path := range []string{"/api/songs/" + songID + "/lines/0", "/api/songs/" + songID + "/progress"} {
		if rec := do(t, s, "PATCH", path, big); rec.Code != 413 {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
	if rec := do(t, s, "POST", "/api/songs", `{"url":"`+strings.Repeat("a", maxJSONBody)+`"}`); rec.Code != 413 {
		t.Errorf("POST /api/songs: %d", rec.Code)
	}
}
