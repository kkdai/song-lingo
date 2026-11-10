// Package tts generates the teacher's read-aloud clips with Gemini TTS: one designed voice per
// language, each line read at a normal and a slow speed, cached as WAV files.
package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultAPIBase = "https://generativelanguage.googleapis.com/v1beta"
	requestTimeout = 60 * time.Second
)

// Error is a failure to report to the user with an HTTP status.
type Error struct {
	Status  int
	Message string
	// For 429s: when the API says the quota frees up again (zero if unknown).
	RetryAt time.Time
}

func (e *Error) Error() string { return e.Message }

// API calls the Gemini REST endpoints the Go SDK doesn't cover (voices, interactions).
type API struct {
	Base   string
	APIKey func() (string, error)
	HTTP   *http.Client
}

func (a *API) Call(ctx context.Context, method, endpoint string, body any) (map[string]any, error) {
	key, err := a.APIKey()
	if err != nil {
		return nil, &Error{Status: 500, Message: err.Error()}
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, a.Base+"/"+endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", key)
	req.Header.Set("Content-Type", "application/json")
	client := a.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &Error{Status: 504, Message: "Gemini 回應逾時，請再試一次。"}
		}
		return nil, &Error{Status: 502, Message: fmt.Sprintf("無法連線到 Gemini：%v", err)}
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := errorMessage(raw)
		if message == "" {
			message = res.Status
		}
		if res.StatusCode == http.StatusTooManyRequests {
			e := &Error{Status: 429, Message: "額度已用完：" + message}
			if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s > 0 {
				e.RetryAt = time.Now().Add(time.Duration(s) * time.Second)
			}
			return nil, e
		}
		return nil, &Error{Status: 502, Message: fmt.Sprintf("Gemini 回傳錯誤 %d：%s", res.StatusCode, message)}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &Error{Status: 502, Message: "Gemini 回傳的內容無法解析。"}
	}
	return out, nil
}

// errorMessage reads {error: {message}} or [{error: {message}}].
func errorMessage(raw []byte) string {
	var one struct {
		Error struct{ Message string } `json:"error"`
	}
	if json.Unmarshal(raw, &one) == nil && one.Error.Message != "" {
		return one.Error.Message
	}
	var many []struct {
		Error struct{ Message string } `json:"error"`
	}
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return many[0].Error.Message
	}
	return ""
}

// ExtractAudio pulls base64 audio out of an interactions response. The REST payload carries it in
// steps[].content[] (type "audio") of the last model_output steps, or in legacy outputs[];
// output_audio is a convenience field some SDKs derive, kept as a fallback.
func ExtractAudio(resp map[string]any) string {
	fromContent := func(content any) string {
		items, _ := content.([]any)
		for i := len(items) - 1; i >= 0; i-- {
			item, _ := items[i].(map[string]any)
			if data, _ := item["data"].(string); item["type"] == "audio" && data != "" {
				return data
			}
		}
		return ""
	}
	steps, _ := resp["steps"].([]any)
	for i := len(steps) - 1; i >= 0; i-- {
		step, _ := steps[i].(map[string]any)
		if step["type"] == "user_input" {
			break
		}
		if step["type"] != "model_output" {
			continue
		}
		if data := fromContent(step["content"]); data != "" {
			return data
		}
	}
	if data := fromContent(resp["outputs"]); data != "" {
		return data
	}
	if audio, ok := resp["output_audio"].(map[string]any); ok {
		data, _ := audio["data"].(string)
		return data
	}
	return ""
}

// ToWAV wraps raw 16-bit mono PCM in a WAV header (in case the API returns headerless audio).
func ToWAV(audio []byte, sampleRate int) []byte {
	if bytes.HasPrefix(audio, []byte("RIFF")) {
		return audio
	}
	var h bytes.Buffer
	h.WriteString("RIFF")
	binary.Write(&h, binary.LittleEndian, uint32(36+len(audio)))
	h.WriteString("WAVEfmt ")
	binary.Write(&h, binary.LittleEndian, uint32(16))
	binary.Write(&h, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&h, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&h, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&h, binary.LittleEndian, uint32(sampleRate*2))
	binary.Write(&h, binary.LittleEndian, uint16(2))
	binary.Write(&h, binary.LittleEndian, uint16(16))
	h.WriteString("data")
	binary.Write(&h, binary.LittleEndian, uint32(len(audio)))
	return append(h.Bytes(), audio...)
}

// Synthesize reads one line aloud in the given voice and speaking style.
func (a *API) Synthesize(ctx context.Context, model, voice, text, style string) ([]byte, error) {
	resp, err := a.Call(ctx, "POST", "interactions", map[string]any{
		"model": model,
		"input": []any{map[string]any{
			"type": "user_input",
			"content": []any{map[string]any{
				"type": "text", "text": text,
				"annotations": []any{map[string]any{"type": "speech_metadata", "style": style}},
			}},
		}},
		"response_format":   map[string]any{"type": "audio"},
		"generation_config": map[string]any{"speech_config": []any{map[string]any{"voice": voice}}},
	})
	if err != nil {
		return nil, err
	}
	data := ExtractAudio(resp)
	if data == "" {
		keys := make([]string, 0, len(resp))
		for k := range resp {
			keys = append(keys, k)
		}
		log.Printf("[tts] no audio in interactions response; top-level keys: %v", keys)
		return nil, &Error{Status: 502, Message: "Gemini 沒有回傳音訊。"}
	}
	audio, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, &Error{Status: 502, Message: "Gemini 回傳的音訊無法解碼。"}
	}
	return ToWAV(audio, 24_000), nil
}
