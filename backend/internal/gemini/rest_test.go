package gemini

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func call429(t *testing.T, retryAfter string) *Error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"quota"}}`))
	}))
	defer srv.Close()
	api := &API{Base: srv.URL, APIKey: func() (string, error) { return "k", nil }}
	_, err := api.Call(context.Background(), "POST", "x", nil)
	var e *Error
	if !errors.As(err, &e) || e.Status != 429 {
		t.Fatalf("err = %v", err)
	}
	return e
}

func TestRateLimitDefaultsRetryAt(t *testing.T) {
	e := call429(t, "")
	if d := time.Until(e.RetryAt); d < DefaultRetryAfter-5*time.Second || d > DefaultRetryAfter {
		t.Errorf("RetryAt in %v, want about %v", d, DefaultRetryAfter)
	}
}

func TestRateLimitUsesRetryAfter(t *testing.T) {
	e := call429(t, "120")
	if d := time.Until(e.RetryAt); d < 115*time.Second || d > 120*time.Second {
		t.Errorf("RetryAt in %v, want about 120s", d)
	}
}
