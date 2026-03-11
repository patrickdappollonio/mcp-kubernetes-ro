# Kubernetes read-only MCP server

[![Github Downloads](https://img.shields.io/github/downloads/patrickdappollonio/mcp-kubernetes-ro/total?color=orange&label=github%20downloads)](https://github.com/patrickdappollonio/mcp-kubernetes-ro/releases)

<img src="kubernetes-ro.png" alt="Kubernetes read-only MCP server" width="200" height="200" align="right"> `mcp-kubernetes-ro` is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction) server providing read-only access to Kubernetes clusters for AI assistants. It enables AI models to list resources, get resource details, retrieve pod logs, discover API resources, and perform base64 encoding/decoding operations - all while maintaining security through read-only access.

The server leverages your local `kubectl` configuration (even when `kubectl` is not required to be installed) and provides a safe, read-only interface to your Kubernetes clusters, preventing any destructive operations while allowing comprehensive cluster inspection and troubleshooting capabilities.

## Features

- **Does not require `kubectl`**: The MCP server uses your local `kubectl` configuration to connect to your Kubernetes cluster but not the binary, so it works even when `kubectl` is not installed on your machine.
- **Resource Listing**: List any Kubernetes resources by type with optional filtering by labels, fields, and namespaces
- **Resource Details**: Get complete details for specific Kubernetes resources
- **Pod Logs**: Retrieve pod logs with advanced filtering options including grep patterns, time filtering, and previous logs
- **Container Discovery**: List containers within pods for targeted log access
- **API Discovery**: Discover available Kubernetes API resources and their capabilities
- **Base64 Utilities**: Encode and decode base64 data for Kubernetes secrets and configurations
- **Multiple Transport Modes**: Support for both stdio and Server-Sent Events (SSE) communication
- **Read-Only Security**: Complete prevention of destructive operations while maintaining full inspection capabilities
- **Resource Access Control**: Disable access to specific Kubernetes resource types (e.g., Secrets) to prevent AI agents from querying sensitive data
- **Namespace Support**: Work with specific namespaces or cluster-wide resources
- **Advanced Filtering**: Support for label selectors, field selectors, and pagination
- **Per-Command Context**: Specify different Kubernetes contexts for individual commands
- **Environment Variable Support**: Automatic detection of KUBECONFIG environment variable
- **Port Forwarding (opt-in)**: Establish tunneled connections to pod ports for debugging, with support for multiple ports per session
- **Startup Connectivity Check**: Automatic validation of cluster connectivity and basic permissions on startup

## Installation

Feel free to grab a pre-built binary from the [releases page](https://github.com/patrickdappollonio/mcp-kubernetes-ro/releases).

Alternatively, you can use Homebrew in macOS or Linux to install it:

```bash
brew install patrickdappollonio/tap/mcp-kubernetes-ro
```

You can also use it as an NPM package: just make sure the configuration is given to your AI agent:

```bash
npx -y @patrickdappollonio/mcp-kubernetes-ro
```

Finally, Docker users can use the prebuilt image from the GitHub Container Registry:

```bash
docker pull ghcr.io/patrickdappollonio/mcp-kubernetes-ro:latest
```

### Editor Configuration

Add the following configuration to your editor's settings to use `mcp-kubernetes-ro`:

```json5
{
  "mcpServers": {
    "kubernetes-ro": {
      "command": "mcp-kubernetes-ro",
      "args": [
        // Uncomment and modify as needed:
        // "--kubeconfig=/path/to/kubeconfig",
        // "--namespace=default",
        // "--transport=stdio",
        // "--port=8080",
        // "--disabled-tools=get_logs,decode_base64",
        // "--disabled-resources=secrets"
      ],
      "env": {
        // Set KUBECONFIG environment variable if needed:
        // "KUBECONFIG": "/path/to/kubeconfig",
        // Set MCP_KUBERNETES_RO_DISABLED_TOOLS environment variable if needed:
        // "MCP_KUBERNETES_RO_DISABLED_TOOLS": "get_logs,decode_base64",
        // Or use generic DISABLED_TOOLS environment variable:
        // "DISABLED_TOOLS": "get_logs,decode_base64",
        // Disable access to specific resource types:
        // "MCP_KUBERNETES_RO_DISABLED_RESOURCES": "secrets,configmaps"
      }
    }
  }
}
```

You can use `mcp-kubernetes-ro` directly from your `$PATH` as shown above, or provide the full path to the binary (e.g., `/path/to/mcp-kubernetes-ro`).

You can also simplify the installation process by using it as an `npx` package:

```json5
{
  "mcpServers": {
    "kubernetes-ro": {
      "command": "npx",
      "args": [
        "-y",
        "@patrickdappollonio/mcp-kubernetes-ro"
        // Uncomment and modify as needed:
        // "--kubeconfig=/path/to/kubeconfig",
        // "--namespace=default",
        // "--transport=stdio",
        // "--port=8080",
        // "--disabled-tools=get_logs,decode_base64",
        // "--disabled-resources=secrets"
      ],
      "env": {
        // Set KUBECONFIG environment variable if needed:
        // "KUBECONFIG": "/path/to/kubeconfig",
        // Set MCP_KUBERNETES_RO_DISABLED_TOOLS environment variable if needed:
        // "MCP_KUBERNETES_RO_DISABLED_TOOLS": "get_logs,decode_base64",
        // Or use generic DISABLED_TOOLS environment variable:
        // "DISABLED_TOOLS": "get_logs,decode_base64",
        // Disable access to specific resource types:
        // "MCP_KUBERNETES_RO_DISABLED_RESOURCES": "secrets,configmaps"
      }
    }
  }
}
```

And this is how to leverage the Docker image instead:

```json5
{
  "mcpServers": {
    "kubernetes-ro": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e", "KUBECONFIG=/root/.kube/config",
        "-v", "/path/to/kubeconfig:/root/.kube/config",
        "ghcr.io/patrickdappollonio/mcp-kubernetes-ro"
        // Place additional flags here, like:
        // "--disabled-tools=get_logs,decode_base64",
        // "--disabled-resources=secrets"
      ],
      "env": {
        // Set KUBECONFIG environment variable if needed:
        // "KUBECONFIG": "/path/to/kubeconfig",
        // Set MCP_KUBERNETES_RO_DISABLED_TOOLS environment variable if needed:
        // "MCP_KUBERNETES_RO_DISABLED_TOOLS": "get_logs,decode_base64",
        // Or use generic DISABLED_TOOLS environment variable:
        // "DISABLED_TOOLS": "get_logs,decode_base64",
        // Disable access to specific resource types:
        // "MCP_KUBERNETES_RO_DISABLED_RESOURCES": "secrets,configmaps"
      }
    },
  }
}
```

Do note that you'll need to mount your kubeconfig file into the container, and either set the `KUBECONFIG` environment variable to the path of the mounted file, or use the `--kubeconfig` flag to set it.

### Prerequisites

- A valid Kubernetes configuration file (typically `~/.kube/config`)
- Valid credentials and cluster access (kubectl binary is not required)
- Appropriate RBAC permissions for read operations
- **Metrics Server** (required for metrics tools): For metrics functionality (`get_node_metrics`, `get_pod_metrics`), the metrics-server must be installed in your cluster. If not available, these tools will return an error message.

## Available MCP Tools

There are **10 tools** available by default, plus **3 additional tools** when port forwarding is enabled:

- **`list_resources`**: List any Kubernetes resources by type with optional filtering, sorted newest first. `metadata.managedFields` is omitted by default unless `include_managed_fields=true`
- **`get_resource`**: Get specific resource details. `metadata.managedFields` is omitted by default unless `include_managed_fields=true`
- **`get_logs`**: Get pod logs with advanced filtering options including grep patterns, time filtering, and previous logs
- **`get_pod_containers`**: List containers in a pod for log access
- **`list_api_resources`**: List available Kubernetes API resources with their details (similar to kubectl api-resources)
- **`list_contexts`**: List available Kubernetes contexts from the kubeconfig file
- **`get_node_metrics`**: Get node metrics (CPU and memory usage)
- **`get_pod_metrics`**: Get pod metrics (CPU and memory usage)
- **`encode_base64`**: Encode text data to base64 format
- **`decode_base64`**: Decode base64 data to text format
- **`start_port_forward`** *(opt-in)*: Start port forwarding to a pod with one or more port mappings
- **`stop_port_forward`** *(opt-in)*: Stop an active port-forwarding session by ID
- **`list_port_forwards`** *(opt-in)*: List all active port-forwarding sessions

## Tool Management

### Disabling Tools

You can disable specific tools using the `--disabled-tools` flag or the `MCP_KUBERNETES_RO_DISABLED_TOOLS` / `DISABLED_TOOLS` environment variables. The flag is repeatable and accepts comma-separated values:

```bash
# Comma-separated
mcp-kubernetes-ro --disabled-tools=get_logs,decode_base64

# Repeated flags
mcp-kubernetes-ro --disabled-tools=get_logs --disabled-tools=decode_base64

# Using environment variable
export MCP_KUBERNETES_RO_DISABLED_TOOLS=get_logs,decode_base64
mcp-kubernetes-ro
```

Values from flags and environment variables are merged. If the `MCP_KUBERNETES_RO_DISABLED_TOOLS` env var is not set, `DISABLED_TOOLS` is used as a fallback.

When a tool is disabled, it will not be registered with the MCP server and will not appear in the available tools list. A message will be logged to stderr indicating which tools have been skipped.

**Available tool names for disabling:**
- `list_resources`
- `get_resource`
- `get_logs`
- `get_pod_containers`
- `list_api_resources`
- `list_contexts`
- `get_node_metrics`
- `get_pod_metrics`
- `encode_base64`
- `decode_base64`
- `start_port_forward` *(only when port forwarding is enabled)*
- `stop_port_forward` *(only when port forwarding is enabled)*
- `list_port_forwards` *(only when port forwarding is enabled)*

### Disabling Access to Specific Resources

You can prevent AI agents from querying specific Kubernetes resource types using the `--disabled-resources` flag or the `MCP_KUBERNETES_RO_DISABLED_RESOURCES` environment variable. This is particularly useful for preventing access to sensitive resources like Secrets.

Resources can be specified by name (singular, plural, kind, or short name) or as a full `group/version/resource` triple. The `core` keyword is used as an alias for the Kubernetes core API group. All names are resolved against the cluster's discovery API at startup, so singular names, kind names, and short names are all accepted:

```bash
# By resource name (resolved via the cluster's discovery API)
mcp-kubernetes-ro --disabled-resources=secrets

# Singular, kind, and short names all work
mcp-kubernetes-ro --disabled-resources=secret      # singular
mcp-kubernetes-ro --disabled-resources=Secret      # kind
mcp-kubernetes-ro --disabled-resources=cm          # short name for configmaps

# Full group/version/resource format
mcp-kubernetes-ro --disabled-resources=core/v1/secrets

# Multiple resources (comma-separated or repeated flags)
mcp-kubernetes-ro --disabled-resources=secrets,configmaps
mcp-kubernetes-ro --disabled-resources=secrets --disabled-resources=configmaps

# Non-core API groups
mcp-kubernetes-ro --disabled-resources=apps/v1/deployments

# Using environment variable
export MCP_KUBERNETES_RO_DISABLED_RESOURCES=secrets,configmaps
mcp-kubernetes-ro
```

When a disabled resource is queried via `list_resources` or `get_resource`, the server returns a clear error:

```
access to resource "secrets" (core/v1/secrets) is disabled by configuration and cannot be queried
```

Disabled resources are also hidden from `list_api_resources` output, so AI agents won't discover them as available.

If a resource name cannot be resolved against the cluster (e.g., a typo or a CRD that doesn't exist), the server will refuse to start with a descriptive error — ensuring disabled resources always take effect.

## Running Modes

### Standard (stdio) Mode

By default, `mcp-kubernetes-ro` runs in stdio mode, which is suitable for integration with editors and other tools that communicate via standard input/output.

```bash
mcp-kubernetes-ro
```

### Server-Sent Events (SSE) Mode

Alternatively, you can run `mcp-kubernetes-ro` as an HTTP server with SSE support for web-based integrations:

```bash
mcp-kubernetes-ro --transport=sse --port=8080
```

In SSE mode, the server will listen on the specified port (default: 8080) and provide the same MCP tools over HTTP using Server-Sent Events. This is useful for web applications or environments where stdio communication isn't practical.

## Configuration Options

The following command-line flags are available to configure the MCP server:

### Kubernetes Configuration
- `--kubeconfig=PATH`: Path to kubeconfig file (defaults to `KUBECONFIG` environment variable, then `~/.kube/config`)
- `--namespace=NAME`: Default namespace for operations (defaults to current namespace)

### Transport Options
- `--transport=TYPE`: Transport type: `stdio` or `sse` (default: `stdio`)
- `--port=PORT`: Port for SSE server (default: 8080, only used with `--transport=sse`)

### Tool and Resource Management
- `--disabled-tools=NAMES`: Tool names to disable, repeatable and comma-separated (optional)
- `--disabled-resources=RESOURCES`: Resource types to block, repeatable and comma-separated (optional). Accepts resource names (`secrets`, `deploy`, `cm`) or full specs (`core/v1/secrets`, `apps/v1/deployments`)
- `MCP_KUBERNETES_RO_DISABLED_TOOLS`: Environment variable for disabled tools (merged with flag values, fallback: `DISABLED_TOOLS`)
- `MCP_KUBERNETES_RO_DISABLED_RESOURCES`: Environment variable for disabled resources (merged with flag values)

### Port Forwarding
- `--enable-port-forwarding`: Enable port forwarding tools (disabled by default)
- `MCP_KUBERNETES_RO_ENABLE_PORT_FORWARDING`: App-specific environment variable (set to `true`, `1`, or `yes`)
- `ENABLE_PORT_FORWARDING`: Generic environment variable (set to `true`, `1`, or `yes`)

### Context Configuration

The server supports per-command context. This provides more flexibility when working with multiple Kubernetes clusters or contexts within the same `$KUBECONFIG` file.

**Configuration Priority:**
1. **Command-level context**: Use the `context` parameter in individual tool calls
2. **Kubeconfig default**: Use the current context specified in your kubeconfig file

**Kubeconfig Resolution Priority:**
1. **Command-line flag**: `--kubeconfig` parameter
2. **Environment variable**: `KUBECONFIG` environment variable
3. **Default path**: `~/.kube/config`
4. **In-cluster config**: Automatic detection when running inside a Kubernetes pod

**Examples:**
```json
{
  "resource_type": "pods",
  "namespace": "default",
  "context": "production-cluster"
}
```

This approach allows you to:
- Use different contexts for different operations in the same session
- Switch contexts per command without restarting the server
- Maintain compatibility with existing kubeconfig setups

## Tool Usage Documentation

### List Resources

Lists any Kubernetes resources by type with optional filtering, sorted newest first.

**Arguments:**
- `resource_type` (required): The type of resource to list - use plural form (e.g., 'pods', 'deployments', 'services')
- `api_version` (optional): API version for the resource (e.g., 'v1', 'apps/v1')
- `namespace` (optional): Target namespace (leave empty for cluster-scoped resources)
- `context` (optional): Kubernetes context to use (defaults to current context from kubeconfig)
- `label_selector` (optional): Label selector to filter resources (e.g., 'app=nginx,version=1.0')
- `field_selector` (optional): Field selector to filter resources (e.g., 'status.phase=Running')
- `limit` (optional): Maximum number of resources to return (defaults to all)
- `continue` (optional): Continue token for pagination (from previous response)

**Example:**
```json
{
  "resource_type": "pods",
  "namespace": "default",
  "context": "production",
  "label_selector": "app=nginx"
}
```

### Get Resource

Gets specific resource details with complete configuration.

**Arguments:**
- `resource_type` (required): The type of resource to get
- `name` (required): Resource name
- `api_version` (optional): API version for the resource (e.g., 'v1', 'apps/v1')
- `namespace` (optional): Target namespace (required for namespaced resources)
- `context` (optional): Kubernetes context to use (defaults to current context from kubeconfig)

**Example:**
```json
{
  "resource_type": "deployment",
  "name": "nginx-deployment",
  "namespace": "default",
  "context": "production"
}
```

### Get Logs

Gets pod logs with advanced filtering options including grep patterns, time filtering, and previous logs.

**Arguments:**
- `namespace` (required): Pod namespace
- `name` (required): Pod name
- `container` (optional): Container name (required for multi-container pods)
- `context` (optional): Kubernetes context to use (defaults to current context from kubeconfig)
- `max_lines` (optional): Maximum number of lines to retrieve
- `grep_include` (optional): Include only lines matching these patterns (comma-separated). Works like grep - includes lines containing any of these patterns
- `grep_exclude` (optional): Exclude lines matching these patterns (comma-separated). Works like grep -v - excludes lines containing any of these patterns
- `use_regex` (optional): Whether to treat grep patterns as regular expressions instead of literal strings
- `since` (optional): Return logs newer than this time. Supports durations like "5m", "1h", "2h30m", "1d" or absolute times like "2023-01-01T10:00:00Z"
- `previous` (optional): Return logs from the previous terminated container instance (like kubectl logs --previous)

**Example:**
```json
{
  "namespace": "default",
  "name": "nginx-pod-12345",
  "container": "nginx",
  "context": "production",
  "max_lines": "100",
  "grep_include": "error,warning",
  "since": "5m"
}
```

### Get Pod Containers

Lists containers in a pod for log access.

**Arguments:**
- `namespace` (required): Pod namespace
- `name` (required): Pod name
- `context` (optional): Kubernetes context to use (defaults to current context from kubeconfig)

**Example:**
```json
{
  "namespace": "default",
  "name": "nginx-pod-12345",
  "context": "production"
}
```

### List API Resources

Lists available Kubernetes API resources with their details (similar to kubectl api-resources).

**Arguments:**
- None required

**Example:**
```json
{}
```

### List Contexts

Lists available Kubernetes contexts from the kubeconfig file. This is useful for discovering what contexts are available for use with the `context` parameter in other tools.

**Arguments:**
- None required

**Example:**
```json
{}
```

**Example Response:**
```json
{
  "contexts": [
    {
      "name": "production",
      "cluster": "prod-cluster",
      "user": "prod-user",
      "namespace": "default",
      "current": true
    },
    {
      "name": "staging",
      "cluster": "staging-cluster",
      "user": "staging-user",
      "namespace": "staging",
      "current": false
    }
  ],
  "count": 2
}
```

### Get Node Metrics

Gets node metrics (CPU and memory usage) from the metrics server. Results are sorted by timestamp (newest first) for consistent ordering and pagination, since the built-in metrics server endpoint does not support needle-based pagination.

**Arguments:**
- `node_name` (optional): Specific node name to get metrics for. If not provided, returns metrics for all nodes.
- `context` (optional): Kubernetes context to use (defaults to current context from kubeconfig)
- `limit` (optional): Maximum number of node metrics to return. If not provided, returns all available metrics.
- `continue` (optional): Continue token for pagination (from previous response).

**Error Handling:**
- If the metrics server is not available, returns an error message
- Detects common metrics server errors and provides specific guidance

**Example:**
```json
{
  "node_name": "worker-node-1",
  "context": "production",
  "limit": 5
}
```

**Example Response (Single Node):**
```json
{
  "kind": "NodeMetrics",
  "apiVersion": "metrics.k8s.io/v1beta1",
  "metadata": {
    "name": "worker-node-1",
    "creationTimestamp": "2023-01-01T12:00:00Z"
  },
  "timestamp": "2023-01-01T12:00:00Z",
  "window": "10.062s",
  "usage": {
    "cpu": "137m",
    "memory": "1368128Ki"
  }
}
```

**Example Response (List with Pagination):**
```json
{
  "kind": "NodeMetricsList",
  "apiVersion": "metrics.k8s.io/v1beta1",
  "count": 5,
  "items": [
    {
      "kind": "NodeMetrics",
      "metadata": { "name": "node-1" },
      "timestamp": "2023-01-01T12:00:00Z",
      "usage": { "cpu": "137m", "memory": "1368128Ki" }
    }
  ],
  "continue": "eyJvZmZzZXQiOjUsInR5cGUiOiJub2RlIiwibmFtZXNwYWNlIjoiIn0="
}
```

### Get Pod Metrics

Gets pod metrics (CPU and memory usage) from the metrics server. Results are sorted by timestamp (newest first) for consistent ordering and pagination, since the built-in metrics server endpoint does not support needle-based pagination.

**Arguments:**
- `namespace` (optional): Namespace to get pod metrics from. If not provided, returns metrics for all pods in all namespaces.
- `pod_name` (optional): Specific pod name to get metrics for. Requires `namespace` if specified.
- `context` (optional): Kubernetes context to use (defaults to current context from kubeconfig)
- `limit` (optional): Maximum number of pod metrics to return. If not provided, returns all available metrics.
- `continue` (optional): Continue token for pagination (from previous response).

**Error Handling:**
- If the metrics server is not available, returns an error message
- Detects common metrics server errors and provides specific guidance
- Validates that `namespace` is provided when `pod_name` is specified

**Pagination Notes:**
- Continue tokens are context-aware and reset if the namespace context changes
- Client-side pagination is implemented for consistent ordering and filtering

**Example (Specific Pod):**
```json
{
  "namespace": "kube-system",
  "pod_name": "metrics-server-557ff575fb-9dcl4",
  "context": "production"
}
```

**Example (With Pagination):**
```json
{
  "namespace": "kube-system",
  "context": "production",
  "limit": 10,
  "continue": "eyJvZmZzZXQiOjEwLCJ0eXBlIjoicG9kIiwibmFtZXNwYWNlIjoia3ViZS1zeXN0ZW0ifQ=="
}
```

**Example Response (Single Pod):**
```json
{
  "kind": "PodMetrics",
  "apiVersion": "metrics.k8s.io/v1beta1",
  "metadata": {
    "name": "metrics-server-557ff575fb-9dcl4",
    "namespace": "kube-system",
    "creationTimestamp": "2023-01-01T12:00:00Z"
  },
  "timestamp": "2023-01-01T12:00:00Z",
  "window": "18.888s",
  "containers": [
    {
      "name": "metrics-server",
      "usage": {
        "cpu": "8020419n",
        "memory": "48164Ki"
      }
    }
  ]
}
```

**Example Response (List with Pagination):**
```json
{
  "kind": "PodMetricsList",
  "apiVersion": "metrics.k8s.io/v1beta1",
  "namespace": "kube-system",
  "count": 10,
  "items": [
    {
      "kind": "PodMetrics",
      "metadata": { "name": "pod-1", "namespace": "kube-system" },
      "timestamp": "2023-01-01T12:00:00Z",
      "containers": [
        {
          "name": "container-1",
          "usage": { "cpu": "8020419n", "memory": "48164Ki" }
        }
      ]
    }
  ],
  "continue": "eyJvZmZzZXQiOjIwLCJ0eXBlIjoicG9kIiwibmFtZXNwYWNlIjoia3ViZS1zeXN0ZW0ifQ=="
}
```

### Encode Base64

Encodes text data to base64 format.

**Arguments:**
- `data` (required): Text data to encode

**Example:**
```json
{
  "data": "username:password"
}
```

### Decode Base64

Decodes base64 data to text format.

**Arguments:**
- `data` (required): Base64 data to decode

**Example:**
```json
{
  "data": "dXNlcm5hbWU6cGFzc3dvcmQ="
}
```

### Port Forwarding (opt-in)

Port forwarding is **disabled by default** because it goes beyond read-only operations. While it does not modify any cluster state (no resources are created, updated, or deleted), it establishes active network tunnels from your local machine to pod ports. This means traffic can flow through those tunnels, which could interact with the running application — for example, hitting an HTTP endpoint, connecting to a database, or triggering side effects in the target service. For this reason, port forwarding must be explicitly enabled.

Enable it with the `--enable-port-forwarding` flag or by setting the `MCP_KUBERNETES_RO_ENABLE_PORT_FORWARDING` environment variable to `true`.

> [!WARNING]
> Port forwarding can target **any pod** in the cluster that your kubeconfig credentials have access to, including infrastructure pods. If the Kubernetes API server itself is running as a pod (e.g., in self-hosted or certain managed setups), an AI agent could theoretically forward to it. While port forwarding alone **does not grant additional privileges** — you still need valid credentials and RBAC permissions to authenticate against the API server — exposing the API server on a local port could lead to unintended interactions if other local tools or scripts discover it. Always review your RBAC policies and consider using `--disabled-resources` alongside port forwarding to limit what the AI agent can discover and target.

When enabled, three additional tools become available:

#### Start Port Forward

Establishes a port-forwarding session to a Kubernetes pod. Supports forwarding multiple ports in a single session. Each port mapping forwards a local port to a port on the pod. Set `local_port` to `0` (or omit it) to let the system auto-assign a free local port.

**Arguments:**
- `namespace` (required): Pod namespace
- `pod` (required): Pod name
- `ports` (required): Array of port mappings, each with:
  - `pod_port` (required): Port on the pod to forward to (1-65535)
  - `local_port` (optional): Local port to listen on (0 or omit for auto-assign)
- `context` (optional): Kubernetes context to use (defaults to current context from kubeconfig)

**Example (single port, auto-assign):**
```json
{
  "namespace": "default",
  "pod": "my-app-pod-abc123",
  "ports": [
    { "pod_port": 8080 }
  ]
}
```

**Example (multiple ports, explicit local ports):**
```json
{
  "namespace": "default",
  "pod": "my-app-pod-abc123",
  "ports": [
    { "pod_port": 8080, "local_port": 18080 },
    { "pod_port": 5432, "local_port": 15432 }
  ]
}
```

**Example Response:**
```json
{
  "id": "pf-1",
  "namespace": "default",
  "pod": "my-app-pod-abc123",
  "ports": [
    { "pod_port": 8080, "local_port": 18080 },
    { "pod_port": 5432, "local_port": 15432 }
  ],
  "started_at": "2025-01-15T10:30:00Z"
}
```

#### Stop Port Forward

Terminates an active port-forwarding session by its ID.

**Arguments:**
- `id` (required): Port forward session ID (e.g., `"pf-1"`)

**Example:**
```json
{
  "id": "pf-1"
}
```

**Example Response:**
```json
{
  "id": "pf-1",
  "stopped": true
}
```

#### List Port Forwards

Lists all active port-forwarding sessions with their port mappings and metadata. Takes no arguments.

**Example:**
```json
{}
```

**Example Response:**
```json
{
  "count": 2,
  "port_forwards": [
    {
      "id": "pf-1",
      "namespace": "default",
      "pod": "my-app-pod-abc123",
      "ports": [
        { "pod_port": 8080, "local_port": 18080 }
      ],
      "started_at": "2025-01-15T10:30:00Z"
    },
    {
      "id": "pf-2",
      "namespace": "monitoring",
      "pod": "grafana-xyz789",
      "ports": [
        { "pod_port": 3000, "local_port": 13000 }
      ],
      "started_at": "2025-01-15T10:35:00Z"
    }
  ]
}
```

#### Port Forwarding Behavior

- **Automatic cleanup**: If the target pod is deleted or the connection drops, the port-forward session is automatically removed. Subsequent calls to `list_port_forwards` will no longer show the terminated session.
- **Graceful shutdown**: When the MCP server is stopped (via `SIGINT` or `SIGTERM`), all active port-forward sessions are terminated.
- **Session IDs**: Each session gets a unique, incrementing ID (e.g., `pf-1`, `pf-2`). Use this ID with `stop_port_forward` to terminate a specific session.
- **Multiple sessions**: You can have multiple port-forward sessions active simultaneously, each targeting different pods or ports.

#### Editor Configuration with Port Forwarding

```json5
{
  "mcpServers": {
    "kubernetes-ro": {
      "command": "mcp-kubernetes-ro",
      "args": [
        "--enable-port-forwarding"
      ],
      "env": {
        // Or use the environment variable instead of the flag:
        // "MCP_KUBERNETES_RO_ENABLE_PORT_FORWARDING": "true"
      }
    }
  }
}
```

## Examples

### Basic Usage Examples

```bash
# Start with default kubeconfig and context
mcp-kubernetes-ro

# Start with specific kubeconfig
mcp-kubernetes-ro --kubeconfig ~/.kube/config

# Start with KUBECONFIG environment variable
export KUBECONFIG=~/.kube/config
mcp-kubernetes-ro

# Start with specific namespace
mcp-kubernetes-ro --namespace kube-system

# Start in SSE mode
mcp-kubernetes-ro --transport=sse --port=3000

# Start with port forwarding enabled
mcp-kubernetes-ro --enable-port-forwarding

# Start with port forwarding via environment variable
export MCP_KUBERNETES_RO_ENABLE_PORT_FORWARDING=true
mcp-kubernetes-ro
```

### Advanced Configuration Examples

```bash
# Production cluster with specific kubeconfig
mcp-kubernetes-ro \
  --kubeconfig ~/.kube/prod-config \
  --namespace monitoring

# Development setup with SSE mode using environment variable
export KUBECONFIG=~/.kube/dev-config
mcp-kubernetes-ro \
  --transport=sse \
  --port=8080

# Using per-command context (specify context in tool calls)
# Context is now specified at the tool level, not globally

# Disable specific tools for security or performance reasons
mcp-kubernetes-ro --disabled-tools=get_logs,decode_base64

# Disable metrics tools when metrics server is not available
mcp-kubernetes-ro --disabled-tools=get_node_metrics,get_pod_metrics

# Prevent AI agents from reading Secrets and ConfigMaps
mcp-kubernetes-ro --disabled-resources=secrets --disabled-resources=configmaps

# Lock down a production environment: no logs, no secrets, no base64 decoding
mcp-kubernetes-ro \
  --kubeconfig ~/.kube/prod-config \
  --disabled-tools=get_logs,decode_base64 \
  --disabled-resources=secrets

# Use environment variables for disabled tools and resources
export MCP_KUBERNETES_RO_DISABLED_TOOLS=get_logs,decode_base64
export MCP_KUBERNETES_RO_DISABLED_RESOURCES=secrets
mcp-kubernetes-ro
```

## Use Cases

### Cluster Troubleshooting
- List failing pods across namespaces
- Get detailed resource configurations
- Retrieve pod logs for debugging
- Discover available API resources

### Resource Discovery
- Explore cluster resources by type
- Find resources with specific labels
- Understand resource relationships
- Identify resource configurations

### Security and Compliance
- Read-only access prevents accidental changes
- Inspect configurations without modification risk
- Audit resource states and settings
- Safe exploration of production clusters

### AI-Assisted Operations
- Let AI assistants help diagnose cluster issues
- Get intelligent suggestions for resource problems
- Automated log analysis and pattern recognition
- Natural language queries for Kubernetes resources

## AI Assistant Considerations

While this MCP server provides comprehensive tools for Kubernetes cluster inspection, some AI assistants may have limitations or policies that prevent them from using certain tool combinations, even when technically available:

### Potential Limitations

- **Secret Access**: Some AI assistants may refuse to retrieve, decode, or display Kubernetes secrets (even using the provided `get_resource` and `decode_base64` tools) due to security policies around credential handling
- **Sensitive Data**: AI models may have built-in restrictions against exposing sensitive information in chat interfaces, regardless of user permissions or tool availability
- **Security Patterns**: Certain AI assistants prioritize security best practices over technical capability, potentially refusing operations that could expose sensitive data

### Workarounds

If your AI assistant refuses to use available tools for security reasons:

1. **Direct CLI Access**: Use `kubectl` directly for sensitive operations, and just ask the AI to give you the command to run, for example:
   ```bash
   kubectl get secret <secret-name> -n <namespace> -o yaml
   echo "<base64-data>" | base64 -d
   ```

2. **Manual Tool Usage**: If using the MCP server programmatically, call the tools directly rather than through an AI assistant

3. **Documentation**: Consider the security implications - the AI's refusal might actually be protecting you from inadvertent credential exposure

### Design Philosophy

This behavior reflects different approaches to security:
- **Tool-based**: If you have the tools and permissions, you should be able to use them
- **AI-safety**: Prioritize preventing accidental exposure over technical capability

Both perspectives are valid, and this limitation should be considered when designing workflows that involve sensitive data retrieval.

## Startup Connectivity Check

The MCP server performs an automatic connectivity check on startup to verify that it can successfully connect to your Kubernetes cluster. This check includes:

1. **API Server Reachability**: Verifies that the Kubernetes API server is accessible and responding
2. **Authentication**: Confirms that your credentials are valid and accepted by the cluster
3. **API Discovery**: Tests that the server can discover available API resources
4. **Basic Permissions**: Validates that you have at least read access to namespaces (basic RBAC check)

### What You'll See

On successful startup, you'll see output like:
```
Testing connectivity to Kubernetes cluster...
✓ Successfully connected to Kubernetes cluster (version: v1.28.0)
```

### Troubleshooting Connectivity Issues

If the connectivity check fails, you'll see a detailed error message. Common issues include:

- **Invalid kubeconfig**: Check that your kubeconfig file exists and is properly formatted
- **Cluster unreachable**: Verify that the cluster endpoint is accessible from your network
- **Authentication failed**: Ensure your credentials haven't expired and are valid
- **Insufficient permissions**: Verify you have at least read access to basic cluster resources

The connectivity check has a 30-second timeout to prevent hanging on unresponsive clusters.

## Security Considerations

- **Read-Only Access**: The server only supports read operations (`get`, `list`, `watch`)
- **Resource Access Control**: Block AI agents from querying specific resource types (e.g., Secrets) using `--disabled-resources`
- **Local Authentication**: Uses your existing kubectl configuration and credentials
- **No Destructive Operations**: Cannot create, update, or delete resources
- **Namespace Isolation**: Respects RBAC permissions from your kubeconfig
- **Secure Communication**: Supports both stdio and HTTPS-based SSE communication

## Metrics Implementation Details

### Error Detection and Handling

The metrics tools (`get_node_metrics` and `get_pod_metrics`) include sophisticated error detection for metrics server availability:

- **Automatic Detection**: Detects when the metrics server is not installed or not responding
- **Helpful Error Messages**: Provides specific installation commands when metrics server is missing
- **Common Error Patterns**: Recognizes various metrics server error scenarios:
  - `metrics-server` service not found
  - `metrics.k8s.io` API group unavailable
  - "server could not find the requested resource" errors
  - "no metrics available" conditions

### Pagination Implementation

Both metrics tools implement client-side pagination for consistent results, since the built-in metrics server endpoint does not support needle-based pagination, and also to provide a safe way for AI tools to request just the data they need, especially useful in small context windows.

- **Sorting**: All results are sorted by timestamp (newest first) before pagination
- **Continue Tokens**: Base64-encoded JSON tokens containing:
  - `offset`: Current position in the result set
  - `type`: Resource type ("node" or "pod")
  - `namespace`: Context namespace (for pod metrics)
- **Context Awareness**: Pagination state resets when namespace context changes
- **Token Format**: `eyJvZmZzZXQiOjEwLCJ0eXBlIjoicG9kIiwibmFtZXNwYWNlIjoia3ViZS1zeXN0ZW0ifQ==`

### Resource Retrieval Strategy

- **Fetch-Then-Filter**: Always retrieves all available metrics from the server, then applies client-side filtering and pagination
- **Consistent Ordering**: Ensures predictable results across paginated requests
- **Namespace Scoping**: Automatically scopes pod metrics to specific namespaces when provided

## Error Handling

The server provides detailed error messages for common issues:

- Invalid resource types or API versions
- Missing required parameters
- RBAC permission errors
- Network connectivity issues
- Malformed kubeconfig files
- Metrics server unavailability (with installation guidance)

## Limitations

- Requires local kubectl configuration
- Read-only access only (no write operations)
- Limited to resources accessible by your kubeconfig credentials
- No real-time streaming of logs (static retrieval only)
- No support for custom resource definitions discovery beyond API resources
