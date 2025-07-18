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

	"github.com/mark3labs/mcp-go/server"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/handlers"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/toolfilter"
)

var (
	kubeconfig    = flag.String("kubeconfig", "", "Path to kubeconfig file")
	namespace     = flag.String("namespace", "", "Default namespace")
	transport     = flag.String("transport", "stdio", "Transport type: stdio or sse")
	port          = flag.Int("port", 8080, "Port for SSE server (only used with -transport=sse)")
	disabledTools = flag.String("disabled-tools", "", "Comma-separated list of tool names to disable")
	version       = "dev"
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

	// Create tool filter
	filter := toolfilter.NewFilter(*disabledTools)

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

	// Register all tools from handlers
	allHandlers := []handlers.ToolRegistrator{
		resourceHandler,
		logHandler,
		metricsHandler,
		utilsHandler,
	}

	for _, handler := range allHandlers {
		for _, mcpTool := range handler.GetTools() {
			toolName := mcpTool.Tool().Name
			if filter.IsDisabled(toolName) {
				fmt.Fprintf(os.Stderr, "Skipping disabled tool: %q\n", toolName)
				continue
			}
			s.AddTool(mcpTool.Tool(), mcpTool.Handler())
		}
	}

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
