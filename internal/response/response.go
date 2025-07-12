package response

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

func JSON(data interface{}) (*mcp.CallToolResult, error) {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func Error(message string) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(message), nil
}
