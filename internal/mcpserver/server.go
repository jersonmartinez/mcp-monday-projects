package mcpserver

import (
	"context"
	"runtime"

	"github.com/jersonmartinez/mcp-monday-projects/internal/config"
	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerInfoInput is intentionally empty; it makes the diagnostic tool easy to call.
type ServerInfoInput struct{}

// ServerInfoOutput describes the running MCP binary without exposing secrets.
type ServerInfoOutput struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Runtime    string `json:"runtime"`
	APIVersion string `json:"api_version"`
}

// ServerInfo returns safe build/runtime metadata.
func ServerInfo(_ context.Context, _ *mcp.CallToolRequest, _ ServerInfoInput) (*mcp.CallToolResult, ServerInfoOutput, error) {
	return nil, ServerInfoOutput{
		Name:       "mcp-monday-projects",
		Version:    "0.1.0",
		Runtime:    runtime.Version(),
		APIVersion: "configured-at-runtime",
	}, nil
}

// New creates the MCP server and registers all currently available tools.
func New(cfg config.Config) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "mcp-monday-projects", Version: "0.1.0"},
		nil,
	)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "server_info",
		Description: "Return safe server metadata and configuration diagnostics.",
	}, ServerInfo)
	client := monday.NewClient(cfg)
	RegisterBoardTools(server, client)
	RegisterBoardResourceTools(server, client)
	return server
}
