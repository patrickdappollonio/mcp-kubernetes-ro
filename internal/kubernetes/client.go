// Package kubernetes provides a unified client interface for interacting with Kubernetes clusters.
// It wraps the standard Kubernetes client libraries to provide a simplified API for
// read-only operations across resources, logs, metrics, and API discovery.
package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsClient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Client provides a unified interface for read-only Kubernetes operations.
// It wraps multiple Kubernetes client types (clientset, dynamic, discovery, metrics)
// to provide a single interface for all the operations needed by the MCP server.
//
// The client supports:
//   - Resource listing and retrieval using dynamic client
//   - Pod log access with filtering options
//   - Container discovery within pods
//   - API resource discovery for dynamic resource type resolution
//   - Node and pod metrics retrieval (requires metrics-server)
//   - Connectivity testing for startup validation
type Client struct {
	clientset       kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	metricsClient   metricsClient.Interface
	config          *rest.Config
	namespace       string
}

// Config holds the configuration parameters for creating a Kubernetes client.
// It supports both explicit kubeconfig paths and automatic detection from
// environment variables and default locations.
type Config struct {
	// Kubeconfig is the path to the kubeconfig file. If empty, the client will
	// attempt to use the KUBECONFIG environment variable, then ~/.kube/config,
	// and finally fall back to in-cluster configuration.
	Kubeconfig string

	// Namespace is the default namespace for operations. If empty, operations
	// will use the current namespace from the kubeconfig or require explicit
	// namespace specification.
	Namespace string
}

// NewClient creates a new Kubernetes client using the provided configuration.
// It initializes all necessary client interfaces and validates connectivity.
// This is a convenience wrapper around NewClientWithContext with an empty context.
func NewClient(cfg *Config) (*Client, error) {
	return NewClientWithContext(cfg, "")
}

// NewClientWithContext creates a new Kubernetes client using the provided configuration
// and a specific Kubernetes context. This allows for per-operation context switching
// without recreating the entire client.
//
// The context parameter specifies which Kubernetes context from the kubeconfig
// to use. If empty, it uses the current context from the kubeconfig file.
func NewClientWithContext(cfg *Config, context string) (*Client, error) {
	config, err := buildConfig(cfg.Kubeconfig, context)
	if err != nil {
		return nil, fmt.Errorf("failed to build Kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	metricsClient, err := metricsClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %w", err)
	}

	return &Client{
		clientset:       clientset,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		metricsClient:   metricsClient,
		config:          config,
		namespace:       cfg.Namespace,
	}, nil
}

func buildConfig(kubeconfig, context string) (*rest.Config, error) {
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

	if _, err := os.Stat(kubeconfig); os.IsNotExist(err) {
		return rest.InClusterConfig()
	}

	configLoadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	configOverrides := &clientcmd.ConfigOverrides{}

	if context != "" {
		configOverrides.CurrentContext = context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		configLoadingRules,
		configOverrides,
	)

	return clientConfig.ClientConfig()
}

// ListResources retrieves a list of Kubernetes resources of the specified type.
// It supports both namespaced and cluster-scoped resources, with optional filtering
// through the provided ListOptions (label selectors, field selectors, pagination).
//
// The gvr parameter specifies the GroupVersionResource to list.
// The namespace parameter is used for namespaced resources; leave empty for cluster-scoped resources.
// The opts parameter provides filtering and pagination options.
func (c *Client) ListResources(ctx context.Context, gvr schema.GroupVersionResource, namespace string, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	if namespace == "" && c.namespace != "" {
		namespace = c.namespace
	}

	var resourceInterface dynamic.ResourceInterface
	if namespace != "" {
		resourceInterface = c.dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = c.dynamicClient.Resource(gvr)
	}

	return resourceInterface.List(ctx, opts)
}

