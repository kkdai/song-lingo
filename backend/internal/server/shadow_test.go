package server

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/gemini"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

type fakeTranscriber struct {
	text  string
	err   error
	calls int
	got   []byte
	mime  string
}

func (f *fakeTranscriber) Transcribe(_ context.Context, audio []byte, mimeType, language string) (string, error) {
	f.calls++
	f.got, f.mime = audio, mimeType
	return f.text, f.err
}

func postRecording(t *testing.T, s *Server, index string, contentType string, size int) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="audio"; filename="blob"`)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	part, _ := mw.CreatePart(h)
	part.Write(bytes.Repeat([]byte{1}, size))
	mw.Close()
	req := httptest.NewRequest("POST", "/api/songs/"+songID+"/lines/"+index+"/shadow", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestShadowScoresAndSaves(t *testing.T) {
	s, _ := newTestServer(t)
	fake := &fakeTranscriber{text: "きみ"}
	s.shadow = fake
	rec := postRecording(t, s, "0", "audio/mp4;codecs=mp4a", 2000)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"score":100`) || !strings.Contains(rec.Body.String(), `"saved":true`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if fake.mime != "audio/m4a" || len(fake.got) != 2000 {
		t.Errorf("sent %s, %d bytes", fake.mime, len(fake.got))
	}
	p, _ := s.progress.Read(context.Background(), songID)
	if a := p.Lines[songs.TextHash("君")].Shadow; len(a) != 1 || a[0].Score != 100 || p.LastLine != 0 {
		t.Errorf("progress = %+v", p.Lines)
	}
}

func TestShadowRejects(t *testing.T) {
	s, _ := newTestServer(t)
	fake := &fakeTranscriber{text: "x"}
	s.shadow = fake
	for _, c := range []struct {
		index, contentType string
		size, code         int
	}{
		{"0", "audio/webm", 10, 422},
		{"0", "audio/webm", 6 * 1024 * 1024, 413},
		{"0", "video/mp4", 2000, 415},
		{"0", "", 2000, 415},
		{"7", "audio/webm", 2000, 404},
		{"x", "audio/webm", 2000, 404},
	} {
		if rec := postRecording(t, s, c.index, c.contentType, c.size); rec.Code != c.code {
			t.Errorf("%+v: %d %s", c, rec.Code, rec.Body)
		}
	}
	if fake.calls != 0 {
		t.Errorf("transcribed %d times", fake.calls)
	}

	fake.text = "  "
	if rec := postRecording(t, s, "0", "audio/webm", 2000); rec.Code != 422 || !strings.Contains(rec.Body.String(), "沒有聽到聲音") {
		t.Errorf("silence: %d %s", rec.Code, rec.Body)
	}
	fake.err = errors.New("boom\nmore")
	if rec := postRecording(t, s, "0", "audio/webm", 2000); rec.Code != 502 || !strings.Contains(rec.Body.String(), "評分失敗：boom\"") {
		t.Errorf("failure: %d %s", rec.Code, rec.Body)
	}
}

func TestShadowQuotaStopsCalls(t *testing.T) {
	s, _ := newTestServer(t)
	fake := &fakeTranscriber{err: &gemini.Error{Status: 429, Message: "quota", RetryAt: time.Now().Add(10 * time.Minute)}}
	s.shadow = fake
	if rec := postRecording(t, s, "0", "audio/webm", 2000); rec.Code != 429 {
		t.Fatalf("%d", rec.Code)
	}
	rec := postRecording(t, s, "0", "audio/webm", 2000)
	if rec.Code != 429 || !strings.Contains(rec.Body.String(), "大約 10 分鐘後") || fake.calls != 1 {
		t.Errorf("%d %s, %d calls", rec.Code, rec.Body, fake.calls)
	}
}
