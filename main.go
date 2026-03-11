package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/handlers"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/portforward"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/resourcefilter"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/toolfilter"
)

// stringSlice implements flag.Value for a repeatable, comma-separated string flag.
// Each use of the flag appends to the list, and values within a single use
// can be comma-separated. For example:
//
//	-flag=a,b -flag=c → ["a", "b", "c"]
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	for _, v := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		*s = append(*s, v)
	}
	return nil
}

var (
	kubeconfig           = flag.String("kubeconfig", "", "Path to kubeconfig file")
	namespace            = flag.String("namespace", "", "Default namespace")
	transport            = flag.String("transport", "stdio", "Transport type: stdio or sse")
	port                 = flag.Int("port", 8080, "Port for SSE server (only used with -transport=sse)")
	disabledTools        stringSlice
	disabledResources    stringSlice
	enablePortForwarding = flag.Bool("enable-port-forwarding", false, "Enable port forwarding tools (start_port_forward, stop_port_forward, list_port_forwards)")
	version              = "dev"
)

func init() {
	flag.Var(&disabledTools, "disabled-tools", "Tool names to disable (repeatable, comma-separated)")
	flag.Var(&disabledResources, "disabled-resources", "Resources to disable (repeatable, comma-separated, e.g. secrets or core/v1/secrets)")
}

// resolveEnvSlice appends values from environment variables to a stringSlice
// if the env var is set. This allows both flag and env var sources to contribute.
func resolveEnvSlice(s *stringSlice, envVars ...string) {
	for _, key := range envVars {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			_ = s.Set(value)
			return // use first set env var only
		}
	}
}

func main() {
	flag.Parse()

	// Merge environment variables into flag values
	resolveEnvSlice(&disabledTools, "MCP_KUBERNETES_RO_DISABLED_TOOLS", "DISABLED_TOOLS")
	resolveEnvSlice(&disabledResources, "MCP_KUBERNETES_RO_DISABLED_RESOURCES")

	// Resolve port forwarding flag from CLI or environment variables
	portForwardingEnabled := *enablePortForwarding
	if !portForwardingEnabled {
		for _, key := range []string{"MCP_KUBERNETES_RO_ENABLE_PORT_FORWARDING", "ENABLE_PORT_FORWARDING"} {
			if val := strings.TrimSpace(os.Getenv(key)); val != "" {
				portForwardingEnabled = strings.EqualFold(val, "true") || val == "1" || strings.EqualFold(val, "yes")
				break
			}
		}
	}

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

	// Create resource filter for disabled resources, using the client to
	// resolve user-friendly names (singular, kind, short names) to canonical GVRs
	resFilter, err := resourcefilter.NewFilter(strings.Join(disabledResources, ","), client)
	if err != nil {
		log.Fatalf("Failed to parse disabled resources: %v", err)
	}
	if resFilter.HasDisabledResources() {
		for _, res := range resFilter.GetDisabledResources() {
			fmt.Fprintf(os.Stderr, "Disabling access to resource: %q\n", res)
		}
	}

	// Define tools and handlers
	resourceHandler := handlers.NewResourceHandler(client, resFilter)
	logHandler := handlers.NewLogHandler(client)
	metricsHandler := handlers.NewMetricsHandler(client)
	utilsHandler := handlers.NewUtilsHandler()

	// Create port-forward manager (may be nil if not enabled)
	var pfManager *portforward.Manager
	if portForwardingEnabled {
		pfManager = portforward.NewManager()
		fmt.Fprintln(os.Stderr, "Port forwarding tools enabled")

		if *transport == "sse" {
			fmt.Fprintln(os.Stderr, "WARNING: Port forwarding with SSE mode — forwarded ports bind to this server's local interface, not the client's machine. Remote clients will need to expose or tunnel those ports to access forwarded services.")
		}
	}

	// Build server instructions
	instructions := "This MCP server provides read-only access to Kubernetes clusters. It can list resources, get resource details, retrieve pod logs, discover API resources, get node and pod metrics, and perform base64 encoding/decoding operations.\n\n" +
		"IMPORTANT LIMITATIONS AND GUIDELINES:\n" +
		"• This is a READ-ONLY server - it cannot perform any destructive or write operations\n" +
		"• DO NOT execute commands that modify cluster state through shell commands or kubectl\n" +
		"• Always ask for explicit user permission before suggesting any write operations\n" +
		"• When suggesting write operations, provide kubectl commands as examples rather than executing them\n" +
		"• Focus on observability, debugging, and informational tasks\n" +
		"• Use tools like kubectl get, describe, logs for guidance, but do not execute them directly\n\n" +
		"RECOMMENDED USAGE:\n" +
		"• Use this server to explore and understand cluster state\n" +
		"• Retrieve logs and metrics for troubleshooting\n" +
		"• Discover available resources and their configurations\n" +
		"• Provide insights based on observed cluster data\n" +
		"• Guide users on how to perform write operations safely using kubectl commands\n\n" +
		"When users need to make changes to the cluster, provide them with the appropriate kubectl commands to run manually, such as \"kubectl apply\", \"kubectl patch\", \"kubectl delete\", etc., but do not execute these commands yourself."

	if portForwardingEnabled {
		instructions += "\n\nPORT FORWARDING:\n" +
			"• Port forwarding tools are enabled. Use start_port_forward to establish tunnels to pod ports for debugging.\n" +
			"• Port forwarding does not modify cluster state — it creates local network tunnels to existing pod ports.\n" +
			"• Use list_port_forwards to see active sessions and stop_port_forward to terminate them.\n" +
			"• Each session can forward multiple ports simultaneously."
	}

	s := server.NewMCPServer(
		"mcp-kubernetes-ro",
		version,
		server.WithInstructions(instructions),
		server.WithLogging(),
	)

	// Register all tools from handlers
	allHandlers := []handlers.ToolRegistrator{
		resourceHandler,
		logHandler,
		metricsHandler,
		utilsHandler,
	}

	if portForwardingEnabled {
		portForwardHandler := handlers.NewPortForwardHandler(client, pfManager)
		allHandlers = append(allHandlers, portForwardHandler)
	}

	// Create tool filter
	filter := toolfilter.NewFilterFromList(disabledTools)

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

	// Set up graceful shutdown for port forwarding
	if portForwardingEnabled && pfManager != nil {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Fprintln(os.Stderr, "Shutting down, stopping all port forwards...")
			pfManager.StopAll()
			os.Exit(0)
		}()
	}

	switch *transport {
	case "stdio":
		log.Printf("Starting MCP server with stdio transport")

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
