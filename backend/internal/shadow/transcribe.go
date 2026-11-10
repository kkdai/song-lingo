package shadow

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"

	"google.golang.org/genai"

	"github.com/kkdai/song-lingo/backend/internal/gemini"
)

const Model = "gemini-3.5-transcribe"

var languageCodes = map[string]string{"ja": "ja-JP", "ko": "ko-KR", "en": "en-US"}

type Transcriber struct {
	Client func(ctx context.Context) (*genai.Client, error)
	API    *gemini.API
}

// Transcribe uploads the recording, has gemini-3.5-transcribe write down what was said (verbatim,
// not a cleaned-up guess) and deletes the upload again. The recording is never stored.
func (t *Transcriber) Transcribe(ctx context.Context, audio []byte, mimeType, language string) (string, error) {
	client, err := t.Client(ctx)
	if err != nil {
		return "", err
	}
	file, err := client.Files.Upload(ctx, bytes.NewReader(audio), &genai.UploadFileConfig{MIMEType: mimeType})
	if err != nil {
		var apiErr genai.APIError
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusTooManyRequests {
			return "", &gemini.Error{Status: 429, Message: "額度已用完：" + apiErr.Message}
		}
		return "", err
	}
	defer func() {
		if _, err := client.Files.Delete(context.WithoutCancel(ctx), file.Name, nil); err != nil {
			log.Printf("[shadow] could not delete uploaded recording %s: %v", file.Name, err)
		}
	}()

	code := languageCodes[language]
	if code == "" {
		code = "en-US"
	}
	resp, err := t.API.Call(ctx, "POST", "interactions", map[string]any{
		"model": Model,
		"input": []any{map[string]any{"type": "audio", "uri": file.URI, "mime_type": file.MIMEType}},
		"generation_config": map[string]any{
			"transcription_config": map[string]any{
				"language_codes": []string{code},
				"mode":           map[string]any{"type": "verbatim", "timestamp_granularities": []string{"word"}},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return outputText(resp), nil
}

// outputText joins the text of the model_output steps.
func outputText(resp map[string]any) string {
	var out bytes.Buffer
	steps, _ := resp["steps"].([]any)
	for _, s := range steps {
		step, _ := s.(map[string]any)
		if step["type"] != "model_output" {
			continue
		}
		content, _ := step["content"].([]any)
		for _, c := range content {
			item, _ := c.(map[string]any)
			if text, _ := item["text"].(string); item["type"] == "text" {
				out.WriteString(text)
			}
		}
	}
	return out.String()
}
