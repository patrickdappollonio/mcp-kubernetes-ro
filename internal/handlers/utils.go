package handlers

import (
	"context"
	"encoding/base64"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/response"
)

// UtilsHandler provides MCP tools for utility operations related to Kubernetes.
// It includes base64 encoding and decoding capabilities that are commonly needed
// when working with Kubernetes secrets, ConfigMaps, and other encoded data.
type UtilsHandler struct{}

// NewUtilsHandler creates a new UtilsHandler.
// No configuration is required as utility operations are stateless.
func NewUtilsHandler() *UtilsHandler {
	return &UtilsHandler{}
}

// EncodeBase64Params defines the parameters for the encode_base64 MCP tool.
type EncodeBase64Params struct {
	// Data is the text data to encode to base64 format.
	Data string `json:"data"`
}

// DecodeBase64Params defines the parameters for the decode_base64 MCP tool.
type DecodeBase64Params struct {
	// Data is the base64-encoded data to decode to text format.
	Data string `json:"data"`
}

// EncodeBase64 implements the encode_base64 MCP tool.
// It encodes text data to base64 format, which is useful for creating or understanding
// Kubernetes secrets and other base64-encoded resources.
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

// DecodeBase64 implements the decode_base64 MCP tool.
// It decodes base64 data to text format, which is useful for reading the contents
// of Kubernetes secrets and other base64-encoded resources.
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

// GetTools returns all utility-related MCP tools provided by this handler.
// This includes tools for base64 encoding and decoding operations commonly
// needed when working with Kubernetes secrets and encoded data.
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
