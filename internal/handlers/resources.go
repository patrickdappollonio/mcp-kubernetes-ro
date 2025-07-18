package handlers

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/response"
)

// ResourceHandler provides MCP tools for Kubernetes resource operations.
// It handles listing and retrieving resources across all API groups, with support
// for filtering, pagination, and dynamic resource type resolution. The handler
// supports both namespaced and cluster-scoped resources.
type ResourceHandler struct {
	client *kubernetes.Client
}

// NewResourceHandler creates a new ResourceHandler with the provided Kubernetes client.
func NewResourceHandler(client *kubernetes.Client) *ResourceHandler {
	return &ResourceHandler{
		client: client,
	}
}

// ListResourcesParams defines the parameters for the list_resources MCP tool.
// It supports comprehensive filtering and pagination options for resource queries.
type ListResourcesParams struct {
	// ResourceType is the type of resource to list (e.g., "pods", "deployments").
	// Supports plural names, singular names, kinds, and short names.
	ResourceType string `json:"resource_type"`

	// APIVersion optionally constrains the search to a specific API version.
	// If empty, searches across all available API versions.
	APIVersion string `json:"api_version,omitempty"`

	// Namespace specifies the target namespace for namespaced resources.
	// Leave empty for cluster-scoped resources.
	Namespace string `json:"namespace,omitempty"`

	// Context specifies which Kubernetes context to use for this operation.
	// If empty, uses the current context from kubeconfig.
	Context string `json:"context,omitempty"`

	// LabelSelector filters resources by labels (e.g., "app=nginx,version=1.0").
	LabelSelector string `json:"label_selector,omitempty"`

	// FieldSelector filters resources by fields (e.g., "status.phase=Running").
	FieldSelector string `json:"field_selector,omitempty"`

	// Limit restricts the maximum number of resources returned.
	// If 0, returns all matching resources.
	Limit int `json:"limit,omitempty"`

	// Continue is a pagination token from a previous response.
	// Used to retrieve the next page of results.
	Continue string `json:"continue,omitempty"`
}

// ListResources implements the list_resources MCP tool.
// It retrieves a list of Kubernetes resources of the specified type with optional
// filtering and pagination. Results are sorted by creation timestamp (newest first)
// for consistent ordering across requests.
func (h *ResourceHandler) ListResources(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params ListResourcesParams
	if err := request.BindArguments(&params); err != nil {
		return response.Errorf("failed to parse arguments: %s", err)
	}

	if params.ResourceType == "" {
		return response.Error("resource_type is required")
	}

	// Use the appropriate client based on context
	client := h.client
	if params.Context != "" {
		contextClient, err := h.client.WithContext(params.Context)
		if err != nil {
			return response.Errorf("failed to create client with context %s: %v", params.Context, err)
		}
		client = contextClient
	}

	gvr, err := client.ResolveResourceType(params.ResourceType, params.APIVersion)
	if err != nil {
		return response.Errorf("failed to resolve resource type: %v", err)
	}

	listOptions := metav1.ListOptions{
		LabelSelector: params.LabelSelector,
		FieldSelector: params.FieldSelector,
		Continue:      params.Continue,
	}

	if params.Limit > 0 {
		listOptions.Limit = int64(params.Limit)
	}

	resources, err := client.ListResources(ctx, gvr, params.Namespace, listOptions)
	if err != nil {
		return response.Errorf("failed to list resources: %v", err)
	}

	// Extract metadata-only resource summaries
	items := make([]map[string]interface{}, len(resources.Items))
	for i, resource := range resources.Items {
		items[i] = extractResourceSummary(&resource)
	}

	// Only sort if not using pagination (no continue token and no limit)
	// When using pagination, sorting should be handled consistently by the server
	if params.Continue == "" && params.Limit == 0 {
		// Sort by creation timestamp (newest first)
		sort.Slice(items, func(i, j int) bool {
			timeI, okI := getCreationTime(items[i])
			timeJ, okJ := getCreationTime(items[j])

			if !okI && !okJ {
				return false // both invalid, maintain order
			}
			if !okI {
				return false // i is invalid, j comes first
			}
			if !okJ {
				return true // j is invalid, i comes first
			}

			return timeI.After(timeJ) // newer first
		})
	}

	result := map[string]interface{}{
		"resource_type": params.ResourceType,
		"namespace":     params.Namespace,
		"count":         len(items),
		"items":         items,
	}

	// Add continue token if there are more results
	if resources.GetContinue() != "" {
		result["continue"] = resources.GetContinue()
	}

	return response.JSON(result)
}

