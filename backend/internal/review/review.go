// Package review applies edits from the review UI to a song's transcript and annotated file.
package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/kkdai/song-lingo/backend/internal/apierr"
	"github.com/kkdai/song-lingo/backend/internal/progress"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

const maxFieldLength = 500

// Error is the shared user-facing error (see apierr).
type Error = apierr.Error

func errorf(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

type Edit struct {
	Text          *string
	Reading       *string
	TranslationZh *string
	Reviewed      *bool
	// Apply to every line with the same original text (repeated choruses).
	ApplyToSame bool
}

// DecodeObject decodes a JSON object body into its fields.
func DecodeObject(r io.Reader) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.NewDecoder(r).Decode(&fields) != nil || fields == nil {
		return nil, false
	}
	return fields, true
}

// cleanString reads an optional string field; null or another type is an error.
func cleanString(fields map[string]json.RawMessage, key, field string) (*string, error) {
	raw, present := fields[key]
	if !present {
		return nil, nil
	}
	var s string
	// Lengths count UTF-16 code units, as the JavaScript version did.
	if IsNull(raw) || json.Unmarshal(raw, &s) != nil || len(utf16.Encode([]rune(s))) > maxFieldLength {
		return nil, errorf(400, "%s 格式不正確。", field)
	}
	s = strings.TrimSpace(s)
	return &s, nil
}

func ParseEdit(body io.Reader) (Edit, error) {
	fields, ok := DecodeObject(body)
	if !ok {
		return Edit{}, errorf(400, "請求格式不正確。")
	}
	var edit Edit
	var err error
	if edit.Text, err = cleanString(fields, "text", "歌詞"); err != nil {
		return edit, err
	}
	if edit.Text != nil && *edit.Text == "" {
		return edit, errorf(400, "歌詞不能是空的。")
	}
	if edit.Reading, err = cleanString(fields, "reading", "讀音"); err != nil {
		return edit, err
	}
	if edit.TranslationZh, err = cleanString(fields, "translation_zh", "翻譯"); err != nil {
		return edit, err
	}
	if raw, present := fields["reviewed"]; present {
		var b bool
		if IsNull(raw) || json.Unmarshal(raw, &b) != nil {
			return edit, errorf(400, "reviewed 格式不正確。")
		}
		edit.Reviewed = &b
	}
	var applyToSame bool
	_ = json.Unmarshal(fields["applyToSame"], &applyToSame)
	edit.ApplyToSame = applyToSame
	return edit, nil
}

type Service struct {
	Songs    songs.Store
	Progress progress.Store
}

func readFile(path string) (*songs.File, error) {
	f, err := songs.ReadFile(path)
	if errors.Is(err, songs.ErrNotFound) {
		return nil, errorf(404, "找不到歌曲檔案。")
	}
	return f, err
}

// EditLine applies a review edit to both the transcript (the source for annotation) and the
// annotated file the web app reads. Text/reading changes mark the annotation stale until the song
// is re-annotated; manual translations and the reviewed flag survive re-annotation.
// It returns how many lines were changed.
func (s *Service) EditLine(ctx context.Context, videoID string, index int, edit Edit) (int, error) {
	defer songs.Lock(videoID)()
	transcriptPath, annotatedPath := s.Songs.TranscriptPath(videoID), s.Songs.AnnotatedPath(videoID)
	transcript, err := readFile(transcriptPath)
	if err != nil {
		return 0, err
	}
	annotated, err := readFile(annotatedPath)
	if err != nil {
		return 0, err
	}
	if len(transcript.Lines) != len(annotated.Lines) {
		return 0, errorf(409, "轉錄檔和標註檔的句數不一致，請重新分析整首歌。")
	}
	if index < 0 || index >= len(annotated.Lines) {
		return 0, errorf(404, "找不到這一句。")
	}

	original := annotated.Lines[index].Text
	targets := []int{index}
	if edit.ApplyToSame {
		targets = nil
		for i, l := range annotated.Lines {
			if l.Text == original {
				targets = append(targets, i)
			}
		}
	}

	for _, i := range targets {
		t, a := &transcript.Lines[i], &annotated.Lines[i]
		if edit.Text != nil && *edit.Text != a.Text {
			t.Text, a.Text = *edit.Text, *edit.Text
			a.Stale = true
		}
		if edit.Reading != nil && *edit.Reading != a.ReadingOrEmpty() {
			var reading *string
			if *edit.Reading != "" {
				reading = edit.Reading
			}
			t.Reading, a.Reading = reading, reading
			a.Stale = true
		}
		if v := edit.TranslationZh; v != nil && *v != deref(a.TranslationZh) {
			if *v != "" {
				t.TranslationZhManual, a.TranslationZhManual = *v, *v
				a.TranslationZh = v
			} else {
				t.TranslationZhManual, a.TranslationZhManual = "", ""
			}
		}
		if edit.Reviewed != nil {
			t.Reviewed, a.Reviewed = edit.Reviewed, edit.Reviewed
			if *edit.Reviewed {
				t.Uncertain, a.Uncertain = false, false
				no := false
				a.NeedsReview = &no
			}
		}
	}

	// Copy progress before writing the files: the old entry is kept, so if a write below fails the
	// edit can simply be retried, whereas copying afterwards would lose the old text on a retry.
	if edit.Text != nil {
		if err := progress.RenameLine(ctx, s.Progress, videoID, original, *edit.Text); err != nil {
			return 0, err
		}
	}
	if err := songs.WriteFile(transcriptPath, transcript); err != nil {
		return 0, err
	}
	if err := songs.WriteFile(annotatedPath, annotated); err != nil {
		return 0, err
	}
	return len(targets), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// IsNull reports a JSON null, which Unmarshal would otherwise accept for any type.
func IsNull(raw json.RawMessage) bool { return raw == nil || string(raw) == "null" }
