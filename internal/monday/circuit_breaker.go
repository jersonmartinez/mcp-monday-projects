package monday

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	circuitFailureThreshold = 5
	circuitCooldown         = 30 * time.Second
	circuitMaxEntries       = 256
)

type circuitState struct {
	failures    int
	openedUntil time.Time
	lastUsed    time.Time
}

type circuitBreakers struct {
	mu      sync.Mutex
	entries map[string]*circuitState
	now     func() time.Time
}

func newCircuitBreakers() *circuitBreakers {
	return &circuitBreakers{entries: map[string]*circuitState{}, now: time.Now}
}

func credentialFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}

// ErrCircuitOpen is returned before calling monday when repeated temporary
// failures make a credential endpoint look unhealthy.
var ErrCircuitOpen = errors.New("monday circuit breaker is open")

func (c *circuitBreakers) allow(key string) error {
	now := c.now()
	c.mu.Lock()
	entry := c.entries[key]
	if entry == nil {
		entry = &circuitState{}
		c.entries[key] = entry
		c.evictLocked()
	}
	entry.lastUsed = now
	openUntil := entry.openedUntil
	c.mu.Unlock()
	if !openUntil.IsZero() && now.Before(openUntil) {
		return fmt.Errorf("%w until %s; repeated temporary monday failures were observed for this credential", ErrCircuitOpen, openUntil.UTC().Format(time.RFC3339))
	}
	return nil
}

func (c *circuitBreakers) success(key string) {
	c.mu.Lock()
	if entry := c.entries[key]; entry != nil {
		entry.failures = 0
		entry.openedUntil = time.Time{}
		entry.lastUsed = c.now()
	}
	c.mu.Unlock()
}

func (c *circuitBreakers) failure(key string) {
	now := c.now()
	c.mu.Lock()
	entry := c.entries[key]
	if entry == nil {
		entry = &circuitState{}
		c.entries[key] = entry
		c.evictLocked()
	}
	entry.failures++
	entry.lastUsed = now
	if entry.failures >= circuitFailureThreshold {
		entry.openedUntil = now.Add(circuitCooldown)
		entry.failures = 0
	}
	c.mu.Unlock()
}

// evictLocked keeps request-auth deployments bounded. It removes the least
// recently used closed breaker; if every entry is open it removes the oldest.
func (c *circuitBreakers) evictLocked() {
	for len(c.entries) > circuitMaxEntries {
		var oldestKey string
		var oldest time.Time
		for key, entry := range c.entries {
			if oldestKey == "" || entry.lastUsed.Before(oldest) {
				oldestKey, oldest = key, entry.lastUsed
			}
		}
		delete(c.entries, oldestKey)
	}
}

func temporaryFailure(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Temporary
	}
	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		return reqErr.Temporary()
	}
	var transportErr *TransportError
	return errors.As(err, &transportErr)
}

// TransportError is a network-level failure before a monday response exists.
type TransportError struct {
	Message string
}

func (e *TransportError) Error() string { return e.Message }
