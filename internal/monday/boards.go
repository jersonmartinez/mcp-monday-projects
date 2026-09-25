package monday

import "context"

const listWorkspacesQuery = `query ListWorkspaces($limit: Int!) {
  workspaces(limit: $limit) { id name kind }
}`

const listBoardsQuery = `query ListBoards($limit: Int!) {
  boards(limit: $limit) { id name description state board_kind }
}`

const getBoardQuery = `query GetBoard($id: ID!) {
  boards(ids: [$id]) { id name description state board_kind }
}`

// Workspace is the provider-neutral representation of a monday workspace.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Board is the provider-neutral representation of a monday board.
type Board struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	State       string `json:"state,omitempty"`
	BoardKind   string `json:"board_kind,omitempty"`
}

// ListWorkspaces returns visible workspaces up to the requested limit.
func (c *Client) ListWorkspaces(ctx context.Context, limit int) ([]Workspace, error) {
	var data struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	if err := c.Do(ctx, listWorkspacesQuery, map[string]any{"limit": limit}, &data); err != nil {
		return nil, err
	}
	return data.Workspaces, nil
}

// ListBoards returns visible boards up to the requested limit.
func (c *Client) ListBoards(ctx context.Context, limit int) ([]Board, error) {
	var data struct {
		Boards []Board `json:"boards"`
	}
	if err := c.Do(ctx, listBoardsQuery, map[string]any{"limit": limit}, &data); err != nil {
		return nil, err
	}
	return data.Boards, nil
}

// GetBoard returns a board by its Monday identifier.
func (c *Client) GetBoard(ctx context.Context, id string) (*Board, error) {
	var data struct {
		Boards []Board `json:"boards"`
	}
	if err := c.Do(ctx, getBoardQuery, map[string]any{"id": id}, &data); err != nil {
		return nil, err
	}
	if len(data.Boards) == 0 {
		return nil, &NotFoundError{Resource: "board", ID: id}
	}
	return &data.Boards[0], nil
}

// NotFoundError represents a missing Monday resource without exposing payloads.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return e.Resource + " not found: " + e.ID
}
