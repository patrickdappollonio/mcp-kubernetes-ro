package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/response"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// MetricsHandler provides MCP tools for retrieving Kubernetes node and pod metrics.
// It requires the metrics-server to be installed and running in the cluster.
// The handler supports both cluster-wide and targeted metrics retrieval with
// client-side pagination for consistent ordering and performance.
type MetricsHandler struct {
	client *kubernetes.Client
}

// NewMetricsHandler creates a new MetricsHandler with the provided Kubernetes client.
func NewMetricsHandler(client *kubernetes.Client) *MetricsHandler {
	return &MetricsHandler{
		client: client,
	}
}

// isMetricsServerError checks if an error indicates that the metrics server is unavailable.
// It recognizes common error patterns that occur when the metrics-server is not installed
// or not responding, allowing for helpful error messages to be provided to users.
func isMetricsServerError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "metrics-server") ||
		strings.Contains(errStr, "metrics.k8s.io") ||
		strings.Contains(errStr, "the server could not find the requested resource") ||
		strings.Contains(errStr, "no metrics available") ||
		strings.Contains(errStr, "unable to fetch metrics")
}

// formatMetricsServerError provides a helpful error message when the metrics server is unavailable.
// It includes installation guidance to help users understand how to enable metrics functionality.
func formatMetricsServerError(err error) string {
	return fmt.Sprintf("Metrics server appears to be unavailable: %v\n\nYou might need to install the \"metrics-server\" in your cluster.", err)
}

// GetNodeMetricsParams defines the parameters for the get_node_metrics MCP tool.
// It supports both cluster-wide metrics retrieval and targeted node metrics with pagination.
type GetNodeMetricsParams struct {
	// NodeName specifies a specific node to get metrics for.
	// If empty, retrieves metrics for all nodes in the cluster.
	NodeName string `json:"node_name,omitempty"`

	// Context specifies which Kubernetes context to use for this operation.
	// If empty, uses the current context from kubeconfig.
	Context string `json:"context,omitempty"`

	// Limit restricts the maximum number of node metrics returned.
	// If 0, returns all matching metrics.
	Limit int `json:"limit,omitempty"`

	// Continue is a pagination token from a previous response.
	// Used to retrieve the next page of results.
	Continue string `json:"continue,omitempty"`

	// TitleOnly when true, returns only node names.
	// When false (default), returns complete node metrics information.
	TitleOnly *bool `json:"title_only,omitempty"`
}

// GetPodMetricsParams defines the parameters for the get_pod_metrics MCP tool.
// It supports namespace-scoped, cluster-wide, and targeted pod metrics with pagination.
type GetPodMetricsParams struct {
	// Namespace specifies the target namespace for pod metrics.
	// If empty, retrieves metrics for pods across all namespaces.
	Namespace string `json:"namespace,omitempty"`

	// PodName specifies a specific pod to get metrics for.
	// If provided, Namespace must also be specified.
	PodName string `json:"pod_name,omitempty"`

	// Context specifies which Kubernetes context to use for this operation.
	// If empty, uses the current context from kubeconfig.
	Context string `json:"context,omitempty"`

	// Limit restricts the maximum number of pod metrics returned.
	// If 0, returns all matching metrics.
	Limit int `json:"limit,omitempty"`

	// Continue is a pagination token from a previous response.
	// Used to retrieve the next page of results.
	Continue string `json:"continue,omitempty"`

	// TitleOnly when true, returns only pod names.
	// When false (default), returns complete pod metrics information.
	TitleOnly *bool `json:"title_only,omitempty"`
}