// GetResourceParams defines the parameters for the get_resource MCP tool.
// It specifies which specific resource instance to retrieve by name and type.
type GetResourceParams struct {
	// ResourceType is the type of resource to retrieve (e.g., "pod", "deployment").
	// Supports plural names, singular names, kinds, and short names.
	ResourceType string `json:"resource_type"`

	// Name is the specific name of the resource instance to retrieve.
	Name string `json:"name"`

	// APIVersion optionally constrains the search to a specific API version.
	// If empty, searches across all available API versions.
	APIVersion string `json:"api_version,omitempty"`

	// Namespace specifies the target namespace for namespaced resources.
	// Required for namespaced resources, leave empty for cluster-scoped resources.
	Namespace string `json:"namespace,omitempty"`

	// Context specifies which Kubernetes context to use for this operation.
	// If empty, uses the current context from kubeconfig.
	Context string `json:"context,omitempty"`
}

// GetResource implements the get_resource MCP tool.
// It retrieves the complete configuration and status of a specific Kubernetes resource
// by name and type. Returns the full resource object including all fields.
func (h *ResourceHandler) GetResource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params GetResourceParams
	if err := request.BindArguments(&params); err != nil {
		return response.Errorf("failed to parse arguments: %s", err)
	}

	if params.ResourceType == "" {
		return response.Error("resource_type is required")
	}

	if params.Name == "" {
		return response.Error("name is required")
	}

	// Use the appropriate client based on context
	client := h.client
	if params.Context != "" {
		contextClient, err := h.client.WithContext(params.Context)
		if err != nil {
			return response.Errorf("failed to create client with context %s: %v", params.Context, err)
		}
		client = contextClient
	}

	gvr, err := client.ResolveResourceType(params.ResourceType, params.APIVersion)
	if err != nil {
		return response.Errorf("failed to resolve resource type: %v", err)
	}

	resource, err := client.GetResource(ctx, gvr, params.Namespace, params.Name)
	if err != nil {
		return response.Errorf("failed to get resource: %v", err)
	}

	return response.JSON(resource.Object)
}

// extractResourceSummary extracts only essential fields from a resource for list operations.
// It returns a lightweight summary containing just metadata, apiVersion, and kind,
// which is sufficient for most listing and browsing operations while minimizing
// response size and processing time.
func extractResourceSummary(resource *unstructured.Unstructured) map[string]interface{} {
	summary := make(map[string]interface{})

	if apiVersion := resource.GetAPIVersion(); apiVersion != "" {
		summary["apiVersion"] = apiVersion
	}

	if kind := resource.GetKind(); kind != "" {
		summary["kind"] = kind
	}

	if metadata := resource.Object["metadata"]; metadata != nil {
		summary["metadata"] = metadata
	}

	return summary
}

// getCreationTime extracts the creation timestamp from a resource summary for sorting purposes.
// It safely navigates the metadata structure and parses the RFC3339 timestamp format
// used by Kubernetes. Returns false if the timestamp is missing or invalid.
func getCreationTime(item map[string]interface{}) (time.Time, bool) {
	metadata, ok := item["metadata"].(map[string]interface{})
	if !ok {
		return time.Time{}, false
	}

	creationTimestamp, ok := metadata["creationTimestamp"].(string)
	if !ok {
		return time.Time{}, false
	}

	t, err := time.Parse(time.RFC3339, creationTimestamp)
	if err != nil {
		return time.Time{}, false
	}

	return t, true
}

// APIResource represents metadata about a Kubernetes API resource type.
// It contains information about the resource's capabilities, naming conventions,
// and supported operations, similar to the output of "kubectl api-resources".
type APIResource struct {
	// Name is the plural name of the resource (e.g., "pods", "deployments").
	Name string `json:"name"`

	// SingularName is the singular form of the resource name (e.g., "pod", "deployment").
	SingularName string `json:"singularName"`

	// Namespaced indicates whether the resource is namespace-scoped or cluster-scoped.
	Namespaced bool `json:"namespaced"`

	// Kind is the resource kind used in YAML manifests (e.g., "Pod", "Deployment").
	Kind string `json:"kind"`

	// Verbs lists the supported operations for this resource (e.g., ["get", "list", "create"]).
	Verbs []string `json:"verbs"`

	// ShortNames contains abbreviated names for the resource (e.g., "po" for "pods").
	ShortNames []string `json:"shortNames,omitempty"`

	// APIVersion specifies the API group and version (e.g., "v1", "apps/v1").
	APIVersion string `json:"apiVersion"`

	// Categories groups resources into logical categories (e.g., "all").
	Categories []string `json:"categories,omitempty"`
}