// GetResource retrieves a specific Kubernetes resource by name and type.
// It works with both namespaced and cluster-scoped resources.
//
// The gvr parameter specifies the GroupVersionResource to retrieve.
// The namespace parameter is required for namespaced resources; leave empty for cluster-scoped resources.
// The name parameter specifies which resource instance to retrieve.
func (c *Client) GetResource(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if namespace == "" && c.namespace != "" {
		namespace = c.namespace
	}

	var resourceInterface dynamic.ResourceInterface
	if namespace != "" {
		resourceInterface = c.dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = c.dynamicClient.Resource(gvr)
	}

	return resourceInterface.Get(ctx, name, metav1.GetOptions{})
}

// DiscoverResources retrieves the list of available API resources from the cluster.
// This is used to understand what resource types are available and their capabilities
// (namespaced vs cluster-scoped, supported verbs, etc.).
func (c *Client) DiscoverResources(ctx context.Context) ([]*metav1.APIResourceList, error) {
	return c.discoveryClient.ServerPreferredResources()
}

// ResolveResourceType converts a user-friendly resource type name to a GroupVersionResource.
// It supports various input formats including plural names, singular names, kinds, and short names.
// For example: "pods", "pod", "Pod", "po" all resolve to the same GVR.
//
// The resourceType parameter can be any recognized name for the resource.
// The apiVersion parameter optionally constrains the search to a specific API version.
//
// Returns a detailed error message with available resource types if the lookup fails.
func (c *Client) ResolveResourceType(resourceType, apiVersion string) (schema.GroupVersionResource, error) {
	lists, err := c.discoveryClient.ServerPreferredResources()
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to discover resources: %w", err)
	}

	// Build a comprehensive mapping of all possible names to their resource info
	type resourceInfo struct {
		gvr        schema.GroupVersionResource
		apiVersion string
	}

	nameToResource := make(map[string]resourceInfo)
	var allResourceNames []string

	for _, list := range lists {
		// Skip if API version is specified and doesn't match
		if apiVersion != "" && list.GroupVersion != apiVersion {
			continue
		}

		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}

		for _, resource := range list.APIResources {
			// Skip subresources (those with '/' in the name)
			if strings.Contains(resource.Name, "/") {
				continue
			}

			gvr := gv.WithResource(resource.Name)
			info := resourceInfo{
				gvr:        gvr,
				apiVersion: list.GroupVersion,
			}

			// Map all possible names (case-insensitive)
			names := []string{
				resource.Name,         // plural name (e.g., "pods")
				resource.SingularName, // singular name (e.g., "pod")
				resource.Kind,         // kind (e.g., "Pod")
			}

			// Add short names
			names = append(names, resource.ShortNames...)

			for _, name := range names {
				if name != "" {
					lowerName := strings.ToLower(name)

					// Prefer exact API version match over others
					if existing, exists := nameToResource[lowerName]; !exists ||
						(apiVersion != "" && existing.apiVersion != apiVersion && info.apiVersion == apiVersion) {
						nameToResource[lowerName] = info
					}

					// Collect for error message (only from specified API version or all if none specified)
					if apiVersion == "" || list.GroupVersion == apiVersion {
						allResourceNames = append(allResourceNames, name)
					}
				}
			}
		}
	}

	// Look up the resource type (case-insensitive)
	if info, found := nameToResource[strings.ToLower(resourceType)]; found {
		return info.gvr, nil
	}

	// Resource not found - provide helpful error message
	errorMsg := fmt.Sprintf("resource type %q not found", resourceType)
	if apiVersion != "" {
		errorMsg += fmt.Sprintf(" in API version %q", apiVersion)
	} else {
		errorMsg += " in any available API version"
	}

	if len(allResourceNames) > 0 {
		// Remove duplicates and sort for better readability
		uniqueNames := make(map[string]bool)
		for _, name := range allResourceNames {
			uniqueNames[name] = true
		}

		var sortedNames []string
		for name := range uniqueNames {
			sortedNames = append(sortedNames, name)
		}

		// Sort the names for consistent, readable output
		sort.Strings(sortedNames)

		if len(sortedNames) > 10 {
			sortedNames = sortedNames[:10]
			errorMsg += fmt.Sprintf(". Available resource types include: %v (and %d more)", sortedNames, len(uniqueNames)-10)
		} else {
			errorMsg += fmt.Sprintf(". Available resource types include: %v", sortedNames)
		}
	}

	return schema.GroupVersionResource{}, errors.New(errorMsg)
}