// GetNodeMetrics implements the get_node_metrics MCP tool.
// It retrieves CPU and memory usage metrics for cluster nodes from the metrics-server.
// Supports both single-node queries and cluster-wide metrics with client-side pagination
// for consistent ordering. Results are sorted by timestamp (newest first).
func (h *MetricsHandler) GetNodeMetrics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params GetNodeMetricsParams
	if err := request.BindArguments(&params); err != nil {
		return response.Errorf("failed to parse arguments: %s", err)
	}

	// Use the appropriate client based on context
	client, err := h.client.ForContext(params.Context)
	if err != nil {
		return response.Errorf("failed to create client with context %q: %s", params.Context, err)
	}

	// Determine whether to show title only (default to false for metrics)
	titleOnly := false
	if params.TitleOnly != nil {
		titleOnly = *params.TitleOnly
	}

	if params.NodeName != "" {
		// Get specific node metrics
		nodeMetrics, err := client.GetNodeMetricsByName(ctx, params.NodeName)
		if err != nil {
			if isMetricsServerError(err) {
				return response.Errorf("%s", formatMetricsServerError(err))
			}
			return response.Errorf("failed to get node metrics for %s: %v", params.NodeName, err)
		}

		if titleOnly {
			result := map[string]interface{}{
				"name": nodeMetrics.Name,
			}
			return response.JSON(result)
		}
		return response.JSON(nodeMetrics)
	}

	// Always fetch all node metrics from the server
	nodeMetricsList, err := client.GetNodeMetrics(ctx)
	if err != nil {
		if isMetricsServerError(err) {
			return response.Errorf("%s", formatMetricsServerError(err))
		}
		return response.Errorf("failed to get node metrics: %v", err)
	}

	if titleOnly {
		// Return only node names
		var nodeNames []string
		for i := range nodeMetricsList.Items {
			nodeNames = append(nodeNames, nodeMetricsList.Items[i].Name)
		}

		// Sort names alphabetically for consistency
		sort.Strings(nodeNames)

		// Handle pagination for names only
		if params.Limit > 0 {
			paginationState, err := parseContinueToken(params.Continue)
			if err != nil {
				return response.Errorf("invalid continue token: %v", err)
			}

			// Convert to interface slice for pagination
			allItems := make([]interface{}, len(nodeNames))
			for i, name := range nodeNames {
				allItems[i] = name
			}

			paginatedItems, hasMore := paginateItems(allItems, params.Limit, paginationState.Offset)

			result := map[string]interface{}{
				"kind":       "NodeMetricsList",
				"apiVersion": "metrics.k8s.io/v1beta1",
				"count":      len(paginatedItems),
				"items":      paginatedItems,
			}

			if hasMore {
				nextOffset := paginationState.Offset + params.Limit
				result["continue"] = generateContinueToken(nextOffset, "node", "")
			}

			return response.JSON(result)
		}

		result := map[string]interface{}{
			"kind":       "NodeMetricsList",
			"apiVersion": "metrics.k8s.io/v1beta1",
			"count":      len(nodeNames),
			"items":      nodeNames,
		}

		return response.JSON(result)
	}

	// Convert to interface slice for client-side pagination
	allItems := make([]interface{}, len(nodeMetricsList.Items))
	for i := range nodeMetricsList.Items {
		allItems[i] = nodeMetricsList.Items[i]
	}

	// Sort by timestamp (newest first) for consistent ordering
	sort.Slice(allItems, func(i, j int) bool {
		nodeI := allItems[i].(metricsv1beta1.NodeMetrics)
		nodeJ := allItems[j].(metricsv1beta1.NodeMetrics)
		return nodeI.Timestamp.After(nodeJ.Timestamp.Time)
	})

	// Handle client-side pagination
	if params.Limit > 0 {
		// Parse continue token to get offset
		paginationState, err := parseContinueToken(params.Continue)
		if err != nil {
			return response.Errorf("invalid continue token: %v", err)
		}

		// Apply client-side pagination
		paginatedItems, hasMore := paginateItems(allItems, params.Limit, paginationState.Offset)

		result := map[string]interface{}{
			"kind":       "NodeMetricsList",
			"apiVersion": "metrics.k8s.io/v1beta1",
			"count":      len(paginatedItems),
			"items":      paginatedItems,
		}

		// Add continue token if there are more results
		if hasMore {
			nextOffset := paginationState.Offset + params.Limit
			result["continue"] = generateContinueToken(nextOffset, "node", "")
		}

		return response.JSON(result)
	}

	// Return all items if no pagination requested
	result := map[string]interface{}{
		"kind":       "NodeMetricsList",
		"apiVersion": "metrics.k8s.io/v1beta1",
		"count":      len(allItems),
		"items":      allItems,
	}

	return response.JSON(result)
}

