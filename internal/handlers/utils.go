package handlers

import (
	"context"
	"encoding/base64"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/response"
)

type UtilsHandler struct{}

func NewUtilsHandler() *UtilsHandler {
	return &UtilsHandler{}
}

type EncodeBase64Params struct {
	Data string `json:"data"`
}

type DecodeBase64Params struct {
	Data string `json:"data"`
}

func (h *UtilsHandler) EncodeBase64(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params EncodeBase64Params
	if err := request.BindArguments(&params); err != nil {
		return response.Errorf("failed to parse arguments: %s", err)
	}

	if params.Data == "" {
		return response.Error("data is required")
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(params.Data))

	result := map[string]interface{}{
		"original": params.Data,
		"encoded":  encoded,
	}

	return response.JSON(result)
}

func (h *UtilsHandler) DecodeBase64(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params DecodeBase64Params
	if err := request.BindArguments(&params); err != nil {
		return response.Errorf("failed to parse arguments: %s", err)
	}

	if params.Data == "" {
		return response.Error("data is required")
	}

	decoded, err := base64.StdEncoding.DecodeString(params.Data)
	if err != nil {
		return response.Errorf("failed to decode base64 data: %s", err)
	}

	result := map[string]any{
		"original": params.Data,
		"decoded":  string(decoded),
	}

	return response.JSON(result)
}

// GetTools returns all utils-related MCP tools
func (h *UtilsHandler) GetTools() []MCPTool {
	return []MCPTool{
		NewMCPTool(
			mcp.NewTool("encode_base64",
				mcp.WithDescription("Encode text data to base64 format"),
				mcp.WithString("data",
					mcp.Required(),
					mcp.Description("Text data to encode"),
				),
			),
			h.EncodeBase64,
		),
		NewMCPTool(
			mcp.NewTool("decode_base64",
				mcp.WithDescription("Decode base64 data to text format"),
				mcp.WithString("data",
					mcp.Required(),
					mcp.Description("Base64 data to decode"),
				),
			),
			h.DecodeBase64,
		),
	}
}
