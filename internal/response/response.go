// Package response provides utility functions for creating standardized MCP tool responses.
// It handles the conversion of Go data structures to properly formatted JSON responses
// and provides consistent error formatting across all tools.
package response

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// JSON creates a successful MCP tool response containing JSON-formatted data.
// It marshals the provided data structure to indented JSON and wraps it in
// an MCP CallToolResult. This is the standard way to return structured data
// from MCP tools.
//
// The data parameter can be any serializable Go value (struct, map, slice, etc.).
// Returns an error if the data cannot be marshaled to JSON.
func JSON(data interface{}) (*mcp.CallToolResult, error) {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// Error creates an MCP tool response indicating an error occurred.
// The message is returned to the client as an error result rather than
// successful tool output.
//
// Use this for user-facing errors that the client should handle gracefully.
func Error(message string) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(message), nil
}

// Errorf creates an MCP tool error response using printf-style formatting.
// This is a convenience function that combines fmt.Sprintf with Error().
//
// The format parameter supports standard printf verbs, and args provides
// the values to substitute. Use this for dynamic error messages that
// include variable information.
func Errorf(format string, args ...any) (*mcp.CallToolResult, error) {
	message := fmt.Sprintf(format, args...)
	return mcp.NewToolResultError(message), nil
}