// LogOptions represents options for retrieving pod logs.
// It provides fine-grained control over which logs to retrieve and how to filter them.
type LogOptions struct {
	// Container specifies which container's logs to retrieve in multi-container pods.
	// If empty, defaults to the first container.
	Container string

	// MaxLines limits the number of log lines to retrieve. If nil, retrieves all logs.
	MaxLines *int64

	// SinceTime retrieves logs newer than this absolute timestamp.
	// Mutually exclusive with SinceSeconds.
	SinceTime *time.Time

	// SinceSeconds retrieves logs newer than this relative duration in seconds.
	// Mutually exclusive with SinceTime.
	SinceSeconds *int64

	// Previous retrieves logs from the previous terminated container instance.
	// Useful for debugging crashed containers.
	Previous bool
}

// GetPodLogs retrieves logs for a specific pod and container with basic filtering options.
// This is a convenience method that wraps GetPodLogsWithOptions for simple use cases.
//
// The namespace parameter specifies the pod's namespace.
// The podName parameter specifies which pod's logs to retrieve.
// The containerName parameter specifies which container (optional for single-container pods).
// The maxLines parameter limits the number of log lines returned.
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName, containerName string, maxLines *int64) (string, error) {
	opts := &LogOptions{
		Container: containerName,
		MaxLines:  maxLines,
	}
	return c.GetPodLogsWithOptions(ctx, namespace, podName, opts)
}

// GetPodLogsWithOptions retrieves logs for a specific pod with comprehensive filtering options.
// It supports time-based filtering, line limits, container selection, and previous container logs.
//
// The namespace parameter specifies the pod's namespace.
// The podName parameter specifies which pod's logs to retrieve.
// The opts parameter provides detailed log retrieval options.
func (c *Client) GetPodLogsWithOptions(ctx context.Context, namespace, podName string, opts *LogOptions) (string, error) {
	if namespace == "" && c.namespace != "" {
		namespace = c.namespace
	}

	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}

	logOptions := &corev1.PodLogOptions{}

	if opts != nil {
		if opts.Container != "" {
			logOptions.Container = opts.Container
		}

		if opts.MaxLines != nil {
			logOptions.TailLines = opts.MaxLines
		}

		if opts.SinceTime != nil {
			sinceTime := metav1.NewTime(*opts.SinceTime)
			logOptions.SinceTime = &sinceTime
		}

		if opts.SinceSeconds != nil {
			logOptions.SinceSeconds = opts.SinceSeconds
		}

		if opts.Previous {
			logOptions.Previous = true
		}
	}

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, logOptions)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer func() {
		_ = podLogs.Close()
	}()

	logBytes, err := io.ReadAll(podLogs)
	if err != nil {
		return "", fmt.Errorf("failed to read pod logs: %w", err)
	}

	return string(logBytes), nil
}

// GetPodContainers returns the list of container names within a specific pod.
// This is useful for identifying which containers are available for log retrieval
// in multi-container pods.
//
// The namespace parameter specifies the pod's namespace.
// The podName parameter specifies which pod to inspect.
func (c *Client) GetPodContainers(ctx context.Context, namespace, podName string) ([]string, error) {
	if namespace == "" && c.namespace != "" {
		namespace = c.namespace
	}

	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	var containers []string
	for _, container := range pod.Spec.Containers {
		containers = append(containers, container.Name)
	}

	return containers, nil
}

// GetNodeMetrics retrieves CPU and memory usage metrics for all nodes in the cluster.
// Requires the metrics-server to be installed and running in the cluster.
func (c *Client) GetNodeMetrics(ctx context.Context) (*metricsv1beta1.NodeMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
}

