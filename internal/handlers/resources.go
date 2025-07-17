package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/response"
)

type ResourceHandler struct {
	client     *kubernetes.Client
	baseConfig *kubernetes.Config
}

func NewResourceHandler(client *kubernetes.Client, baseConfig *kubernetes.Config) *ResourceHandler {
	return &ResourceHandler{
		client:     client,
		baseConfig: baseConfig,
	}
}

type ListResourcesParams struct {
	ResourceType  string `json:"resource_type"`
	APIVersion    string `json:"api_version,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	Context       string `json:"context,omitempty"`
	LabelSelector string `json:"label_selector,omitempty"`
	FieldSelector string `json:"field_selector,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	Continue      string `json:"continue,omitempty"`
}

func (h *ResourceHandler) ListResources(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params ListResourcesParams
	if err := request.BindArguments(&params); err != nil {
		return response.Error(fmt.Sprintf("failed to parse arguments: %v", err))
	}

	if params.ResourceType == "" {
		return response.Error("resource_type is required")
	}

	// Use the appropriate client based on context
	client := h.client
	if params.Context != "" {
		contextClient, err := kubernetes.NewClientWithContext(h.baseConfig, params.Context)
		if err != nil {
			return response.Error(fmt.Sprintf("failed to create client with context %s: %v", params.Context, err))
		}
		client = contextClient
	}

	gvr, err := client.ResolveResourceType(params.ResourceType, params.APIVersion)
	if err != nil {
		return response.Error(fmt.Sprintf("failed to resolve resource type: %v", err))
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
		return response.Error(fmt.Sprintf("failed to list resources: %v", err))
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

type GetResourceParams struct {
	ResourceType string `json:"resource_type"`
	Name         string `json:"name"`
	APIVersion   string `json:"api_version,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	Context      string `json:"context,omitempty"`
}

func (h *ResourceHandler) GetResource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params GetResourceParams
	if err := request.BindArguments(&params); err != nil {
		return response.Error(fmt.Sprintf("failed to parse arguments: %v", err))
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
		contextClient, err := kubernetes.NewClientWithContext(h.baseConfig, params.Context)
		if err != nil {
			return response.Error(fmt.Sprintf("failed to create client with context %s: %v", params.Context, err))
		}
		client = contextClient
	}

	gvr, err := client.ResolveResourceType(params.ResourceType, params.APIVersion)
	if err != nil {
		return response.Error(fmt.Sprintf("failed to resolve resource type: %v", err))
	}

	resource, err := client.GetResource(ctx, gvr, params.Namespace, params.Name)
	if err != nil {
		return response.Error(fmt.Sprintf("failed to get resource: %v", err))
	}

	return response.JSON(resource.Object)
}

// extractResourceSummary extracts only metadata, apiVersion, and kind from a resource
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

// getCreationTime extracts creation timestamp from a resource summary
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

type APIResource struct {
	Name         string   `json:"name"`
	SingularName string   `json:"singularName"`
	Namespaced   bool     `json:"namespaced"`
	Kind         string   `json:"kind"`
	Verbs        []string `json:"verbs"`
	ShortNames   []string `json:"shortNames,omitempty"`
	APIVersion   string   `json:"apiVersion"`
	Categories   []string `json:"categories,omitempty"`
}

func (h *ResourceHandler) ListAPIResources(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lists, err := h.client.DiscoverResources(ctx)
	if err != nil {
		return response.Error(fmt.Sprintf("failed to discover API resources: %v", err))
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

type KubeContext struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace,omitempty"`
	Current   bool   `json:"current"`
}

func (h *ResourceHandler) ListContexts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contexts, err := h.listKubeContexts()
	if err != nil {
		return response.Error(fmt.Sprintf("failed to list contexts: %v", err))
	}

	result := map[string]interface{}{
		"contexts": contexts,
		"count":    len(contexts),
	}

	return response.JSON(result)
}

func (h *ResourceHandler) listKubeContexts() ([]KubeContext, error) {
	kubeconfig := h.baseConfig.Kubeconfig
	if kubeconfig == "" {
		// Check KUBECONFIG environment variable first
		if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
			kubeconfig = envKubeconfig
		} else {
			// Fall back to default location
			if home := homedir.HomeDir(); home != "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}
	}

	configLoadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	configOverrides := &clientcmd.ConfigOverrides{}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		configLoadingRules,
		configOverrides,
	)

	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	var contexts []KubeContext
	for name, context := range rawConfig.Contexts {
		kubeContext := KubeContext{
			Name:      name,
			Cluster:   context.Cluster,
			User:      context.AuthInfo,
			Namespace: context.Namespace,
			Current:   name == rawConfig.CurrentContext,
		}
		contexts = append(contexts, kubeContext)
	}

	// Sort contexts by name for consistent output, but put current context first
	sort.Slice(contexts, func(i, j int) bool {
		if contexts[i].Current {
			return true
		}
		if contexts[j].Current {
			return false
		}
		return contexts[i].Name < contexts[j].Name
	})

	return contexts, nil
}
