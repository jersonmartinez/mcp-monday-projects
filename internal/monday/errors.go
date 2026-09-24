package monday

import (
	"fmt"
	"time"
)

// APIError describes a sanitized HTTP/API failure from monday.com.
type APIError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
	Temporary  bool
}

func (e *APIError) Error() string {
	if e.StatusCode == 0 {
		return e.Message
	}
	return fmt.Sprintf("monday API returned HTTP %d: %s", e.StatusCode, e.Message)
}
