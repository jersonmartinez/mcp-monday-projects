package mcpserver

import (
	"context"
	"fmt"

	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BoardResourceToolset owns item and board-structure handlers.
type BoardResourceToolset struct{ client *monday.Client }

type BoardIDInput struct {
	BoardID string `json:"board_id" jsonschema:"monday board identifier"`
}
type ItemIDInput struct {
	ItemID string `json:"item_id" jsonschema:"monday item identifier"`
}
type ListItemsInput struct {
	BoardID string `json:"board_id" jsonschema:"monday board identifier"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum number of items to return, between 1 and 100"`
}
type ItemValuesInput struct {
	BoardID      string         `json:"board_id" jsonschema:"monday board identifier"`
	ItemID       string         `json:"item_id" jsonschema:"monday item identifier"`
	ColumnValues map[string]any `json:"column_values" jsonschema:"column ID to monday column value mapping"`
}
type CreateItemInput struct {
	BoardID      string         `json:"board_id" jsonschema:"monday board identifier"`
	GroupID      string         `json:"group_id,omitempty" jsonschema:"optional monday group identifier"`
	Name         string         `json:"name" jsonschema:"item name"`
	ColumnValues map[string]any `json:"column_values,omitempty" jsonschema:"optional column ID to value mapping"`
}
type MoveItemInput struct {
	ItemID  string `json:"item_id" jsonschema:"monday item identifier"`
	GroupID string `json:"group_id" jsonschema:"monday group identifier"`
}
type GroupsOutput struct {
	Groups []monday.Group `json:"groups"`
	Count  int            `json:"count"`
}
type ColumnsOutput struct {
	Columns []monday.Column `json:"columns"`
	Count   int             `json:"count"`
}
type ItemsOutput struct {
	Items []monday.Item `json:"items"`
	Count int           `json:"count"`
}
type ItemOutput struct {
	Item monday.Item `json:"item"`
}

func (t *BoardResourceToolset) ListGroups(ctx context.Context, _ *mcp.CallToolRequest, input BoardIDInput) (*mcp.CallToolResult, GroupsOutput, error) {
	if input.BoardID == "" {
		return nil, GroupsOutput{}, fmt.Errorf("board_id is required")
	}
	groups, err := t.client.ListGroups(ctx, input.BoardID)
	if err != nil {
		return nil, GroupsOutput{}, fmt.Errorf("list groups: %w", err)
	}
	return nil, GroupsOutput{Groups: groups, Count: len(groups)}, nil
}

func (t *BoardResourceToolset) ListColumns(ctx context.Context, _ *mcp.CallToolRequest, input BoardIDInput) (*mcp.CallToolResult, ColumnsOutput, error) {
	if input.BoardID == "" {
		return nil, ColumnsOutput{}, fmt.Errorf("board_id is required")
	}
	columns, err := t.client.ListColumns(ctx, input.BoardID)
	if err != nil {
		return nil, ColumnsOutput{}, fmt.Errorf("list columns: %w", err)
	}
	return nil, ColumnsOutput{Columns: columns, Count: len(columns)}, nil
}

func (t *BoardResourceToolset) ListItems(ctx context.Context, _ *mcp.CallToolRequest, input ListItemsInput) (*mcp.CallToolResult, ItemsOutput, error) {
	if input.BoardID == "" {
		return nil, ItemsOutput{}, fmt.Errorf("board_id is required")
	}
	limit, err := normalizeLimit(input.Limit)
	if err != nil {
		return nil, ItemsOutput{}, err
	}
	items, err := t.client.ListItems(ctx, input.BoardID, limit)
	if err != nil {
		return nil, ItemsOutput{}, fmt.Errorf("list items: %w", err)
	}
	return nil, ItemsOutput{Items: items, Count: len(items)}, nil
}

func (t *BoardResourceToolset) CreateItem(ctx context.Context, _ *mcp.CallToolRequest, input CreateItemInput) (*mcp.CallToolResult, ItemOutput, error) {
	if input.BoardID == "" || input.Name == "" {
		return nil, ItemOutput{}, fmt.Errorf("board_id and name are required")
	}
	item, err := t.client.CreateItem(ctx, input.BoardID, input.GroupID, input.Name, input.ColumnValues)
	if err != nil {
		return nil, ItemOutput{}, fmt.Errorf("create item: %w", err)
	}
	return nil, ItemOutput{Item: *item}, nil
}

func (t *BoardResourceToolset) UpdateItemValues(ctx context.Context, _ *mcp.CallToolRequest, input ItemValuesInput) (*mcp.CallToolResult, ItemOutput, error) {
	if input.BoardID == "" || input.ItemID == "" {
		return nil, ItemOutput{}, fmt.Errorf("board_id and item_id are required")
	}
	if input.ColumnValues == nil {
		return nil, ItemOutput{}, fmt.Errorf("column_values is required")
	}
	item, err := t.client.UpdateItemValues(ctx, input.BoardID, input.ItemID, input.ColumnValues)
	if err != nil {
		return nil, ItemOutput{}, fmt.Errorf("update item values: %w", err)
	}
	return nil, ItemOutput{Item: *item}, nil
}

func (t *BoardResourceToolset) MoveItem(ctx context.Context, _ *mcp.CallToolRequest, input MoveItemInput) (*mcp.CallToolResult, ItemOutput, error) {
	if input.ItemID == "" || input.GroupID == "" {
		return nil, ItemOutput{}, fmt.Errorf("item_id and group_id are required")
	}
	item, err := t.client.MoveItem(ctx, input.ItemID, input.GroupID)
	if err != nil {
		return nil, ItemOutput{}, fmt.Errorf("move item: %w", err)
	}
	return nil, ItemOutput{Item: *item}, nil
}

func (t *BoardResourceToolset) ArchiveItem(ctx context.Context, _ *mcp.CallToolRequest, input ItemIDInput) (*mcp.CallToolResult, ItemOutput, error) {
	if input.ItemID == "" {
		return nil, ItemOutput{}, fmt.Errorf("item_id is required")
	}
	item, err := t.client.ArchiveItem(ctx, input.ItemID)
	if err != nil {
		return nil, ItemOutput{}, fmt.Errorf("archive item: %w", err)
	}
	return nil, ItemOutput{Item: *item}, nil
}

func RegisterBoardResourceTools(server *mcp.Server, client *monday.Client) {
	tools := &BoardResourceToolset{client: client}
	mcp.AddTool(server, &mcp.Tool{Name: "list_board_groups", Description: "List groups in a monday.com board."}, tools.ListGroups)
	mcp.AddTool(server, &mcp.Tool{Name: "list_board_columns", Description: "List columns in a monday.com board."}, tools.ListColumns)
	mcp.AddTool(server, &mcp.Tool{Name: "list_items", Description: "List items in a monday.com board."}, tools.ListItems)
	mcp.AddTool(server, &mcp.Tool{Name: "create_item", Description: "Create an item in a monday.com board."}, tools.CreateItem)
	mcp.AddTool(server, &mcp.Tool{Name: "update_item_column_values", Description: "Update monday.com item column values."}, tools.UpdateItemValues)
	mcp.AddTool(server, &mcp.Tool{Name: "move_item", Description: "Move a monday.com item to another group."}, tools.MoveItem)
	mcp.AddTool(server, &mcp.Tool{Name: "archive_item", Description: "Archive a monday.com item."}, tools.ArchiveItem)
}
