package monday

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/config"
)

func TestClientDoSendsAuthAndVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "test-token" {
			t.Errorf("Authorization = %q, want test-token", got)
		}
		if got := r.Header.Get("API-Version"); got != "2026-07" {
			t.Errorf("API-Version = %q, want 2026-07", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"boards":[{"id":"1"}]}}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		APIToken: "test-token", APIVersion: "2026-07", APIURL: server.URL,
		HTTPTimeout: time.Second, MaxResponseBytes: 1024,
	})
	var data struct {
		Boards []struct {
			ID string `json:"id"`
		} `json:"boards"`
	}
	if err := client.Do(context.Background(), "query { boards { id } }", nil, &data); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if len(data.Boards) != 1 || data.Boards[0].ID != "1" {
		t.Fatalf("decoded data = %+v", data)
	}
}

func TestClientDoReturnsGraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"invalid query"}]}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		APIToken: "test-token", APIVersion: "2026-07", APIURL: server.URL,
		HTTPTimeout: time.Second, MaxResponseBytes: 1024,
	})
	if err := client.Do(context.Background(), "query { invalid }", nil, nil); err == nil {
		t.Fatal("Do() error = nil, want GraphQL error")
	}
}

func TestClientRetriesRateLimit(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{
		APIToken: "test-token", APIVersion: "2026-07", APIURL: server.URL,
		HTTPTimeout: time.Second, MaxResponseBytes: 1024, MaxRetries: 1,
	})
	var data struct {
		OK bool `json:"ok"`
	}
	if err := client.Do(context.Background(), "query { ok }", nil, &data); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}