// GetNodeMetricsWithOptions retrieves node metrics with pagination support.
// This allows for controlled retrieval of large numbers of node metrics.
func (c *Client) GetNodeMetricsWithOptions(ctx context.Context, opts metav1.ListOptions) (*metricsv1beta1.NodeMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, opts)
}

// GetNodeMetricsByName retrieves metrics for a specific node by name.
// Useful when you need metrics for just one node rather than all nodes.
func (c *Client) GetNodeMetricsByName(ctx context.Context, nodeName string) (*metricsv1beta1.NodeMetrics, error) {
	return c.metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, nodeName, metav1.GetOptions{})
}

// GetPodMetrics retrieves CPU and memory usage metrics for all pods across all namespaces.
// Requires the metrics-server to be installed and running in the cluster.
func (c *Client) GetPodMetrics(ctx context.Context) (*metricsv1beta1.PodMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
}

// GetPodMetricsWithOptions retrieves pod metrics with pagination support.
// This allows for controlled retrieval of large numbers of pod metrics.
func (c *Client) GetPodMetricsWithOptions(ctx context.Context, opts metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, opts)
}

// GetPodMetricsByNamespace retrieves metrics for all pods in a specific namespace.
// This is more efficient than cluster-wide retrieval when you only need metrics
// for pods in a particular namespace.
func (c *Client) GetPodMetricsByNamespace(ctx context.Context, namespace string) (*metricsv1beta1.PodMetricsList, error) {
	if namespace == "" && c.namespace != "" {
		namespace = c.namespace
	}
	return c.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
}

// GetPodMetricsByNamespaceWithOptions retrieves namespace-scoped pod metrics with pagination support.
// Combines namespace filtering with pagination for efficient large-scale metrics retrieval.
func (c *Client) GetPodMetricsByNamespaceWithOptions(ctx context.Context, namespace string, opts metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, opts)
}

// GetPodMetricsByName retrieves metrics for a specific pod by name and namespace.
// This is the most efficient method when you need metrics for just one pod.
func (c *Client) GetPodMetricsByName(ctx context.Context, namespace, podName string) (*metricsv1beta1.PodMetrics, error) {
	if namespace == "" && c.namespace != "" {
		namespace = c.namespace
	}
	return c.metricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
}

// TestConnectivity performs a comprehensive connectivity check to verify the cluster
// is reachable and the client has basic permissions. This is called during startup
// to ensure the MCP server can function properly.
//
// The test includes:
//   - API server reachability by getting cluster version
//   - API resource discovery to ensure discovery works
//   - Basic RBAC validation by attempting to list namespaces
//
// Returns a detailed error with troubleshooting guidance if any check fails.
func (c *Client) TestConnectivity(ctx context.Context) error {
	// Test 1: Check if we can reach the API server by getting cluster version
	version, err := c.discoveryClient.ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to get server version: %w", err)
	}

	// Test 2: Try to discover API resources to ensure discovery works
	// Note: This can have warnings (like deprecated APIs) but should not fail connectivity
	resources, err := c.discoveryClient.ServerPreferredResources()
	if err != nil {
		// Check if we got partial results (warnings vs complete failure)
		if len(resources) == 0 {
			return fmt.Errorf("failed to discover API resources: %w", err)
		}
		// If we got some resources, it's likely just warnings about deprecated APIs
		fmt.Fprintf(os.Stderr, "Warning: Some API resources may be deprecated or unavailable: %v\n", err)
	}

	// Test 3: Try a simple API call to ensure we have basic permissions
	namespaces, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("failed to list namespaces (check RBAC permissions): %w", err)
	}

	// Log successful connectivity with some basic cluster info
	fmt.Fprintf(os.Stderr, "✓ Successfully connected to Kubernetes cluster (version: %s, %d namespaces accessible)\n",
		version.String(), len(namespaces.Items))
	return nil
}
