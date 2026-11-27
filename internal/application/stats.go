package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

// FlowMetrics are execution statistics for project management. Completion in
// monday is status-derived rather than an event stream, so completed-in-window
// and cycle time are transparent proxies based on updated_at plus Done status.
type FlowMetrics struct {
	BoardID                string                 `json:"board_id"`
	BoardName              string                 `json:"board_name"`
	WindowDays             int                    `json:"window_days"`
	Truncated              bool                   `json:"truncated"`
	TotalItems             int                    `json:"total_items"`
	OpenItems              int                    `json:"open_items"`
	DoneItems              int                    `json:"done_items"`
	CompletionPct          float64                `json:"completion_pct"`
	CreatedInWindow        int                    `json:"created_in_window"`
	UpdatedInWindow        int                    `json:"updated_in_window"`
	CompletedInWindowProxy int                    `json:"completed_in_window_proxy"`
	OverdueItems           int                    `json:"overdue_items"`
	StaleItems             int                    `json:"stale_items"`
	UnassignedOpen         int                    `json:"unassigned_open_items"`
	BlockedItems           int                    `json:"blocked_items"`
	AverageOpenAgeDays     float64                `json:"average_open_age_days"`
	AverageCycleTimeProxy  float64                `json:"average_cycle_time_days_proxy"`
	WIPByStatus            []domain.Count         `json:"wip_by_status"`
	WIPByGroup             []domain.Count         `json:"wip_by_group"`
	Workload               []domain.WorkloadEntry `json:"workload,omitempty"`
	BottleneckStatus       string                 `json:"bottleneck_status,omitempty"`
	Warnings               []string               `json:"warnings,omitempty"`
}

func roundMetric(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func parseBoardTime(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 MST", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// BoardFlowMetrics combines existing board analytics with throughput/aging
// statistics. It uses the same bounded Snapshot as all reports and explicitly
// marks truncated samples.
func (s *Service) BoardFlowMetrics(ctx context.Context, boardID string, windowDays int, override domain.ReportColumns) (FlowMetrics, error) {
	if err := requireID("board_id", boardID); err != nil {
		return FlowMetrics{}, err
	}
	if windowDays == 0 {
		windowDays = 30
	}
	if windowDays < 1 || windowDays > 365 {
		return FlowMetrics{}, invalid("window_days must be between 1 and 365")
	}
	snapshot, err := s.Snapshot(ctx, boardID, s.reportMaxItems)
	if err != nil {
		return FlowMetrics{}, err
	}
	now := s.now()
	summary := domain.Summarize(snapshot, override, now)
	cols, workload := domain.Workload(snapshot, override, now)
	_, overdue := domain.Overdue(snapshot, override, now)
	_, stale := domain.Stale(snapshot, override, 7, now)
	cutoff := now.AddDate(0, 0, -windowDays)
	wipStatus := map[string]int{}
	wipGroup := map[string]int{}
	openAge := 0.0
	openCount := 0
	cycleTotal := 0.0
	cycleCount := 0
	created := 0
	updated := 0
	completedProxy := 0
	for _, item := range snapshot.Items {
		status := item.CellText(cols.StatusColumnID)
		done := isDoneText(status, "")
		createdAt, hasCreated := parseBoardTime(item.CreatedAt)
		updatedAt, hasUpdated := parseBoardTime(item.UpdatedAt)
		if hasCreated && !createdAt.Before(cutoff) {
			created++
		}
		if hasUpdated && !updatedAt.Before(cutoff) {
			updated++
			if done {
				completedProxy++
			}
		}
		if done {
			if hasCreated && hasUpdated && !updatedAt.Before(createdAt) {
				cycleTotal += updatedAt.Sub(createdAt).Hours() / 24
				cycleCount++
			}
			continue
		}
		if status == "" {
			status = "(empty)"
		}
		wipStatus[status]++
		group := "(no group)"
		if item.Group != nil && strings.TrimSpace(item.Group.Title) != "" {
			group = item.Group.Title
		}
		wipGroup[group]++
		if hasCreated {
			openAge += now.Sub(createdAt).Hours() / 24
			openCount++
		}
	}
	sort.Slice(workload, func(i, j int) bool {
		if workload[i].Open == workload[j].Open {
			return workload[i].Name < workload[j].Name
		}
		return workload[i].Open > workload[j].Open
	})
	statusCounts := make([]domain.Count, 0, len(wipStatus))
	for key, count := range wipStatus {
		statusCounts = append(statusCounts, domain.Count{Key: key, Count: count})
	}
	groupCounts := make([]domain.Count, 0, len(wipGroup))
	for key, count := range wipGroup {
		groupCounts = append(groupCounts, domain.Count{Key: key, Count: count})
	}
	sortCountsDesc := func(values []domain.Count) {
		sort.Slice(values, func(i, j int) bool {
			if values[i].Count == values[j].Count {
				return values[i].Key < values[j].Key
			}
			return values[i].Count > values[j].Count
		})
	}
	sortCountsDesc(statusCounts)
	sortCountsDesc(groupCounts)
	result := FlowMetrics{
		BoardID:                boardID,
		BoardName:              snapshot.Board.Name,
		WindowDays:             windowDays,
		Truncated:              snapshot.Truncated,
		TotalItems:             summary.TotalItems,
		OpenItems:              summary.TotalItems - summary.DoneItems,
		DoneItems:              summary.DoneItems,
		CompletionPct:          summary.CompletionPct,
		CreatedInWindow:        created,
		UpdatedInWindow:        updated,
		CompletedInWindowProxy: completedProxy,
		OverdueItems:           len(overdue),
		StaleItems:             len(stale),
		UnassignedOpen:         summary.UnassignedOpen,
		BlockedItems:           summary.BlockedItems,
		WIPByStatus:            statusCounts,
		WIPByGroup:             groupCounts,
		Workload:               workload,
	}
	if openCount > 0 {
		result.AverageOpenAgeDays = roundMetric(openAge / float64(openCount))
	}
	if cycleCount > 0 {
		result.AverageCycleTimeProxy = roundMetric(cycleTotal / float64(cycleCount))
	}
	if len(statusCounts) > 0 {
		result.BottleneckStatus = statusCounts[0].Key
	}
	result.Warnings = append(result.Warnings, "completed_in_window_proxy and cycle time are status/update-derived approximations, not immutable transition events")
	if snapshot.Truncated {
		result.Warnings = append(result.Warnings, "board snapshot truncated; metrics describe the bounded sample")
	}
	if warning := domain.UnreadableColumnsWarning(domain.UnreadableColumns(snapshot.Items, cols.StatusColumnID, cols.DateColumnID, cols.PeopleColumnID)); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return result, nil
}