// GetPodMetrics implements the get_pod_metrics MCP tool.
// It retrieves CPU and memory usage metrics for cluster pods from the metrics-server.
// Supports namespace-scoped, cluster-wide, and single-pod queries with client-side
// pagination for consistent ordering. Results are sorted by timestamp (newest first).
func (h *MetricsHandler) GetPodMetrics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params GetPodMetricsParams
	if err := request.BindArguments(&params); err != nil {
		return response.Errorf("failed to parse arguments: %s", err)
	}

	// Use the appropriate client based on context
	client, err := h.client.ForContext(params.Context)
	if err != nil {
		return response.Errorf("failed to create client with context %s: %v", params.Context, err)
	}

	// Determine whether to show title only (default to false for metrics)
	titleOnly := false
	if params.TitleOnly != nil {
		titleOnly = *params.TitleOnly
	}

	if params.PodName != "" {
		// Get specific pod metrics
		if params.Namespace == "" {
			return response.Error("namespace is required when specifying pod_name")
		}

		podMetrics, err := client.GetPodMetricsByName(ctx, params.Namespace, params.PodName)
		if err != nil {
			if isMetricsServerError(err) {
				return response.Errorf("%s", formatMetricsServerError(err))
			}
			return response.Errorf("failed to get pod metrics for %s/%s: %v", params.Namespace, params.PodName, err)
		}

		if titleOnly {
			result := map[string]interface{}{
				"name":      podMetrics.Name,
				"namespace": podMetrics.Namespace,
			}
			return response.JSON(result)
		}
		return response.JSON(podMetrics)
	}

	// Always fetch all pod metrics from the server
	var podMetricsList *metricsv1beta1.PodMetricsList

	if params.Namespace != "" {
		// Get pod metrics for specific namespace
		podMetricsList, err = client.GetPodMetricsByNamespace(ctx, params.Namespace)
	} else {
		// Get pod metrics for all namespaces
		podMetricsList, err = client.GetPodMetrics(ctx)
	}

	if err != nil {
		if isMetricsServerError(err) {
			return response.Errorf("%s", formatMetricsServerError(err))
		}
		return response.Errorf("failed to get pod metrics: %v", err)
	}

	if titleOnly {
		// Return only pod names with namespaces
		type PodName struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		}
		var podNames []PodName
		for i := range podMetricsList.Items {
			podNames = append(podNames, PodName{
				Name:      podMetricsList.Items[i].Name,
				Namespace: podMetricsList.Items[i].Namespace,
			})
		}

		// Sort by namespace then name for consistency
		sort.Slice(podNames, func(i, j int) bool {
			if podNames[i].Namespace != podNames[j].Namespace {
				return podNames[i].Namespace < podNames[j].Namespace
			}
			return podNames[i].Name < podNames[j].Name
		})

		// Handle pagination for names only
		if params.Limit > 0 {
			paginationState, err := parseContinueToken(params.Continue)
			if err != nil {
				return response.Errorf("invalid continue token: %v", err)
			}

			// Validate that the continue token is for the same request type
			if paginationState.Type != "" && paginationState.Type != "pod" {
				return response.Error("continue token is not valid for pod metrics")
			}

			// Reset pagination if namespace context has changed
			if paginationState.Namespace != params.Namespace {
				paginationState.Offset = 0
			}

			// Convert to interface slice for pagination
			allItems := make([]interface{}, len(podNames))
			for i, podName := range podNames {
				allItems[i] = podName
			}

			paginatedItems, hasMore := paginateItems(allItems, params.Limit, paginationState.Offset)

			result := map[string]interface{}{
				"kind":       "PodMetricsList",
				"apiVersion": "metrics.k8s.io/v1beta1",
				"namespace":  params.Namespace,
				"count":      len(paginatedItems),
				"items":      paginatedItems,
			}

			if hasMore {
				nextOffset := paginationState.Offset + params.Limit
				result["continue"] = generateContinueToken(nextOffset, "pod", params.Namespace)
			}

			return response.JSON(result)
		}

		result := map[string]interface{}{
			"kind":       "PodMetricsList",
			"apiVersion": "metrics.k8s.io/v1beta1",
			"namespace":  params.Namespace,
			"count":      len(podNames),
			"items":      podNames,
		}

		return response.JSON(result)
	}

	// Convert to interface slice for client-side pagination
	allItems := make([]interface{}, len(podMetricsList.Items))
	for i := range podMetricsList.Items {
		allItems[i] = podMetricsList.Items[i]
	}

	// Sort by timestamp (newest first) for consistent ordering
	sort.Slice(allItems, func(i, j int) bool {
		podI := allItems[i].(metricsv1beta1.PodMetrics)
		podJ := allItems[j].(metricsv1beta1.PodMetrics)
		return podI.Timestamp.After(podJ.Timestamp.Time)
	})

	// Handle client-side pagination
	if params.Limit > 0 {
		// Parse continue token to get offset
		paginationState, err := parseContinueToken(params.Continue)
		if err != nil {
			return response.Errorf("invalid continue token: %v", err)
		}

		// Validate that the continue token is for the same request type
		if paginationState.Type != "" && paginationState.Type != "pod" {
			return response.Error("continue token is not valid for pod metrics")
		}

		// Reset pagination if namespace context has changed
		if paginationState.Namespace != params.Namespace {
			paginationState.Offset = 0
		}

		// Apply client-side pagination
		paginatedItems, hasMore := paginateItems(allItems, params.Limit, paginationState.Offset)

		result := map[string]interface{}{
			"kind":       "PodMetricsList",
			"apiVersion": "metrics.k8s.io/v1beta1",
			"namespace":  params.Namespace,
			"count":      len(paginatedItems),
			"items":      paginatedItems,
		}

		// Add continue token if there are more results
		if hasMore {
			nextOffset := paginationState.Offset + params.Limit
			result["continue"] = generateContinueToken(nextOffset, "pod", params.Namespace)
		}

		return response.JSON(result)
	}

	// Return all items if no pagination requested
	result := map[string]interface{}{
		"kind":       "PodMetricsList",
		"apiVersion": "metrics.k8s.io/v1beta1",
		"namespace":  params.Namespace,
		"count":      len(allItems),
		"items":      allItems,
	}

	return response.JSON(result)
}

