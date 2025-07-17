// Package handlers provides the core infrastructure for MCP tool registration and handling.
// It defines the common interfaces and types used by all tool handlers in the system.
package handlers

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// MCPTool represents a Model Context Protocol tool with both its definition and handler combined.
// It encapsulates the tool specification (name, description, parameters) and the actual
// implementation that processes requests for that tool.
type MCPTool struct {
	tool    mcp.Tool
	handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// NewMCPTool creates a new MCPTool with the given tool definition and handler function.
// The tool parameter defines the MCP tool specification (name, description, input schema),
// while the handler parameter provides the implementation that processes requests.
func NewMCPTool(tool mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) MCPTool {
	return MCPTool{
		tool:    tool,
		handler: handler,
	}
}

// Tool returns the MCP tool definition containing the tool's metadata and schema.
// This is used by the MCP server to register the tool and provide information
// to clients about available tools and their parameters.
func (t MCPTool) Tool() mcp.Tool {
	return t.tool
}

// Handler returns the tool handler function that processes incoming requests.
// The handler function takes a context and CallToolRequest, and returns either
// a successful result or an error.
func (t MCPTool) Handler() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return t.handler
}

// ToolRegistrator is the interface that handler structs must implement to provide
// their MCP tools to the server. Each handler (ResourceHandler, LogHandler, etc.)
// implements this interface to expose their available tools.
type ToolRegistrator interface {
	// GetTools returns a slice of MCPTool instances that this handler provides.
	// Each tool represents a specific capability (e.g., list_resources, get_logs)
	// that can be invoked by MCP clients.
	GetTools() []MCPTool
}
