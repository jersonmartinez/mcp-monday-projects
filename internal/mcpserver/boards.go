package mcpserver

import (
	"context"
	"fmt"

	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultListLimit = 25

// BoardToolset owns Monday-backed board and workspace MCP handlers.
type BoardToolset struct {
	client *monday.Client
}

// ListWorkspacesInput controls workspace pagination for the first page.
type ListWorkspacesInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of workspaces to return, between 1 and 100"`
}

// ListWorkspacesOutput is the stable MCP response for workspace discovery.
type ListWorkspacesOutput struct {
	Workspaces []monday.Workspace `json:"workspaces"`
	Count      int                `json:"count"`
}

// ListBoardsInput controls board pagination for the first page.
type ListBoardsInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of boards to return, between 1 and 100"`
}

// ListBoardsOutput is the stable MCP response for board discovery.
type ListBoardsOutput struct {
	Boards []monday.Board `json:"boards"`
	Count  int            `json:"count"`
}

// GetBoardInput identifies one board.
type GetBoardInput struct {
	BoardID string `json:"board_id" jsonschema:"monday board identifier"`
}

// GetBoardOutput is the stable MCP response for one board.
type GetBoardOutput struct {
	Board monday.Board `json:"board"`
}

// ListWorkspaces lists visible monday workspaces.
func (t *BoardToolset) ListWorkspaces(ctx context.Context, _ *mcp.CallToolRequest, input ListWorkspacesInput) (*mcp.CallToolResult, ListWorkspacesOutput, error) {
	limit, err := normalizeLimit(input.Limit)
	if err != nil {
		return nil, ListWorkspacesOutput{}, err
	}
	workspaces, err := t.client.ListWorkspaces(ctx, limit)
	if err != nil {
		return nil, ListWorkspacesOutput{}, fmt.Errorf("list workspaces: %w", err)
	}
	return nil, ListWorkspacesOutput{Workspaces: workspaces, Count: len(workspaces)}, nil
}

// ListBoards lists visible monday boards.
func (t *BoardToolset) ListBoards(ctx context.Context, _ *mcp.CallToolRequest, input ListBoardsInput) (*mcp.CallToolResult, ListBoardsOutput, error) {
	limit, err := normalizeLimit(input.Limit)
	if err != nil {
		return nil, ListBoardsOutput{}, err
	}
	boards, err := t.client.ListBoards(ctx, limit)
	if err != nil {
		return nil, ListBoardsOutput{}, fmt.Errorf("list boards: %w", err)
	}
	return nil, ListBoardsOutput{Boards: boards, Count: len(boards)}, nil
}

// GetBoard gets a board by identifier.
func (t *BoardToolset) GetBoard(ctx context.Context, _ *mcp.CallToolRequest, input GetBoardInput) (*mcp.CallToolResult, GetBoardOutput, error) {
	if input.BoardID == "" {
		return nil, GetBoardOutput{}, fmt.Errorf("board_id is required")
	}
	board, err := t.client.GetBoard(ctx, input.BoardID)
	if err != nil {
		return nil, GetBoardOutput{}, fmt.Errorf("get board: %w", err)
	}
	return nil, GetBoardOutput{Board: *board}, nil
}

func normalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultListLimit, nil
	}
	if limit < 1 || limit > 100 {
		return 0, fmt.Errorf("limit must be between 1 and 100")
	}
	return limit, nil
}

// RegisterBoardTools adds workspace and board discovery tools to an MCP server.
func RegisterBoardTools(server *mcp.Server, client *monday.Client) {
	tools := &BoardToolset{client: client}
	mcp.AddTool(server, &mcp.Tool{Name: "list_workspaces", Description: "List visible monday.com workspaces."}, tools.ListWorkspaces)
	mcp.AddTool(server, &mcp.Tool{Name: "list_boards", Description: "List visible monday.com boards."}, tools.ListBoards)
	mcp.AddTool(server, &mcp.Tool{Name: "get_board", Description: "Get one monday.com board by ID."}, tools.GetBoard)
}
