package mcpserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultMaxRequestBytes = int64(10 * 1024 * 1024)
	rateLimiterMaxClients  = 1024
	tokenValidTTL          = 5 * time.Minute
	tokenInvalidTTL        = 30 * time.Second
)

// HTTPAuthOptions configures authentication and abuse controls for the MCP
// HTTP endpoint. TokenValidator is optional; production wiring validates a
// bearer token against monday once and caches the result by token fingerprint.
type HTTPAuthOptions struct {
	Mode                 string
	AllowSharedToken     bool
	ClientKey            string
	AllowedTokenPrefixes []string
	MaxRequestBytes      int64
	RateLimitRPM         int
	RequireTLS           bool
	ValidateBearer       bool
	TokenValidator       func(context.Context, string) error
}

// NewHTTPHandler returns a stateless Streamable HTTP handler with liveness and
// readiness endpoints. Configuration is loaded before this handler is built,
// so a registered handler is ready to serve requests.
func NewHTTPHandler(server *mcp.Server, path string, authOptions ...HTTPAuthOptions) http.Handler {
	path = normalizeHTTPPath(path)
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})

	var auth HTTPAuthOptions
	if len(authOptions) > 0 {
		auth = authOptions[0]
	}
	if auth.MaxRequestBytes <= 0 {
		auth.MaxRequestBytes = defaultMaxRequestBytes
	}
	mcpHandler := http.Handler(stream)
	if auth.Mode == "request" || auth.ClientKey != "" {
		mcpHandler = authMiddleware(mcpHandler, auth)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/readyz", healthHandler)
	mux.Handle(path, securityHeaders(mcpHandler))
	return mux
}

func normalizeHTTPPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/mcp"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func authMiddleware(next http.Handler, options HTTPAuthOptions) http.Handler {
	limiter := newClientRateLimiter(options.RateLimitRPM)
	validator := newTokenValidationCache(options.TokenValidator)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if options.Mode == "env" && !options.AllowSharedToken {
			writeUnauthorized(w)
			return
		}
		if options.RequireTLS && !secureTransport(r) && !loopbackRequest(r) {
			writeError(w, http.StatusUpgradeRequired, "tls_required")
			return
		}
		if !validClientKey(r.Header.Get("X-MCP-Client-Key"), options.ClientKey) {
			writeUnauthorized(w)
			return
		}
		if r.ContentLength > options.MaxRequestBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, options.MaxRequestBytes)
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if options.Mode == "request" {
			if !ok || !allowedPrefix(token, options.AllowedTokenPrefixes) {
				writeUnauthorized(w)
				return
			}
		}
		if !limiter.allow(rateLimitKey(r, token, options.ClientKey)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded")
			return
		}
		if options.Mode == "request" {
			if options.ValidateBearer && options.TokenValidator != nil {
				if err := validator.validate(r.Context(), token); err != nil {
					var unavailable *tokenValidationUnavailable
					if errors.As(err, &unavailable) {
						writeError(w, http.StatusServiceUnavailable, "token_validation_unavailable")
						return
					}
					writeUnauthorized(w)
					return
				}
			}
			r = r.WithContext(monday.WithRequestToken(r.Context(), token))
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func allowedPrefix(token string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}

func validClientKey(actual, expected string) bool {
	if expected == "" {
		return true
	}
	actualHash := sha256.Sum256([]byte(actual))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(actualHash[:], expectedHash[:]) == 1
}

func secureTransport(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func loopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func fingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func rateLimitKey(r *http.Request, token, clientKey string) string {
	switch {
	case token != "":
		return "token:" + fingerprint(token)
	case clientKey != "":
		return "client-key:" + fingerprint(clientKey) + ":" + r.RemoteAddr
	default:
		return "remote:" + r.RemoteAddr
	}
}

type rateWindow struct {
	reset time.Time
	count int
}

type clientRateLimiter struct {
	mu      sync.Mutex
	rpm     int
	clients map[string]*rateWindow
	now     func() time.Time
}

func newClientRateLimiter(rpm int) *clientRateLimiter {
	return &clientRateLimiter{rpm: rpm, clients: map[string]*rateWindow{}, now: time.Now}
}

func (l *clientRateLimiter) allow(key string) bool {
	if l.rpm <= 0 {
		return true
	}
	now := l.now()
	l.mu.Lock()
	entry := l.clients[key]
	if entry == nil {
		entry = &rateWindow{}
		l.clients[key] = entry
		l.evictLocked()
	}
	if entry.reset.IsZero() || !now.Before(entry.reset) {
		entry.reset = now.Add(time.Minute)
		entry.count = 0
	}
	if entry.count >= l.rpm {
		l.mu.Unlock()
		return false
	}
	entry.count++
	l.mu.Unlock()
	return true
}

func (l *clientRateLimiter) evictLocked() {
	for len(l.clients) > rateLimiterMaxClients {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range l.clients {
			if oldestKey == "" || entry.reset.Before(oldest) {
				oldestKey, oldest = key, entry.reset
			}
		}
		delete(l.clients, oldestKey)
	}
}

type tokenValidationUnavailable struct{ error }

func (e *tokenValidationUnavailable) Error() string { return e.error.Error() }

type tokenValidationEntry struct {
	err         error
	unavailable bool
	expires     time.Time
}

type tokenValidationCache struct {
	mu        sync.Mutex
	validator func(context.Context, string) error
	entries   map[string]tokenValidationEntry
	now       func() time.Time
}

func newTokenValidationCache(validator func(context.Context, string) error) *tokenValidationCache {
	return &tokenValidationCache{validator: validator, entries: map[string]tokenValidationEntry{}, now: time.Now}
}

func (c *tokenValidationCache) validate(ctx context.Context, token string) error {
	key := fingerprint(token)
	now := c.now()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && now.Before(entry.expires) {
		c.mu.Unlock()
		if entry.err == nil {
			return nil
		}
		if entry.unavailable {
			return &tokenValidationUnavailable{error: entry.err}
		}
		return entry.err
	}
	c.mu.Unlock()

	err := c.validator(ctx, token)
	unavailable := temporaryValidationFailure(err)
	ttl := tokenInvalidTTL
	if err == nil {
		ttl = tokenValidTTL
	}
	c.mu.Lock()
	if len(c.entries) >= rateLimiterMaxClients {
		c.entries = map[string]tokenValidationEntry{}
	}
	c.entries[key] = tokenValidationEntry{err: err, unavailable: unavailable, expires: c.now().Add(ttl)}
	c.mu.Unlock()
	if err != nil && unavailable {
		return &tokenValidationUnavailable{error: err}
	}
	return err
}

func temporaryValidationFailure(err error) bool {
	if errors.Is(err, monday.ErrCircuitOpen) {
		return true
	}
	var apiErr *monday.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Temporary
	}
	var reqErr *monday.RequestError
	if errors.As(err, &reqErr) {
		return reqErr.Temporary()
	}
	var transportErr *monday.TransportError
	return errors.As(err, &transportErr)
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthorized")
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