// ListAPIResources implements the list_api_resources MCP tool.
// It discovers and returns information about all available Kubernetes API resources
// in the cluster, similar to "kubectl api-resources". This is useful for understanding
// what resource types are available and their capabilities.
func (h *ResourceHandler) ListAPIResources(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lists, err := h.client.DiscoverResources(ctx)
	if err != nil {
		return response.Errorf("failed to discover API resources: %v", err)
	}

	var resources []APIResource

	for _, list := range lists {
		_, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}

		for _, resource := range list.APIResources {
			if strings.Contains(resource.Name, "/") {
				continue
			}

			resources = append(resources, APIResource{
				Name:         resource.Name,
				SingularName: resource.SingularName,
				Namespaced:   resource.Namespaced,
				Kind:         resource.Kind,
				Verbs:        resource.Verbs,
				ShortNames:   resource.ShortNames,
				APIVersion:   list.GroupVersion,
				Categories:   resource.Categories,
			})
		}
	}

	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})

	result := map[string]interface{}{
		"resources": resources,
		"count":     len(resources),
	}

	return response.JSON(result)
}

// ListContexts implements the list_contexts MCP tool.
// It reads the kubeconfig file and returns information about all available
// Kubernetes contexts. This helps users understand what clusters and configurations
// are available for use with the context parameter in other tools.
func (h *ResourceHandler) ListContexts(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contexts, err := h.listKubeContexts()
	if err != nil {
		return response.Errorf("failed to list contexts: %v", err)
	}

	result := map[string]interface{}{
		"contexts": contexts,
		"count":    len(contexts),
	}

	return response.JSON(result)
}

// listKubeContexts delegates to the client's ListContexts method.
func (h *ResourceHandler) listKubeContexts() ([]kubernetes.KubeContext, error) {
	return h.client.ListContexts()
}

// GetTools returns all resource-related MCP tools provided by this handler.
// This includes tools for listing resources, getting specific resources,
// discovering API resources, and managing Kubernetes contexts.
func (h *ResourceHandler) GetTools() []MCPTool {
	return []MCPTool{
		NewMCPTool(
			mcp.NewTool("list_resources",
				mcp.WithDescription("List any Kubernetes resources by type with optional filtering, sorted newest first. Returns only metadata, apiVersion, and kind for lightweight responses. Use get_resource for full resource details. If you need a list of all resources, use the list_api_resources tool."),
				mcp.WithString("resource_type",
					mcp.Required(),
					mcp.Description("The type of resource to list"),
				),
				mcp.WithString("api_version",
					mcp.Description("API version for the resource (e.g., \"v1\", \"apps/v1\"), if not provided, the tool will try to resolve the resource type from the API resources list"),
				),
				mcp.WithString("namespace",
					mcp.Description("Target namespace (leave empty for cluster-scoped resources)"),
				),
				mcp.WithString("context",
					mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
				),
				mcp.WithString("label_selector",
					mcp.Description("Label selector to filter resources (e.g., \"app=nginx,version=1.0\")"),
				),
				mcp.WithString("field_selector",
					mcp.Description("Field selector to filter resources (e.g., \"status.phase=Running\")"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum number of resources to return (defaults to all)"),
				),
				mcp.WithString("continue",
					mcp.Description("Continue token for pagination (from previous response)"),
				),
			),
			h.ListResources,
		),
		NewMCPTool(
			mcp.NewTool("get_resource",
				mcp.WithDescription("Get specific resource details"),
				mcp.WithString("resource_type",
					mcp.Required(),
					mcp.Description("The type of resource to get"),
				),
				mcp.WithString("name",
					mcp.Required(),
					mcp.Description("Resource name"),
				),
				mcp.WithString("api_version",
					mcp.Description("API version for the resource (e.g., \"v1\", \"apps/v1\"), if not provided, the tool will try to resolve the resource type from the API resources list"),
				),
				mcp.WithString("namespace",
					mcp.Description("Target namespace (required for namespaced resources)"),
				),
				mcp.WithString("context",
					mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
				),
			),
			h.GetResource,
		),
		NewMCPTool(
			mcp.NewTool("list_api_resources",
				mcp.WithDescription("List available Kubernetes API resources with their details (similar to kubectl api-resources)"),
			),
			h.ListAPIResources,
		),
		NewMCPTool(
			mcp.NewTool("list_contexts",
				mcp.WithDescription("List available Kubernetes contexts from the kubeconfig file"),
			),
			h.ListContexts,
		),
	}
}
