package mcpserver

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
)

// toolMetrics is a bounded, privacy-safe in-process registry. It records only
// aggregate numbers keyed by tool name; arguments, results, tokens, and error
// messages are never stored.
type toolMetrics struct {
	started time.Time
	mu      sync.Mutex
	tools   map[string]*toolMetric
}

type toolMetric struct {
	Calls           int64
	Errors          int64
	RateLimitErrors int64
	TotalMS         int64
	MaxMS           int64
}

func newToolMetrics() *toolMetrics {
	return &toolMetrics{started: time.Now(), tools: map[string]*toolMetric{}}
}

func (m *toolMetrics) record(tool string, elapsed time.Duration, err error) {
	if m == nil {
		return
	}
	ms := elapsed.Milliseconds()
	m.mu.Lock()
	entry := m.tools[tool]
	if entry == nil {
		entry = &toolMetric{}
		m.tools[tool] = entry
	}
	entry.Calls++
	if err != nil {
		entry.Errors++
		if rateLimitFailure(err) {
			entry.RateLimitErrors++
		}
	}
	entry.TotalMS += ms
	if ms > entry.MaxMS {
		entry.MaxMS = ms
	}
	m.mu.Unlock()
}

// rateLimitFailure classifies upstream throttling/budget failures without
// retaining the error text. Other validation or authorization failures are
// counted only as generic errors.
func rateLimitFailure(err error) bool {
	var apiErr *monday.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	var reqErr *monday.RequestError
	return errors.As(err, &reqErr) && reqErr.Temporary()
}

// ToolOperationalMetric is the aggregate latency, error, and rate-limit shape for one tool.
type ToolOperationalMetric struct {
	Name                  string  `json:"name"`
	Calls                 int64   `json:"calls"`
	Errors                int64   `json:"errors"`
	RateLimitErrors       int64   `json:"rate_limit_errors"`
	ErrorRate             float64 `json:"error_rate"`
	RateLimitErrorRate    float64 `json:"rate_limit_error_rate"`
	AverageMS             int64   `json:"average_ms"`
	MaxMS                 int64   `json:"max_ms"`
	TotalMS               int64   `json:"total_ms"`
	CallsPerMin           float64 `json:"calls_per_minute"`
	ErrorsPerMin          float64 `json:"errors_per_minute"`
	RateLimitErrorsPerMin float64 `json:"rate_limit_errors_per_minute"`
}

// OperationalMetricsInput optionally narrows the report to one tool.
type OperationalMetricsInput struct {
	Tool string `json:"tool,omitempty" jsonschema:"optional exact tool name filter"`
}

// OperationalMetricsOutput is safe to expose to operators: aggregate counters only.
type OperationalMetricsOutput struct {
	UptimeSeconds        int64                   `json:"uptime_seconds"`
	ToolCount            int                     `json:"tool_count"`
	TotalCalls           int64                   `json:"total_calls"`
	TotalErrors          int64                   `json:"total_errors"`
	TotalRateLimitErrors int64                   `json:"total_rate_limit_errors"`
	Tools                []ToolOperationalMetric `json:"tools"`
}

func (m *toolMetrics) snapshot(filter string) OperationalMetricsOutput {
	if m == nil {
		return OperationalMetricsOutput{Tools: []ToolOperationalMetric{}}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	uptime := time.Since(m.started)
	minutes := uptime.Minutes()
	if minutes <= 0 {
		minutes = 1.0 / 60.0
	}
	out := OperationalMetricsOutput{
		UptimeSeconds: int64(uptime.Seconds()),
		Tools:         make([]ToolOperationalMetric, 0, len(m.tools)),
	}
	for name, entry := range m.tools {
		if filter != "" && !strings.EqualFold(filter, name) {
			continue
		}
		average := int64(0)
		errorRate := 0.0
		rateLimitErrorRate := 0.0
		if entry.Calls > 0 {
			average = entry.TotalMS / entry.Calls
			errorRate = float64(entry.Errors) / float64(entry.Calls)
			rateLimitErrorRate = float64(entry.RateLimitErrors) / float64(entry.Calls)
		}
		metric := ToolOperationalMetric{
			Name: name, Calls: entry.Calls, Errors: entry.Errors, RateLimitErrors: entry.RateLimitErrors,
			ErrorRate: errorRate, RateLimitErrorRate: rateLimitErrorRate,
			AverageMS: average, MaxMS: entry.MaxMS, TotalMS: entry.TotalMS,
			CallsPerMin: float64(entry.Calls) / minutes, ErrorsPerMin: float64(entry.Errors) / minutes,
			RateLimitErrorsPerMin: float64(entry.RateLimitErrors) / minutes,
		}
		out.Tools = append(out.Tools, metric)
		out.TotalCalls += entry.Calls
		out.TotalErrors += entry.Errors
		out.TotalRateLimitErrors += entry.RateLimitErrors
	}
	sort.Slice(out.Tools, func(i, j int) bool { return out.Tools[i].Name < out.Tools[j].Name })
	out.ToolCount = len(out.Tools)
	return out
}
