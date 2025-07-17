package handlers

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// MCPTool represents a tool with both its definition and handler combined
type MCPTool struct {
	tool    mcp.Tool
	handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// NewMCPTool creates a new MCPTool with the given tool definition and handler
func NewMCPTool(tool mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) MCPTool {
	return MCPTool{
		tool:    tool,
		handler: handler,
	}
}

// Tool returns the tool definition
func (t MCPTool) Tool() mcp.Tool {
	return t.tool
}

// Handler returns the tool handler function
func (t MCPTool) Handler() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return t.handler
}

// ToolRegistrator interface for structs that can provide MCP tools
type ToolRegistrator interface {
	GetTools() []MCPTool
}
