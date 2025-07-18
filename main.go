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

	client, err := kubernetes.NewClientWithContext(kubeConfig, "")
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Test connectivity to the cluster to ensure we can operate otherwise
	// prevent the MCP server from starting
	fmt.Fprintln(os.Stderr, "Testing connectivity to Kubernetes cluster...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := client.TestConnectivity(ctx); err != nil {
		cancel()
		log.Fatalf("Failed to connect to Kubernetes cluster: %v\n\nPlease check:\n- Your kubeconfig file is valid\n- The cluster is accessible\n- You have the necessary RBAC permissions\n- The cluster is running and responding", err)
	}
	cancel() // Clean up the context
	fmt.Fprintln(os.Stderr, "Connected to Kubernetes cluster, starting MCP server...")

	// Define tools and handlers
	resourceHandler := handlers.NewResourceHandler(client)
	logHandler := handlers.NewLogHandler(client)
	metricsHandler := handlers.NewMetricsHandler(client)
	utilsHandler := handlers.NewUtilsHandler()

	s := server.NewMCPServer(
		"mcp-kubernetes-ro",
		version,
		server.WithInstructions(
			"This MCP server provides read-only access to Kubernetes clusters. It can list resources, get resource details, retrieve pod logs, discover API resources, get node and pod metrics, and perform base64 encoding/decoding operations.\n\n"+
				"IMPORTANT LIMITATIONS AND GUIDELINES:\n"+
				"• This is a READ-ONLY server - it cannot perform any destructive or write operations\n"+
				"• DO NOT execute commands that modify cluster state through shell commands or kubectl\n"+
				"• Always ask for explicit user permission before suggesting any write operations\n"+
				"• When suggesting write operations, provide kubectl commands as examples rather than executing them\n"+
				"• Focus on observability, debugging, and informational tasks\n"+
				"• Use tools like kubectl get, describe, logs for guidance, but do not execute them directly\n\n"+
				"RECOMMENDED USAGE:\n"+
				"• Use this server to explore and understand cluster state\n"+
				"• Retrieve logs and metrics for troubleshooting\n"+
				"• Discover available resources and their configurations\n"+
				"• Provide insights based on observed cluster data\n"+
				"• Guide users on how to perform write operations safely using kubectl commands\n\n"+
				"When users need to make changes to the cluster, provide them with the appropriate kubectl commands to run manually, such as \"kubectl apply\", \"kubectl patch\", \"kubectl delete\", etc., but do not execute these commands yourself.",
		),
		server.WithLogging(),
	)

	// Register all tools from handlers
	allHandlers := []handlers.ToolRegistrator{
		resourceHandler,
		logHandler,
		metricsHandler,
		utilsHandler,
	}

	// Create tool filter
	filter := toolfilter.NewFilter(*disabledTools)

	// Register tools from handlers
	for _, handler := range allHandlers {
		for i := range handler.GetTools() {
			mcpTool := &handler.GetTools()[i]

			if tool := mcpTool.Tool().Name; filter.IsDisabled(tool) {
				fmt.Fprintf(os.Stderr, "Skipping disabled tool: %q\n", tool)
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

		httpServer := &http.Server{
			Addr:         addr,
			Handler:      sseServer,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		if err := httpServer.ListenAndServe(); err != nil {
			fmt.Printf("SSE server error: %v\n", err)
		}
	default:
		log.Fatalf("Unknown transport type: %s. Supported: stdio, sse", *transport)
	}
}
