package mcpserver

import (
	"context"

	"github.com/jersonmartinez/mcp-monday-projects/internal/application"
)

// RunBoardAutomationInput executes deterministic server-side workflow rules.
type RunBoardAutomationInput struct {
	BoardID string                       `json:"board_id" jsonschema:"monday board identifier"`
	Rules   []application.AutomationRule `json:"rules" jsonschema:"ordered deterministic rules (max 10); first matching rule wins"`
	Limit   int                          `json:"limit,omitempty" jsonschema:"items to scan (1-200, default 100)"`
	DryRun  *bool                        `json:"dry_run,omitempty" jsonschema:"default true: plan only; set false to apply"`
}

// AutomationExecutionOutput wraps custom workflow execution.
type AutomationExecutionOutput struct {
	Result application.AutomationRunResult `json:"result"`
}

// CompleteItemInput completes one item through validated multi-step workflow.
type CompleteItemInput struct {
	ItemID         string `json:"item_id" jsonschema:"monday item identifier"`
	StatusColumnID string `json:"status_column_id,omitempty" jsonschema:"status column override; auto-detected by default"`
	DoneValue      string `json:"done_value,omitempty" jsonschema:"exact status label to set (default Done)"`
	MoveGroupID    string `json:"move_group_id,omitempty" jsonschema:"optional destination group ID or exact title"`
	UpdateBody     string `json:"update_body,omitempty" jsonschema:"optional completion update"`
	DryRun         *bool  `json:"dry_run,omitempty" jsonschema:"default true: preview the multi-step workflow; set false to apply"`
}

// CompleteItemOutput wraps completion workflow outcome.
type CompleteItemOutput struct {
	Result application.CompleteItemResult `json:"result"`
}

// HandoffItemInput transfers ownership and optionally moves workflow state.
type HandoffItemInput struct {
	ItemID         string  `json:"item_id" jsonschema:"monday item identifier"`
	PeopleColumnID string  `json:"people_column_id,omitempty" jsonschema:"people column override; auto-detected by default"`
	Assignees      []int64 `json:"assignees" jsonschema:"monday user IDs to assign; resolve with list_users first"`
	StatusColumnID string  `json:"status_column_id,omitempty" jsonschema:"optional status column"`
	StatusValue    string  `json:"status_value,omitempty" jsonschema:"optional workflow status to set"`
	UpdateBody     string  `json:"update_body,omitempty" jsonschema:"optional handoff note"`
	DryRun         *bool   `json:"dry_run,omitempty" jsonschema:"default true: preview the multi-step workflow; set false to apply"`
}

// HandoffItemOutput wraps handoff workflow outcome.
type HandoffItemOutput struct {
	Result application.HandoffItemResult `json:"result"`
}

// FlowMetricsInput selects flow statistics.
type FlowMetricsInput struct {
	ReportInput
	WindowDays int `json:"window_days,omitempty" jsonschema:"statistics window in days (1-365, default 30)"`
}

// FlowMetricsOutput wraps project execution statistics.
type FlowMetricsOutput struct {
	Metrics application.FlowMetrics `json:"metrics"`
}

func registerWorkflowTools(r *registry) {
	svc := r.svc
	add(r, ToolSpec{Name: "run_board_automation", Category: CatAutomation, Title: "Run board automation", Capability: "automation.write",
		Description: "Evaluate deterministic workflow rules against a bounded board snapshot and optionally apply status/column updates, moves, and updates/comments. This complements native monday automations and is dry-run by default."},
		func(ctx context.Context, in RunBoardAutomationInput) (AutomationExecutionOutput, error) {
			result, err := svc.RunBoardAutomation(ctx, in.BoardID, in.Rules, in.Limit, dryRun(in.DryRun))
			if err != nil {
				return AutomationExecutionOutput{}, wrap("run board automation", err)
			}
			return AutomationExecutionOutput{Result: result}, nil
		})
	add(r, ToolSpec{Name: "complete_item", Category: CatAutomation, Title: "Complete item", Capability: "automation.write",
		Description: "Validate and execute a completion workflow: set a Done-like status, optionally move to a closing group, and optionally post a completion update. Multi-step, auditable, and dry-run by default."},
		func(ctx context.Context, in CompleteItemInput) (CompleteItemOutput, error) {
			result, err := svc.CompleteItem(ctx, in.ItemID, in.StatusColumnID, in.DoneValue, in.MoveGroupID, in.UpdateBody, dryRun(in.DryRun))
			if err != nil {
				return CompleteItemOutput{}, wrap("complete item", err)
			}
			return CompleteItemOutput{Result: result}, nil
		})
	add(r, ToolSpec{Name: "handoff_item", Category: CatAutomation, Title: "Handoff item", Capability: "automation.write",
		Description: "Transfer ownership to resolved monday users, optionally change workflow status, and post a handoff update in one guarded multi-step workflow. Dry-run by default."},
		func(ctx context.Context, in HandoffItemInput) (HandoffItemOutput, error) {
			result, err := svc.HandoffItem(ctx, in.ItemID, in.PeopleColumnID, in.Assignees, in.StatusColumnID, in.StatusValue, in.UpdateBody, dryRun(in.DryRun))
			if err != nil {
				return HandoffItemOutput{}, wrap("handoff item", err)
			}
			return HandoffItemOutput{Result: result}, nil
		})
	add(r, ToolSpec{Name: "board_flow_metrics", Category: CatReports, Title: "Board flow metrics", ReadOnly: true, Capability: "reports.read",
		Description: "Project execution statistics: throughput proxy, WIP by status/group, bottlenecks, workload, overdue/stale work, completion ratio, and aging/cycle-time approximations."},
		func(ctx context.Context, in FlowMetricsInput) (FlowMetricsOutput, error) {
			metrics, err := svc.BoardFlowMetrics(ctx, in.BoardID, in.WindowDays, in.columns())
			if err != nil {
				return FlowMetricsOutput{}, wrap("board flow metrics", err)
			}
			return FlowMetricsOutput{Metrics: metrics}, nil
		})
}
