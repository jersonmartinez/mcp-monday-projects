package monday

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/config"
)

// Client is the transport boundary for monday.com's GraphQL API.
type Client struct {
	httpClient       *http.Client
	apiURL           string
	apiToken         string
	apiVersion       string
	maxResponseBytes int64
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
}

// NewClient creates a reusable Monday GraphQL client.
func NewClient(cfg config.Config) *Client {
	return &Client{
		httpClient:       &http.Client{Timeout: cfg.HTTPTimeout},
		apiURL:           cfg.APIURL,
		apiToken:         cfg.APIToken,
		apiVersion:       cfg.APIVersion,
		maxResponseBytes: cfg.MaxResponseBytes,
	}
}

// Do executes one GraphQL request and decodes the response into output.
func (c *Client) Do(ctx context.Context, query string, variables map[string]any, output any) error {
	if query == "" {
		return fmt.Errorf("graphql query cannot be empty")
	}
	payload, err := json.Marshal(GraphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("marshal graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create monday request: %w", err)
	}
	req.Header.Set("Authorization", c.apiToken)
	req.Header.Set("API-Version", c.apiVersion)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("monday request failed: %w", err)
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, c.maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read monday response: %w", err)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return fmt.Errorf("monday response exceeded configured limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("monday API returned HTTP %d", response.StatusCode)
	}

	var envelope GraphQLResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode monday response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("monday graphql request failed: %s", envelope.Errors[0].Message)
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("decode monday data: %w", err)
	}
	return nil
}

// Timeout returns the transport timeout for diagnostics and tests.
func (c *Client) Timeout() time.Duration {
	return c.httpClient.Timeout
}