// PaginationState represents the state for client-side pagination
type PaginationState struct {
	Offset    int    `json:"offset"`
	Type      string `json:"type"` // "node" or "pod"
	Namespace string `json:"namespace,omitempty"`
}

// generateContinueToken creates a continue token for client-side pagination
func generateContinueToken(offset int, itemType, namespace string) string {
	state := PaginationState{
		Offset:    offset,
		Type:      itemType,
		Namespace: namespace,
	}

	//nolint:errchkjson // we control the struct and it's strongly typed
	data, _ := json.Marshal(state)
	return base64.URLEncoding.EncodeToString(data)
}

// parseContinueToken parses a continue token to extract pagination state
func parseContinueToken(token string) (*PaginationState, error) {
	if token == "" {
		return &PaginationState{}, nil
	}

	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid continue token: %w", err)
	}

	var state PaginationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("invalid continue token format: %w", err)
	}

	return &state, nil
}

// paginateItems applies client-side pagination to a slice of items
func paginateItems(items []interface{}, limit, offset int) ([]interface{}, bool) {
	if offset >= len(items) {
		return []interface{}{}, false
	}

	end := offset + limit
	hasMore := end < len(items)

	if end > len(items) {
		end = len(items)
	}

	return items[offset:end], hasMore
}

// GetTools returns all metrics-related MCP tools provided by this handler.
// This includes tools for retrieving node and pod CPU/memory metrics from
// the metrics-server with support for filtering and pagination.
func (h *MetricsHandler) GetTools() []MCPTool {
	return []MCPTool{
		NewMCPTool(
			mcp.NewTool("get_node_metrics",
				mcp.WithDescription("Get node metrics (CPU and memory usage). Returns complete metrics by default (title_only=false), or only node names when title_only=true"),
				mcp.WithString("node_name",
					mcp.Description("Specific node name to get metrics for (optional - if not provided, returns metrics for all nodes)"),
				),
				mcp.WithString("context",
					mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum number of node metrics to return (optional - defaults to all)"),
				),
				mcp.WithString("continue",
					mcp.Description("Continue token for pagination (optional - from previous response)"),
				),
				mcp.WithBoolean("title_only",
					mcp.Description("When true, returns only node names. When false (default), returns complete node metrics"),
				),
			),
			h.GetNodeMetrics,
		),
		NewMCPTool(
			mcp.NewTool("get_pod_metrics",
				mcp.WithDescription("Get pod metrics (CPU and memory usage). Returns complete metrics by default (title_only=false), or only pod names with namespaces when title_only=true"),
				mcp.WithString("namespace",
					mcp.Description("Namespace to get pod metrics from (optional - if not provided, returns metrics for all pods)"),
				),
				mcp.WithString("pod_name",
					mcp.Description("Specific pod name to get metrics for (optional - if not provided, returns metrics for all pods in namespace or cluster)"),
				),
				mcp.WithString("context",
					mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum number of pod metrics to return (optional - defaults to all)"),
				),
				mcp.WithString("continue",
					mcp.Description("Continue token for pagination (optional - from previous response)"),
				),
				mcp.WithBoolean("title_only",
					mcp.Description("When true, returns only pod names with namespaces. When false (default), returns complete pod metrics"),
				),
			),
			h.GetPodMetrics,
		),
	}
}
