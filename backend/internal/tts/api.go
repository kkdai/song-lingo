// Package tts generates the teacher's read-aloud clips with Gemini TTS: one designed voice per
// language, each line read at a normal and a slow speed, cached as WAV files.
package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"log"

	"github.com/kkdai/song-lingo/backend/internal/apierr"
	"github.com/kkdai/song-lingo/backend/internal/gemini"
)

// Error and API are shared with the rest of the Gemini REST calls.
type (
	Error = apierr.Error
	API   = gemini.API
)

const DefaultAPIBase = gemini.DefaultAPIBase

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
	wav := make([]byte, 44+len(audio))
	le := binary.LittleEndian
	copy(wav[0:], "RIFF")
	le.PutUint32(wav[4:], uint32(36+len(audio)))
	copy(wav[8:], "WAVEfmt ")
	le.PutUint32(wav[16:], 16)                   // fmt chunk size
	le.PutUint16(wav[20:], 1)                    // PCM
	le.PutUint16(wav[22:], 1)                    // mono
	le.PutUint32(wav[24:], uint32(sampleRate))   // sample rate
	le.PutUint32(wav[28:], uint32(sampleRate*2)) // byte rate
	le.PutUint16(wav[32:], 2)                    // block align
	le.PutUint16(wav[34:], 16)                   // bits per sample
	copy(wav[36:], "data")
	le.PutUint32(wav[40:], uint32(len(audio)))
	copy(wav[44:], audio)
	return wav
}

// Synthesize reads one line aloud in the given voice and speaking style.
func Synthesize(ctx context.Context, a *API, model, voice, text, style string) ([]byte, error) {
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
