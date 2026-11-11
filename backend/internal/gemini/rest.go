// Package gemini calls the Gemini REST endpoints the Go SDK doesn't cover (voices, interactions).
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kkdai/song-lingo/backend/internal/apierr"
)

const (
	DefaultAPIBase = "https://generativelanguage.googleapis.com/v1beta"
	requestTimeout = 60 * time.Second
	// DefaultRetryAfter is the backoff for a 429 that doesn't say when the quota frees up.
	DefaultRetryAfter = time.Minute
)

// Error is the error type shared by all packages (see apierr).
type Error = apierr.Error

// API is a small client for the Gemini REST API.
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
			e := &Error{Status: 429, Message: "額度已用完：" + message, RetryAt: time.Now().Add(DefaultRetryAfter),
				Daily: strings.Contains(message, "per day")}
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
