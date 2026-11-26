package mcpserver

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
)

func TestToolMetricsCountRateLimitFailuresWithoutMessages(t *testing.T) {
	metrics := newToolMetrics()
	rateErr := fmt.Errorf("list items: %w", &monday.APIError{StatusCode: http.StatusTooManyRequests, Message: "Too Many Requests"})
	metrics.record("list_items", 25*time.Millisecond, rateErr)
	metrics.record("list_items", 75*time.Millisecond, nil)

	snapshot := metrics.snapshot("list_items")
	if snapshot.TotalCalls != 2 || snapshot.TotalErrors != 1 || snapshot.TotalRateLimitErrors != 1 {
		t.Fatalf("snapshot totals = %+v", snapshot)
	}
	if len(snapshot.Tools) != 1 {
		t.Fatalf("snapshot tools = %+v", snapshot.Tools)
	}
	entry := snapshot.Tools[0]
	if entry.RateLimitErrors != 1 || entry.RateLimitErrorRate != 0.5 || entry.AverageMS != 50 || entry.MaxMS != 75 {
		t.Fatalf("rate-limit metric = %+v", entry)
	}
}

func TestRateLimitFailureClassifiesMondayBudgetErrors(t *testing.T) {
	if !rateLimitFailure(&monday.RequestError{Code: "COMPLEXITY_BUDGET_EXHAUSTED"}) {
		t.Fatal("complexity budget exhaustion was not classified as throttling")
	}
	if rateLimitFailure(&monday.RequestError{Code: "USER_UNAUTHORIZED"}) {
		t.Fatal("authorization failure was classified as throttling")
	}
}
