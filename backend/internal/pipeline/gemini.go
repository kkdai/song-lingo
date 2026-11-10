// Package pipeline turns a YouTube MV into a study-ready song: transcribe the lyrics, then
// annotate them (romanization, translation, word and grammar notes) with Gemini.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"google.golang.org/genai"
)

const DefaultModel = "gemini-3.8-flash"

// NewClient creates a Gemini API client.
func NewClient(ctx context.Context, apiKey string) (*genai.Client, error) {
	return genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
}

// BlockedError means Gemini refused to answer (finish_reason=RECITATION).
type BlockedError struct{ What string }

func (e *BlockedError) Error() string {
	return fmt.Sprintf("被 Gemini 擋下（finish_reason=RECITATION）：模型拒絕重現這些%s。", e.What)
}

// generateJSON asks for structured output matching schema and decodes it into out.
// It returns the finish reason.
func generateJSON(ctx context.Context, client *genai.Client, model string, parts []*genai.Part, schema map[string]any, out any) (string, error) {
	temperature := float32(0.2)
	resp, err := client.Models.GenerateContent(ctx, model, []*genai.Content{{Role: genai.RoleUser, Parts: parts}}, &genai.GenerateContentConfig{
		ResponseMIMEType:   "application/json",
		ResponseJsonSchema: schema,
		Temperature:        &temperature,
	})
	if err != nil {
		return "", err
	}
	finish := "NO_CANDIDATES"
	if len(resp.Candidates) > 0 {
		finish = string(resp.Candidates[0].FinishReason)
	}
	if finish == string(genai.FinishReasonRecitation) {
		return finish, errRecitation
	}
	if err := json.Unmarshal([]byte(resp.Text()), out); err != nil {
		return finish, fmt.Errorf("Gemini 沒有回傳可解析的結果（finish_reason=%s）", finish)
	}
	return finish, nil
}

var errRecitation = fmt.Errorf("recitation")

// JSON Schema helpers.
func object(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

func str(description string) map[string]any {
	s := map[string]any{"type": "string"}
	if description != "" {
		s["description"] = description
	}
	return s
}

func nullableStr(description string) map[string]any {
	s := map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}}}
	if description != "" {
		s["description"] = description
	}
	return s
}

func array(items map[string]any, description string) map[string]any {
	s := map[string]any{"type": "array", "items": items}
	if description != "" {
		s["description"] = description
	}
	return s
}

// LazyClient creates the Gemini client on first use (so the server starts without an API key)
// and reuses it afterwards.
func LazyClient(apiKey func() (string, error)) func(ctx context.Context) (*genai.Client, error) {
	var mu sync.Mutex
	var client *genai.Client
	return func(ctx context.Context) (*genai.Client, error) {
		mu.Lock()
		defer mu.Unlock()
		if client != nil {
			return client, nil
		}
		key, err := apiKey()
		if err != nil {
			return nil, err
		}
		c, err := NewClient(ctx, key)
		if err != nil {
			return nil, err
		}
		client = c
		return client, nil
	}
}
