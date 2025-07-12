package kubernetes

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

type Client struct {
	clientset       kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	metricsClient   metricsClient.Interface
	config          *rest.Config
	namespace       string
}

type Config struct {
	Kubeconfig string
	Namespace  string
}

func NewClient(cfg *Config) (*Client, error) {
	return NewClientWithContext(cfg, "")
}

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

func (c *Client) DiscoverResources(ctx context.Context) ([]*metav1.APIResourceList, error) {
	return c.discoveryClient.ServerPreferredResources()
}

func (c *Client) ResolveResourceType(resourceType, apiVersion string) (schema.GroupVersionResource, error) {
	lists, err := c.discoveryClient.ServerPreferredResources()
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to discover resources: %w", err)
	}

	for _, list := range lists {
		if apiVersion != "" && list.GroupVersion != apiVersion {
			continue
		}

		for _, resource := range list.APIResources {
			if resource.Name == resourceType || resource.Kind == resourceType {
				gv, err := schema.ParseGroupVersion(list.GroupVersion)
				if err != nil {
					continue
				}
				return gv.WithResource(resource.Name), nil
			}
		}
	}

	return schema.GroupVersionResource{}, fmt.Errorf("resource type %q not found", resourceType)
}

// LogOptions represents options for retrieving pod logs
type LogOptions struct {
	Container    string
	MaxLines     *int64
	SinceTime    *time.Time
	SinceSeconds *int64
	Previous     bool
}

func (c *Client) GetPodLogs(ctx context.Context, namespace, podName, containerName string, maxLines *int64) (string, error) {
	opts := &LogOptions{
		Container: containerName,
		MaxLines:  maxLines,
	}
	return c.GetPodLogsWithOptions(ctx, namespace, podName, opts)
}

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

// GetNodeMetrics retrieves metrics for all nodes in the cluster
func (c *Client) GetNodeMetrics(ctx context.Context) (*metricsv1beta1.NodeMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
}

// GetNodeMetricsWithOptions retrieves metrics for all nodes with pagination support
func (c *Client) GetNodeMetricsWithOptions(ctx context.Context, opts metav1.ListOptions) (*metricsv1beta1.NodeMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, opts)
}

// GetNodeMetricsByName retrieves metrics for a specific node
func (c *Client) GetNodeMetricsByName(ctx context.Context, nodeName string) (*metricsv1beta1.NodeMetrics, error) {
	return c.metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, nodeName, metav1.GetOptions{})
}

// GetPodMetrics retrieves metrics for all pods in the cluster
func (c *Client) GetPodMetrics(ctx context.Context) (*metricsv1beta1.PodMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
}

// GetPodMetricsWithOptions retrieves metrics for all pods with pagination support
func (c *Client) GetPodMetricsWithOptions(ctx context.Context, opts metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, opts)
}

// GetPodMetricsByNamespace retrieves metrics for pods in a specific namespace
func (c *Client) GetPodMetricsByNamespace(ctx context.Context, namespace string) (*metricsv1beta1.PodMetricsList, error) {
	if namespace == "" && c.namespace != "" {
		namespace = c.namespace
	}
	return c.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
}

// GetPodMetricsByNamespaceWithOptions retrieves metrics for pods in a specific namespace with pagination support
func (c *Client) GetPodMetricsByNamespaceWithOptions(ctx context.Context, namespace string, opts metav1.ListOptions) (*metricsv1beta1.PodMetricsList, error) {
	return c.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, opts)
}

// GetPodMetricsByName retrieves metrics for a specific pod
func (c *Client) GetPodMetricsByName(ctx context.Context, namespace, podName string) (*metricsv1beta1.PodMetrics, error) {
	if namespace == "" && c.namespace != "" {
		namespace = c.namespace
	}
	return c.metricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
}

// TestConnectivity performs a basic connectivity check to verify the cluster is reachable
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
