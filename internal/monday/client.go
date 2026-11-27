package monday

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/config"
)

// Client is the transport boundary for monday.com's GraphQL API.
type Client struct {
	httpClient       *http.Client
	apiURL           string
	apiToken         string
	requestAuth      bool
	apiVersion       string
	maxResponseBytes int64
	maxRetries       int
	breakers         *circuitBreakers
}

type requestTokenKey struct{}

// WithRequestToken attaches a per-request Monday token to a context.
func WithRequestToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, requestTokenKey{}, token)
}

func requestToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(requestTokenKey{}).(string)
	return strings.TrimSpace(token), ok && strings.TrimSpace(token) != ""
}

// GraphQLRequest is the wire request sent to monday.com.
type GraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// GraphQLError is a sanitized error returned by the GraphQL API.
type GraphQLError struct {
	Message string         `json:"message"`
	Path    []any          `json:"path,omitempty"`
	Extra   map[string]any `json:"extensions,omitempty"`
}

// GraphQLResponse contains raw data and API-level errors.
type GraphQLResponse struct {
	Data      json.RawMessage `json:"data"`
	Errors    []GraphQLError  `json:"errors,omitempty"`
	AccountID int64           `json:"account_id,omitempty"`
	// Legacy top-level error envelope still used by some monday failures.
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func newRequestError(gqlErr GraphQLError) *RequestError {
	reqErr := &RequestError{Message: gqlErr.Message}
	if code, ok := gqlErr.Extra["code"].(string); ok {
		reqErr.Code = code
	}
	if seconds, ok := gqlErr.Extra["retry_in_seconds"].(float64); ok && seconds > 0 {
		reqErr.RetryAfter = min(time.Duration(seconds)*time.Second, 30*time.Second)
	}
	if len(gqlErr.Path) > 0 {
		parts := make([]string, 0, len(gqlErr.Path))
		for _, part := range gqlErr.Path {
			parts = append(parts, fmt.Sprint(part))
		}
		reqErr.Path = strings.Join(parts, ".")
	}
	return reqErr
}

// NewClient creates a reusable Monday GraphQL client.
func NewClient(cfg config.Config) *Client {
	return &Client{
		httpClient:       &http.Client{Timeout: cfg.HTTPTimeout},
		apiURL:           cfg.APIURL,
		apiToken:         cfg.APIToken,
		requestAuth:      cfg.AuthMode == "request",
		apiVersion:       cfg.APIVersion,
		maxResponseBytes: cfg.MaxResponseBytes,
		maxRetries:       cfg.MaxRetries,
		breakers:         newCircuitBreakers(),
	}
}

// Do executes one GraphQL request and decodes the response into output.
func (c *Client) Do(ctx context.Context, query string, variables map[string]any, output any) error {
	return c.DoWithVersion(ctx, query, variables, output, "")
}

// DoWithVersion executes one GraphQL request with an optional monday API-version override.
// Preview/automation operations use explicit versions without changing the server default.
func (c *Client) DoWithVersion(ctx context.Context, query string, variables map[string]any, output any, versionOverride string) error {
	if query == "" {
		return fmt.Errorf("graphql query cannot be empty")
	}
	apiToken := c.apiToken
	if c.requestAuth {
		var ok bool
		apiToken, ok = requestToken(ctx)
		if !ok {
			return errors.New("monday request requires a bearer credential")
		}
	}
	breakerKey := credentialFingerprint(apiToken)
	if err := c.breakers.allow(breakerKey); err != nil {
		return err
	}
	finish := func(err error) error {
		if err == nil {
			c.breakers.success(breakerKey)
			return nil
		}
		if temporaryFailure(err) && ctx.Err() == nil {
			c.breakers.failure(breakerKey)
		}
		return err
	}
	payload, err := json.Marshal(GraphQLRequest{Query: query, Variables: compactVariables(variables)})
	if err != nil {
		return fmt.Errorf("marshal graphql request: %w", err)
	}

	maxAttempts := c.maxRetries + 1
	if isMutation(query) {
		// A retried mutation can duplicate creates or repeat irreversible deletes:
		// monday may have accepted it even when the response was lost. Reads remain
		// retryable; mutations fail once and leave the retry decision to the caller.
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("create monday request: %w", err)
		}
		req.Header.Set("Authorization", apiToken)
		apiVersion := c.apiVersion
		if versionOverride != "" {
			apiVersion = versionOverride
		}
		req.Header.Set("API-Version", apiVersion)
		req.Header.Set("Content-Type", "application/json")

		response, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = &TransportError{Message: fmt.Sprintf("monday request failed: %s", redactToken(err.Error(), apiToken))}
			if attempt < maxAttempts-1 {
				if err := waitForRetry(ctx, attempt, 0); err != nil {
					return err
				}
				continue
			}
			return finish(lastErr)
		}

		limited := io.LimitReader(response.Body, c.maxResponseBytes+1)
		body, readErr := io.ReadAll(limited)
		response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read monday response: %w", readErr)
		}
		if int64(len(body)) > c.maxResponseBytes {
			return fmt.Errorf("monday response exceeded configured limit")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			apiErr := &APIError{
				StatusCode: response.StatusCode,
				Message:    http.StatusText(response.StatusCode),
				RetryAfter: retryAfter(response.Header.Get("Retry-After")),
				Temporary:  response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			}
			if apiErr.Temporary && attempt < maxAttempts-1 {
				if err := waitForRetry(ctx, attempt, apiErr.RetryAfter); err != nil {
					return err
				}
				continue
			}
			return finish(apiErr)
		}

		var envelope GraphQLResponse
		if err := json.Unmarshal(body, &envelope); err != nil {
			return fmt.Errorf("decode monday response: %w", err)
		}
		if len(envelope.Errors) > 0 {
			reqErr := newRequestError(envelope.Errors[0])
			reqErr.Message = redactToken(reqErr.Message, apiToken)
			if reqErr.Temporary() && attempt < maxAttempts-1 {
				lastErr = reqErr
				if err := waitForRetry(ctx, attempt, reqErr.RetryAfter); err != nil {
					return err
				}
				continue
			}
			return finish(reqErr)
		}
		if envelope.ErrorCode != "" {
			return finish(&RequestError{Code: envelope.ErrorCode, Message: redactToken(envelope.ErrorMessage, apiToken)})
		}
		if output == nil {
			return finish(nil)
		}
		if err := json.Unmarshal(envelope.Data, output); err != nil {
			return fmt.Errorf("decode monday data: %w", err)
		}
		return finish(nil)
	}
	return finish(lastErr)
}

