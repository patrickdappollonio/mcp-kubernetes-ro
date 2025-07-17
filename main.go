package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/handlers"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
)

var (
	kubeconfig = flag.String("kubeconfig", "", "Path to kubeconfig file")
	namespace  = flag.String("namespace", "", "Default namespace")
	transport  = flag.String("transport", "stdio", "Transport type: stdio or sse")
	port       = flag.Int("port", 8080, "Port for SSE server (only used with -transport=sse)")
	version    = "dev"
)

func main() {
	flag.Parse()

	kubeConfig := &kubernetes.Config{
		Kubeconfig: *kubeconfig,
		Namespace:  *namespace,
	}

	client, err := kubernetes.NewClient(kubeConfig)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Test connectivity to the cluster to ensure we can operate otherwise
	// prevent the MCP server from starting
	fmt.Fprintln(os.Stderr, "Testing connectivity to Kubernetes cluster...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.TestConnectivity(ctx); err != nil {
		log.Fatalf("Failed to connect to Kubernetes cluster: %v\n\nPlease check:\n- Your kubeconfig file is valid\n- The cluster is accessible\n- You have the necessary RBAC permissions\n- The cluster is running and responding", err)
	}
	fmt.Fprintln(os.Stderr, "Connected to Kubernetes cluster, starting MCP server...")

	resourceHandler := handlers.NewResourceHandler(client, kubeConfig)
	logHandler := handlers.NewLogHandler(client, kubeConfig)
	metricsHandler := handlers.NewMetricsHandler(client, kubeConfig)
	utilsHandler := handlers.NewUtilsHandler()

	s := server.NewMCPServer(
		"mcp-kubernetes-ro",
		version,
		server.WithInstructions(
			"This MCP server provides read-only access to Kubernetes clusters. It can list resources, get resource details, retrieve pod logs, discover API resources, get node and pod metrics, and perform base64 encoding/decoding operations. It is a read-only server and cannot perform any destructive operations. As an AI, feel free to suggest commands to the user that explain how to perform write operations using things like \"kubectl\" and its patch feature.",
		),
	)

	listResourcesTool := mcp.NewTool("list_resources",
		mcp.WithDescription("List any Kubernetes resources by type with optional filtering, sorted newest first. Returns only metadata, apiVersion, and kind for lightweight responses. Use get_resource for full resource details. If you need a list of all resources, use the list_api_resources tool."),
		mcp.WithString("resource_type",
			mcp.Required(),
			mcp.Description("The type of resource to list - use plural form (e.g., \"pods\", \"deployments\", \"services\")"),
		),
		mcp.WithString("api_version",
			mcp.Description("API version for the resource (e.g., \"v1\", \"apps/v1\")"),
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
	)

	getResourceTool := mcp.NewTool("get_resource",
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
			mcp.Description("API version for the resource (e.g., \"v1\", \"apps/v1\")"),
		),
		mcp.WithString("namespace",
			mcp.Description("Target namespace (required for namespaced resources)"),
		),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
		),
	)

	getLogsTool := mcp.NewTool("get_logs",
		mcp.WithDescription("Get pod logs with advanced filtering options including grep patterns, time filtering, and previous logs"),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Pod namespace"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Pod name"),
		),
		mcp.WithString("container",
			mcp.Description("Container name (required for multi-container pods)"),
		),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
		),
		mcp.WithString("max_lines",
			mcp.Description("Maximum number of lines to retrieve"),
		),
		mcp.WithString("grep_include",
			mcp.Description("Include only lines matching these patterns (comma-separated). Works like grep - includes lines containing any of these patterns"),
		),
		mcp.WithString("grep_exclude",
			mcp.Description("Exclude lines matching these patterns (comma-separated). Works like grep -v - excludes lines containing any of these patterns"),
		),
		mcp.WithBoolean("use_regex",
			mcp.Description("Whether to treat grep patterns as regular expressions instead of literal strings"),
		),
		mcp.WithString("since",
			mcp.Description("Return logs newer than this time. Supports durations like \"5m\", \"1h\", \"2h30m\", \"1d\" or absolute times like \"2023-01-01T10:00:00Z\""),
		),
		mcp.WithBoolean("previous",
			mcp.Description("Return logs from the previous terminated container instance (like kubectl logs --previous)"),
		),
	)

	getPodContainersTool := mcp.NewTool("get_pod_containers",
		mcp.WithDescription("List containers in a pod for log access"),
		mcp.WithString("namespace",
			mcp.Required(),
			mcp.Description("Pod namespace"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Pod name"),
		),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
		),
	)

	listAPIResourcesTool := mcp.NewTool("list_api_resources",
		mcp.WithDescription("List available Kubernetes API resources with their details (similar to kubectl api-resources)"),
	)

	listContextsTool := mcp.NewTool("list_contexts",
		mcp.WithDescription("List available Kubernetes contexts from the kubeconfig file"),
	)

	encodeBase64Tool := mcp.NewTool("encode_base64",
		mcp.WithDescription("Encode text data to base64 format"),
		mcp.WithString("data",
			mcp.Required(),
			mcp.Description("Text data to encode"),
		),
	)

	decodeBase64Tool := mcp.NewTool("decode_base64",
		mcp.WithDescription("Decode base64 data to text format"),
		mcp.WithString("data",
			mcp.Required(),
			mcp.Description("Base64 data to decode"),
		),
	)

	getNodeMetricsTool := mcp.NewTool("get_node_metrics",
		mcp.WithDescription("Get node metrics (CPU and memory usage)"),
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
	)

	getPodMetricsTool := mcp.NewTool("get_pod_metrics",
		mcp.WithDescription("Get pod metrics (CPU and memory usage)"),
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
	)

	s.AddTool(listResourcesTool, resourceHandler.ListResources)
	s.AddTool(getResourceTool, resourceHandler.GetResource)
	s.AddTool(getLogsTool, logHandler.GetLogs)
	s.AddTool(getPodContainersTool, logHandler.GetPodContainers)
	s.AddTool(listAPIResourcesTool, resourceHandler.ListAPIResources)
	s.AddTool(listContextsTool, resourceHandler.ListContexts)
	s.AddTool(getNodeMetricsTool, metricsHandler.GetNodeMetrics)
	s.AddTool(getPodMetricsTool, metricsHandler.GetPodMetrics)
	s.AddTool(encodeBase64Tool, utilsHandler.EncodeBase64)
	s.AddTool(decodeBase64Tool, utilsHandler.DecodeBase64)

	switch *transport {
	case "stdio":
		if err := server.ServeStdio(s); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	case "sse":
		sseServer := server.NewSSEServer(s)

		addr := ":" + strconv.Itoa(*port)
		log.Printf("Starting SSE MCP server on %s", addr)
		log.Printf("SSE endpoint: http://localhost%s/sse", addr)
		log.Printf("Message endpoint: http://localhost%s/message", addr)

		if err := http.ListenAndServe(addr, sseServer); err != nil {
			fmt.Printf("SSE server error: %v\n", err)
		}
	default:
		log.Fatalf("Unknown transport type: %s. Supported: stdio, sse", *transport)
	}
}
