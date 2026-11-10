package pipeline

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"google.golang.org/genai"

	"github.com/kkdai/song-lingo/backend/internal/songs"
)

const transcribePrompt = `You are a professional lyric transcriber. Listen to the vocals in this music video
and transcribe the sung lyrics line by line, with timestamps.

Rules:
- If the video shows on-screen lyrics/subtitles in the original language, use them as the
  primary source (they give the correct kanji/spelling). Otherwise transcribe from the audio.
- Write lyrics in the original script (e.g. Japanese kanji/kana, Korean hangul). Do not translate.
- For Japanese lines, fill ` + "`reading`" + ` with the full hiragana reading of the line.
- One entry per sung line/phrase. Repeat choruses every time they are sung.
- Exclude non-lyric content: intros, spoken ads, credits, instrumental-only sections.
- Set ` + "`uncertain`" + ` to true for any line you could not hear clearly or had to guess.
- Timestamps are MM:SS relative to the start of the video.
`

var transcriptSchema = object(map[string]any{
	"language":     str("ISO 639-1 code of the main sung language, e.g. ja, ko"),
	"title_guess":  nullableStr(""),
	"artist_guess": nullableStr(""),
	"source":       str("'onscreen' if on-screen lyrics were used, 'audio' otherwise, or 'mixed'"),
	"lines": array(object(map[string]any{
		"start":     str("Start time, MM:SS"),
		"end":       str("End time, MM:SS"),
		"text":      str("Lyric line in original script"),
		"reading":   nullableStr("Hiragana reading (Japanese only)"),
		"section":   nullableStr("e.g. verse, pre-chorus, chorus, bridge"),
		"uncertain": map[string]any{"type": "boolean"},
	}, "start", "end", "text"), ""),
}, "language", "source", "lines")

var youtubeID = regexp.MustCompile(`(?:v=|youtu\.be/|/shorts/|/embed/|/live/)([\w-]{11})`)

// ParseVideoID accepts a YouTube URL or a bare 11-character video ID.
func ParseVideoID(url string) (string, bool) {
	url = strings.TrimSpace(url)
	if songs.IsVideoID(url) {
		return url, true
	}
	m := youtubeID.FindStringSubmatch(url)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Transcribe has Gemini watch the MV and write down the lyrics with timestamps.
func Transcribe(ctx context.Context, client *genai.Client, model, videoID string) (*songs.File, error) {
	var transcript songs.File
	parts := []*genai.Part{
		{FileData: &genai.FileData{FileURI: "https://www.youtube.com/watch?v=" + videoID}},
		{Text: transcribePrompt},
	}
	if _, err := generateJSON(ctx, client, model, parts, transcriptSchema, &transcript); err != nil {
		if errors.Is(err, errRecitation) {
			return nil, &BlockedError{What: "歌詞"}
		}
		return nil, err
	}
	if transcript.Lines == nil {
		transcript.Lines = []songs.Line{}
	}
	return &transcript, nil
}
