// Package apierr is the one error type that carries what the HTTP API tells the user: a status
// code and a message. Packages return it (or alias it) and the server answers with it as is.
package apierr

import "time"

// Error is a failure to report to the user with an HTTP status.
type Error struct {
	Status  int
	Message string
	// For 429s: when the quota frees up again (the API's Retry-After, else a default backoff).
	RetryAt time.Time
	// For 429s: the daily quota is used up, so retrying within the day is pointless.
	Daily bool
}

func (e *Error) Error() string { return e.Message }
