package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/portforward"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/response"
)

// PortForwardHandler provides MCP tools for managing port-forwarding sessions to Kubernetes pods.
// It supports starting, stopping, and listing active port forwards with multiple port mappings per session.
type PortForwardHandler struct {
	client  *kubernetes.Client
	manager *portforward.Manager
}

// NewPortForwardHandler creates a new PortForwardHandler with the provided Kubernetes client and port-forward manager.
func NewPortForwardHandler(client *kubernetes.Client, manager *portforward.Manager) *PortForwardHandler {
	return &PortForwardHandler{
		client:  client,
		manager: manager,
	}
}

// StartPortForward implements the start_port_forward MCP tool.
// It establishes a port-forwarding session to a pod with one or more port mappings.
func (h *PortForwardHandler) StartPortForward(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params struct {
		// Namespace specifies the pod's namespace.
		Namespace string `json:"namespace"`

		// Pod specifies the target pod name.
		Pod string `json:"pod"`

		// Ports is an array of port mappings to forward.
		Ports []portforward.PortMapping `json:"ports"`

		// Context specifies which Kubernetes context to use for this operation.
		Context string `json:"context"`
	}

	if err := request.BindArguments(&params); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.Namespace == "" {
		return nil, errors.New("namespace is required")
	}

	if params.Pod == "" {
		return nil, errors.New("pod name is required")
	}

	if len(params.Ports) == 0 {
		return nil, errors.New("at least one port mapping is required")
	}

	for i, p := range params.Ports {
		if p.PodPort <= 0 || p.PodPort > 65535 {
			return nil, fmt.Errorf("ports[%d]: pod_port must be between 1 and 65535, got %d", i, p.PodPort)
		}
		if p.LocalPort < 0 || p.LocalPort > 65535 {
			return nil, fmt.Errorf("ports[%d]: local_port must be between 0 and 65535, got %d", i, p.LocalPort)
		}
	}

	// Use the appropriate client based on context
	client, err := h.client.ForContext(params.Context)
	if err != nil {
		return nil, fmt.Errorf("failed to create client with context %s: %w", params.Context, err)
	}

	entry, err := h.manager.Start(
		client.RESTConfig(),
		client.Clientset(),
		params.Namespace,
		params.Pod,
		params.Ports,
		params.Context,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start port forward: %w", err)
	}

	return response.JSON(entry)
}

// StopPortForward implements the stop_port_forward MCP tool.
// It terminates a specific port-forwarding session by its ID.
func (h *PortForwardHandler) StopPortForward(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params struct {
		// ID is the unique identifier of the port-forward session to stop.
		ID string `json:"id"`
	}

	if err := request.BindArguments(&params); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.ID == "" {
		return nil, errors.New("port forward id is required")
	}

	if err := h.manager.Stop(params.ID); err != nil {
		return nil, fmt.Errorf("failed to stop port forward: %w", err)
	}

	return response.JSON(map[string]interface{}{
		"stopped": true,
		"id":      params.ID,
	})
}

// ListPortForwards implements the list_port_forwards MCP tool.
// It returns all active port-forwarding sessions.
func (h *PortForwardHandler) ListPortForwards(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entries := h.manager.List()

	return response.JSON(map[string]interface{}{
		"port_forwards": entries,
		"count":         len(entries),
	})
}

// GetTools returns all port-forwarding MCP tools provided by this handler.
func (h *PortForwardHandler) GetTools() []MCPTool {
	return []MCPTool{
		NewMCPTool(
			mcp.NewTool("start_port_forward",
				mcp.WithDescription("Start port forwarding to a Kubernetes pod. Supports multiple port mappings per session. Each mapping forwards a local port to a port on the pod. Set local_port to 0 (or omit) for automatic port assignment."),
				mcp.WithString("namespace",
					mcp.Required(),
					mcp.Description("Pod namespace"),
				),
				mcp.WithString("pod",
					mcp.Required(),
					mcp.Description("Pod name"),
				),
				mcp.WithArray("ports",
					mcp.Required(),
					mcp.Description("Array of port mappings to forward"),
					mcp.Items(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"pod_port":   map[string]any{"type": "number", "description": "Port on the pod to forward to (1-65535)"},
							"local_port": map[string]any{"type": "number", "description": "Local port to listen on (0 or omit for auto-assign)"},
						},
						"required": []string{"pod_port"},
					}),
				),
				mcp.WithString("context",
					mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
				),
			),
			h.StartPortForward,
		),
		NewMCPTool(
			mcp.NewTool("stop_port_forward",
				mcp.WithDescription("Stop an active port-forwarding session by its ID"),
				mcp.WithString("id",
					mcp.Required(),
					mcp.Description("Port forward session ID (e.g. \"pf-1\")"),
				),
			),
			h.StopPortForward,
		),
		NewMCPTool(
			mcp.NewTool("list_port_forwards",
				mcp.WithDescription("List all active port-forwarding sessions with their port mappings and metadata"),
			),
			h.ListPortForwards,
		),
	}
}