// isMutation detects GraphQL mutation documents. The server treats every
// non-mutation as a read for retry purposes; monday queries in this codebase
// always use an explicit query/mutation keyword.
func isMutation(query string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(query)), "mutation")
}

// compactVariables drops nil variables. An omitted nullable variable means
// "argument not provided", while an explicit null is rejected by some monday
// resolvers (for example users(page: null) returns an internal error).
func compactVariables(variables map[string]any) map[string]any {
	if len(variables) == 0 {
		return nil
	}
	compact := make(map[string]any, len(variables))
	for key, value := range variables {
		if value == nil {
			continue
		}
		compact[key] = value
	}
	return compact
}

func retryAfter(value string) time.Duration {
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0
	}
	return min(time.Duration(seconds)*time.Second, 30*time.Second)
}

func waitForRetry(ctx context.Context, attempt int, retryAfter time.Duration) error {
	delay := retryAfter
	if delay == 0 {
		delay = min(250*time.Millisecond*time.Duration(1<<attempt), 5*time.Second)
	}
	// Add +/-20% jitter so independently retried reads do not synchronize into
	// a thundering herd after monday recovers or a rate-limit window resets.
	if jitter := delay / 5; jitter > 0 {
		delay += time.Duration(rand.Int64N(int64(2*jitter))) - jitter
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Timeout returns the transport timeout for diagnostics and tests.
func (c *Client) Timeout() time.Duration {
	return c.httpClient.Timeout
}

func redactToken(message, token string) string {
	if token == "" {
		return message
	}
	return strings.ReplaceAll(message, token, "[REDACTED]")
}
