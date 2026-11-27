package mcpserver

import (
	"context"

	"github.com/jersonmartinez/mcp-monday-projects/internal/application"
	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

// BulkCreateInput creates multiple items after validating the complete batch.
type BulkCreateInput struct {
	BoardID string                      `json:"board_id" jsonschema:"monday board identifier"`
	Rows    []application.BulkCreateRow `json:"rows" jsonschema:"creation rows (max 50): name, optional group_id or group title, optional column_values"`
	DryRun  *bool                       `json:"dry_run,omitempty" jsonschema:"default true: validate and plan only; set false to create"`
}

// ImportBoardInput imports CSV or Markdown table rows as board items.
type ImportBoardInput struct {
	BoardID string `json:"board_id" jsonschema:"monday board identifier"`
	Format  string `json:"format" jsonschema:"csv or markdown"`
	Text    string `json:"text" jsonschema:"table content. Required columns: name; optional: group. Other headers resolve as column IDs or exact titles."`
	DryRun  *bool  `json:"dry_run,omitempty" jsonschema:"default true: validate and plan only; set false to create"`
}

// ItemAssetsInput lists file metadata for an item.
type ItemAssetsInput struct {
	ItemID      string   `json:"item_id" jsonschema:"monday item identifier"`
	Source      string   `json:"source,omitempty" jsonschema:"all (default), columns, or gallery"`
	ColumnIDs   []string `json:"column_ids,omitempty" jsonschema:"optional file column IDs"`
	IncludeURLs bool     `json:"include_urls,omitempty" jsonschema:"include expiring provider file URLs; default false"`
}

// UpdateAssetsInput lists file metadata for an update.
type UpdateAssetsInput struct {
	UpdateID    string `json:"update_id" jsonschema:"monday update identifier"`
	ItemID      string `json:"item_id,omitempty" jsonschema:"owning item identifier; strongly recommended for scope verification"`
	IncludeURLs bool   `json:"include_urls,omitempty" jsonschema:"include expiring provider file URLs; default false"`
}

// AssetsOutput wraps asset metadata.
type AssetsOutput struct {
	Assets []domain.Asset `json:"assets"`
	Count  int            `json:"count"`
}

// ListAutomationsInput lists native board automations.
type ListAutomationsInput struct {
	BoardID string `json:"board_id" jsonschema:"monday board identifier"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum automations (1-100, default 100)"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"cursor from a previous page"`
}

// CreateAutomationInput creates a native board automation from a template or
// expert-authored workflow JSON. Workflow fields are decoded JSON values.
type CreateAutomationInput struct {
	BoardID                 string `json:"board_id" jsonschema:"monday board identifier"`
	Title                   string `json:"title" jsonschema:"automation title (1-160 characters)"`
	Description             string `json:"description,omitempty" jsonschema:"optional description (max 1000 characters)"`
	TemplateReferenceID     string `json:"template_reference_id,omitempty" jsonschema:"template ID for template-based creation; do not combine with workflow_blocks"`
	WorkflowBlocks          any    `json:"workflow_blocks,omitempty" jsonschema:"direct-creation workflow block JSON from monday's preview schema"`
	WorkflowVariables       any    `json:"workflow_variables,omitempty" jsonschema:"direct-creation workflow variable definitions; reference credentials, never embed secrets"`
	WorkflowVariablesValues any    `json:"workflow_variables_values,omitempty" jsonschema:"template variable values for template-based creation"`
	DryRun                  *bool  `json:"dry_run,omitempty" jsonschema:"default true: validate and plan only; set false to create the active automation"`
}

// CreateAutomationOutput reports native automation creation.
type CreateAutomationOutput struct {
	Result application.AutomationCreationResult `json:"result"`
}

// ManageAutomationInput controls a native board automation reversibly.
type ManageAutomationInput struct {
	BoardID      string `json:"board_id" jsonschema:"monday board identifier"`
	AutomationID string `json:"automation_id" jsonschema:"automation ID resolved through list_automations; never guess it"`
	Action       string `json:"action" jsonschema:"activate or deactivate. Prefer deactivate unless reactivation is explicit."`
}

// ManageAutomationOutput reports automation control outcome.
type ManageAutomationOutput struct {
	AutomationID string `json:"automation_id"`
	Action       string `json:"action"`
	Success      bool   `json:"success"`
}

// DeleteAutomationInput permanently deletes one native board automation.
type DeleteAutomationInput struct {
	BoardID      string `json:"board_id" jsonschema:"monday board identifier"`
	AutomationID string `json:"automation_id" jsonschema:"automation ID resolved through list_automations; never guess it"`
	Confirm      bool   `json:"confirm" jsonschema:"must be true for permanent deletion"`
}

// AutomationStatisticsInput queries native automation outcome statistics.
type AutomationStatisticsInput struct {
	BoardID              string   `json:"board_id,omitempty" jsonschema:"optional board scope; omit for account-wide"`
	Breakdown            string   `json:"breakdown,omitempty" jsonschema:"totals (default) or by_entity"`
	RunStatus            string   `json:"run_status,omitempty" jsonschema:"required for by_entity: success, failure, or exhausted"`
	UserIDs              []string `json:"user_ids,omitempty" jsonschema:"optional creator user IDs"`
	ExcludeAutomationIDs []string `json:"exclude_automation_ids,omitempty" jsonschema:"by_entity exclusions"`
}

// AutomationStatisticsOutput wraps provider statistics without forcing a schema on provider breakdown JSON.
type AutomationStatisticsOutput struct {
	Statistics any `json:"statistics"`
}

// AutomationRunsInput reads native automation execution history.
type AutomationRunsInput struct {
	BoardID       string   `json:"board_id,omitempty" jsonschema:"optional board scope; omit for account-wide"`
	Offset        int      `json:"offset,omitempty" jsonschema:"zero-based offset page"`
	States        []string `json:"states,omitempty" jsonschema:"success, failure, exhausted"`
	AutomationIDs []string `json:"automation_ids,omitempty" jsonschema:"automation ID filters"`
	ItemID        string   `json:"item_id,omitempty" jsonschema:"optional item filter"`
}

// AutomationRunsOutput wraps execution history.
type AutomationRunsOutput struct {
	Page domain.AutomationRunPage `json:"page"`
}

func registerAutomationTools(r *registry) {
	svc := r.svc
	add(r, ToolSpec{Name: "bulk_create_items", Category: CatBulk, Title: "Bulk create items",
		Description: "Validate and create up to 50 items. dry_run defaults to true; duplicate names in the batch and any invalid row stop the write."},
		func(ctx context.Context, in BulkCreateInput) (BulkOutput, error) {
			result, err := svc.BulkCreateItems(ctx, in.BoardID, in.Rows, dryRun(in.DryRun))
			if err != nil {
				return BulkOutput{}, wrap("bulk create items", err)
			}
			return BulkOutput{Result: result}, nil
		})
	add(r, ToolSpec{Name: "import_board_table", Category: CatBulk, Title: "Import board table",
		Description: "Import CSV or Markdown rows as items with the same validation and dry_run-first behavior as bulk_create_items."},
		func(ctx context.Context, in ImportBoardInput) (BulkOutput, error) {
			result, err := svc.ImportBoardTable(ctx, in.BoardID, in.Format, in.Text, dryRun(in.DryRun))
			if err != nil {
				return BulkOutput{}, wrap("import board table", err)
			}
			return BulkOutput{Result: result}, nil
		})
	add(r, ToolSpec{Name: "list_item_assets", Category: CatItemsRead, Title: "List item assets", ReadOnly: true,
		Description: "List file metadata attached to an item, file columns, gallery, and current-version update attachments. Expiring URLs are hidden unless include_urls=true."},
		func(ctx context.Context, in ItemAssetsInput) (AssetsOutput, error) {
			assets, err := svc.ListItemAssets(ctx, in.ItemID, in.Source, in.ColumnIDs, in.IncludeURLs)
			if err != nil {
				return AssetsOutput{}, wrap("list item assets", err)
			}
			return AssetsOutput{Assets: assets, Count: len(assets)}, nil
		})
	add(r, ToolSpec{Name: "list_update_assets", Category: CatCollab, Title: "List update assets", ReadOnly: true,
		Description: "List file metadata attached to an update. Pass item_id when known so ownership and workspace scope are verified."},
		func(ctx context.Context, in UpdateAssetsInput) (AssetsOutput, error) {
			assets, err := svc.ListUpdateAssets(ctx, in.UpdateID, in.ItemID, in.IncludeURLs)
			if err != nil {
				return AssetsOutput{}, wrap("list update assets", err)
			}
			return AssetsOutput{Assets: assets, Count: len(assets)}, nil
		})
	add(r, ToolSpec{Name: "list_automations", Category: CatAutomation, Title: "List automations", ReadOnly: true, Capability: "automation.read",
		Description: "List native monday board automations with active state and provider configuration. Uses the API version where board_automations is available; some legacy automations may be read-only."},
		func(ctx context.Context, in ListAutomationsInput) (domain.AutomationPage, error) {
			page, err := svc.ListAutomations(ctx, in.BoardID, in.Limit, in.Cursor)
			if err != nil {
				return domain.AutomationPage{}, wrap("list automations", err)
			}
			return page, nil
		})
	add(r, ToolSpec{Name: "create_automation", Category: CatAutomation, Title: "Create automation", Capability: "automation.write",
		Description: "Create a native monday board automation from a template reference or expert-authored workflow JSON. Uses monday's preview workflow API, is dry-run by default, and never accepts embedded credential secrets."},
		func(ctx context.Context, in CreateAutomationInput) (CreateAutomationOutput, error) {
			result, err := svc.CreateAutomation(ctx, application.AutomationCreationInput{
				BoardID:                 in.BoardID,
				Title:                   in.Title,
				Description:             in.Description,
				TemplateReferenceID:     in.TemplateReferenceID,
				WorkflowBlocks:          in.WorkflowBlocks,
				WorkflowVariables:       in.WorkflowVariables,
				WorkflowVariablesValues: in.WorkflowVariablesValues,
			}, dryRun(in.DryRun))
			if err != nil {
				return CreateAutomationOutput{}, wrap("create automation", err)
			}
			return CreateAutomationOutput{Result: result}, nil
		})
	add(r, ToolSpec{Name: "manage_automation", Category: CatAutomation, Title: "Manage automation", Capability: "automation.write",
		Description: "Activate or deactivate a native automation after verifying it belongs to the board. Use delete_automation only when permanent removal is explicit."},
		func(ctx context.Context, in ManageAutomationInput) (ManageAutomationOutput, error) {
			success, err := svc.ManageAutomation(ctx, in.BoardID, in.Action, in.AutomationID)
			if err != nil {
				return ManageAutomationOutput{}, wrap("manage automation", err)
			}
			return ManageAutomationOutput{AutomationID: in.AutomationID, Action: in.Action, Success: success}, nil
		})
	add(r, ToolSpec{Name: "delete_automation", Category: CatDeletes, Title: "Delete automation", Destructive: true, Permanent: true, Capability: "automation.delete",
		Description: "PERMANENTLY delete a native automation after verifying board ownership (MCP_ACCESS_LEVEL=full). Requires confirm=true; prefer manage_automation action=deactivate."},
		func(ctx context.Context, in DeleteAutomationInput) (ManageAutomationOutput, error) {
			success, err := svc.DeleteAutomation(ctx, in.BoardID, in.AutomationID, in.Confirm)
			if err != nil {
				return ManageAutomationOutput{}, wrap("delete automation", err)
			}
			return ManageAutomationOutput{AutomationID: in.AutomationID, Action: "delete", Success: success}, nil
		})
	add(r, ToolSpec{Name: "get_automation_statistics", Category: CatAutomation, Title: "Automation statistics", ReadOnly: true, Capability: "automation.read",
		Description: "Aggregate native automation success/failure totals or per-entity breakdowns. Uses monday's versioned automation statistics API."},
		func(ctx context.Context, in AutomationStatisticsInput) (AutomationStatisticsOutput, error) {
			stats, err := svc.AutomationStatistics(ctx, in.BoardID, in.RunStatus, in.Breakdown, in.UserIDs, in.ExcludeAutomationIDs)
			if err != nil {
				return AutomationStatisticsOutput{}, wrap("get automation statistics", err)
			}
			return AutomationStatisticsOutput{Statistics: stats}, nil
		})
	add(r, ToolSpec{Name: "list_automation_runs", Category: CatAutomation, Title: "Automation runs", ReadOnly: true, Capability: "automation.read",
		Description: "Read one bounded offset page of native automation/workflow execution history with state counts."},
		func(ctx context.Context, in AutomationRunsInput) (AutomationRunsOutput, error) {
			page, err := svc.ListAutomationRuns(ctx, in.BoardID, in.Offset, in.States, in.AutomationIDs, in.ItemID)
			if err != nil {
				return AutomationRunsOutput{}, wrap("list automation runs", err)
			}
			return AutomationRunsOutput{Page: page}, nil
		})
}
