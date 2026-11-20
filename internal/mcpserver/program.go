package mcpserver

import (
	"context"
	"fmt"

	"github.com/jersonmartinez/mcp-monday-projects/internal/application"
	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

// Program-scale read inputs.
type FindItemTextInput struct {
	BoardID   string   `json:"board_id" jsonschema:"monday board identifier"`
	Text      string   `json:"text" jsonschema:"text to find in item name or visible column values"`
	ColumnIDs []string `json:"column_ids,omitempty" jsonschema:"optional columns to search; default all visible columns"`
	MaxItems  int      `json:"max_items,omitempty" jsonschema:"bounded snapshot size"`
}
type VerifyColumnsInput struct {
	Expected map[string]map[string]any `json:"expected" jsonschema:"item ID to expected column ID and friendly value mappings"`
}
type VerifyOutput struct {
	Result application.VerifyResult `json:"result"`
}
type ProgressInput struct {
	BoardID               string `json:"board_id" jsonschema:"monday board identifier"`
	StatusColumnID        string `json:"status_column_id" jsonschema:"status column used for grouping"`
	PhaseColumnID         string `json:"phase_column_id,omitempty" jsonschema:"optional phase/dropdown column used for grouping"`
	PlannedEffortColumnID string `json:"planned_effort_column_id" jsonschema:"numeric planned effort column"`
	EffortSpentColumnID   string `json:"effort_spent_column_id,omitempty" jsonschema:"optional numeric spent effort column"`
	MaxItems              int    `json:"max_items,omitempty" jsonschema:"bounded snapshot size"`
}
type ProgressOutput struct {
	Report application.ProgressReport `json:"report"`
}
type DependenciesInput struct {
	BoardID  string `json:"board_id"`
	ItemID   string `json:"item_id"`
	ColumnID string `json:"column_id"`
}
type DependenciesOutput struct {
	Dependencies application.Dependencies `json:"dependencies"`
}
type SetDependenciesInput struct {
	BoardID  string   `json:"board_id"`
	ItemID   string   `json:"item_id"`
	ColumnID string   `json:"column_id"`
	Mode     string   `json:"mode,omitempty" jsonschema:"add, remove, or replace (default replace)"`
	ItemIDs  []string `json:"item_ids" jsonschema:"dependency item IDs"`
}
type SetDependenciesOutput struct {
	Item     domain.Item `json:"item"`
	Warnings []string    `json:"warnings,omitempty"`
}
type TimelinePreviewInput struct {
	BoardID            string `json:"board_id"`
	ItemID             string `json:"item_id"`
	TimelineColumnID   string `json:"timeline_column_id"`
	DependencyColumnID string `json:"dependency_column_id"`
	From               string `json:"from"`
	To                 string `json:"to"`
	MaxItems           int    `json:"max_items,omitempty"`
}
type TimelinePreviewOutput struct {
	Items []application.DependencyPreview `json:"items"`
}
type SubitemsRollupInput struct {
	ItemIDs        []string `json:"item_ids"`
	StatusColumnID string   `json:"status_column_id"`
}
type SubitemsRollupOutput struct {
	Items []application.SubitemRollup `json:"items"`
}

// Update batch inputs.
type BulkUpdatesInput struct {
	Updates map[string]string `json:"updates" jsonschema:"item ID to update body"`
	Body    string            `json:"body,omitempty" jsonschema:"common body used when an item has no specific body"`
	DryRun  *bool             `json:"dry_run,omitempty"`
}
type BulkUpdatesOutput struct {
	Result application.BulkUpdatesResult `json:"result"`
}
type BulkPeopleRow struct {
	ItemID   string   `json:"item_id"`
	UserIDs  []string `json:"user_ids"`
	ColumnID string   `json:"column_id,omitempty"`
}
type BulkPeopleInput struct {
	BoardID string          `json:"board_id"`
	Rows    []BulkPeopleRow `json:"rows"`
	DryRun  *bool           `json:"dry_run,omitempty"`
}
type BulkDateRow struct {
	ItemID   string `json:"item_id"`
	Date     string `json:"date"`
	ColumnID string `json:"column_id,omitempty"`
}
type BulkDateInput struct {
	BoardID string        `json:"board_id"`
	Rows    []BulkDateRow `json:"rows"`
	DryRun  *bool         `json:"dry_run,omitempty"`
}

func registerProgramTools(r *registry) {
	svc := r.svc
	add(r, ToolSpec{Name: "find_item_by_text", Category: CatItemsRead, Title: "Find item by text", ReadOnly: true, Description: "Find visible items whose name or selected text, link, or long-text columns contain a string."}, func(ctx context.Context, in FindItemTextInput) (ItemsOutput, error) {
		page, err := svc.TextMatch(ctx, in.BoardID, in.Text, in.ColumnIDs, in.MaxItems)
		if err != nil {
			return ItemsOutput{}, wrap("find item by text", err)
		}
		return pageOutput(page), nil
	})
	add(r, ToolSpec{Name: "verify_item_columns", Category: CatItemsRead, Title: "Verify item columns", ReadOnly: true, Description: "Compare expected values with persisted board values and report every difference and unreadable column."}, func(ctx context.Context, in VerifyColumnsInput) (VerifyOutput, error) {
		result, err := svc.VerifyColumns(ctx, in.Expected)
		if err != nil {
			return VerifyOutput{}, wrap("verify item columns", err)
		}
		return VerifyOutput{Result: result}, nil
	})
	add(r, ToolSpec{Name: "board_progress_report", Category: CatReports, Title: "Board progress report", ReadOnly: true, Description: "Roll up planned and spent numeric effort by status and optional phase, including item and effort percentages."}, func(ctx context.Context, in ProgressInput) (ProgressOutput, error) {
		report, err := svc.Progress(ctx, in.BoardID, in.StatusColumnID, in.PhaseColumnID, in.PlannedEffortColumnID, in.EffortSpentColumnID, in.MaxItems)
		if err != nil {
			return ProgressOutput{}, wrap("board progress report", err)
		}
		return ProgressOutput{Report: report}, nil
	})
	add(r, ToolSpec{Name: "get_item_dependencies", Category: CatItemsRead, Title: "Get item dependencies", ReadOnly: true, Description: "Read the dependency column and return both items this item depends on and items that depend on it."}, func(ctx context.Context, in DependenciesInput) (DependenciesOutput, error) {
		result, err := svc.Dependencies(ctx, in.BoardID, in.ItemID, in.ColumnID)
		if err != nil {
			return DependenciesOutput{}, wrap("get item dependencies", err)
		}
		return DependenciesOutput{Dependencies: result}, nil
	})
	add(r, ToolSpec{Name: "set_item_dependencies", Category: CatItemsWrite, Title: "Set item dependencies", Description: "Add, remove, or replace dependency links after validating that referenced items exist on the same board."}, func(ctx context.Context, in SetDependenciesInput) (SetDependenciesOutput, error) {
		item, warnings, err := svc.SetDependencies(ctx, in.BoardID, in.ItemID, in.ColumnID, in.Mode, in.ItemIDs)
		if err != nil {
			return SetDependenciesOutput{}, wrap("set item dependencies", err)
		}
		return SetDependenciesOutput{Item: item, Warnings: warnings}, nil
	})
	add(r, ToolSpec{Name: "timeline_dependency_preview", Category: CatReports, Title: "Preview timeline dependency move", ReadOnly: true, Description: "Preview the date shift for directly dependent items without writing any timeline."}, func(ctx context.Context, in TimelinePreviewInput) (TimelinePreviewOutput, error) {
		items, err := svc.PreviewDependencyMove(ctx, in.BoardID, in.ItemID, in.TimelineColumnID, in.DependencyColumnID, in.From, in.To, in.MaxItems)
		if err != nil {
			return TimelinePreviewOutput{}, wrap("timeline dependency preview", err)
		}
		return TimelinePreviewOutput{Items: items}, nil
	})
	add(r, ToolSpec{Name: "subitems_status_rollup", Category: CatReports, Title: "Subitems status rollup", ReadOnly: true, Description: "Summarize subitem status counts for several parent items in one bounded read."}, func(ctx context.Context, in SubitemsRollupInput) (SubitemsRollupOutput, error) {
		items, err := svc.SubitemsRollup(ctx, in.ItemIDs, in.StatusColumnID)
		if err != nil {
			return SubitemsRollupOutput{}, wrap("subitems status rollup", err)
		}
		return SubitemsRollupOutput{Items: items}, nil
	})
	add(r, ToolSpec{Name: "bulk_create_updates", Category: CatBulk, Title: "Bulk create updates", Description: "Publish one update per item with per-item results; dry_run defaults to true."}, func(ctx context.Context, in BulkUpdatesInput) (BulkUpdatesOutput, error) {
		result, err := svc.BulkCreateUpdates(ctx, in.Updates, in.Body, dryRun(in.DryRun))
		if err != nil {
			return BulkUpdatesOutput{}, wrap("bulk create updates", err)
		}
		return BulkUpdatesOutput{Result: result}, nil
	})
	add(r, ToolSpec{Name: "bulk_set_people", Category: CatBulk, Title: "Bulk set people", Description: "Validate and set people values for up to 50 items with the normal dry-run safety gate."}, func(ctx context.Context, in BulkPeopleInput) (BulkOutput, error) {
		if len(in.Rows) == 0 || len(in.Rows) > application.MaxBulkItems {
			return BulkOutput{}, fmt.Errorf("bulk set people: rows must contain between 1 and %d items", application.MaxBulkItems)
		}
		rows := make([]application.BulkUpdate, 0, len(in.Rows))
		for _, row := range in.Rows {
			column := row.ColumnID
			if column == "" {
				column = "owner"
			}
			values := make([]any, 0, len(row.UserIDs))
			for _, id := range row.UserIDs {
				values = append(values, id)
			}
			rows = append(rows, application.BulkUpdate{ItemID: row.ItemID, ColumnValues: map[string]any{column: values}})
		}
		result, err := svc.BulkUpdateItems(ctx, in.BoardID, rows, dryRun(in.DryRun))
		if err != nil {
			return BulkOutput{}, wrap("bulk set people", err)
		}
		return BulkOutput{Result: result}, nil
	})
	add(r, ToolSpec{Name: "bulk_set_dates", Category: CatBulk, Title: "Bulk set dates", Description: "Validate and set date values for up to 50 items with persisted-value warnings."}, func(ctx context.Context, in BulkDateInput) (BulkOutput, error) {
		if len(in.Rows) == 0 || len(in.Rows) > application.MaxBulkItems {
			return BulkOutput{}, fmt.Errorf("bulk set dates: rows must contain between 1 and %d items", application.MaxBulkItems)
		}
		rows := make([]application.BulkUpdate, 0, len(in.Rows))
		for _, row := range in.Rows {
			column := row.ColumnID
			if column == "" {
				column = "due"
			}
			rows = append(rows, application.BulkUpdate{ItemID: row.ItemID, ColumnValues: map[string]any{column: row.Date}})
		}
		result, err := svc.BulkUpdateItems(ctx, in.BoardID, rows, dryRun(in.DryRun))
		if err != nil {
			return BulkOutput{}, wrap("bulk set dates", err)
		}
		return BulkOutput{Result: result}, nil
	})
}

func persistedValueWarnings(ctx context.Context, svc *application.Service, itemID string, values map[string]any) []string {
	var warnings []string
	for columnID, value := range values {
		if _, ok := value.(map[string]any); !ok {
			continue
		}
		warnings = append(warnings, svc.PersistedValueWarnings(ctx, itemID, columnID, value)...)
	}
	return warnings
}
