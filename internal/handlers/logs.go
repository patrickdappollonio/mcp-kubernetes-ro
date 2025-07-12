package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/logfilter"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/response"
)

type LogHandler struct {
	client     *kubernetes.Client
	baseConfig *kubernetes.Config
}

func NewLogHandler(client *kubernetes.Client, baseConfig *kubernetes.Config) *LogHandler {
	return &LogHandler{
		client:     client,
		baseConfig: baseConfig,
	}
}

func (h *LogHandler) GetLogs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params struct {
		Namespace   string `json:"namespace"`
		Name        string `json:"name"`
		Container   string `json:"container"`
		Context     string `json:"context"`
		MaxLines    string `json:"max_lines"`
		GrepInclude string `json:"grep_include"`
		GrepExclude string `json:"grep_exclude"`
		UseRegex    bool   `json:"use_regex"`
		Since       string `json:"since"`
		Previous    bool   `json:"previous"`
	}

	if err := request.BindArguments(&params); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.Name == "" {
		return nil, fmt.Errorf("pod name is required")
	}

	// Use the appropriate client based on context
	client := h.client
	if params.Context != "" {
		contextClient, err := kubernetes.NewClientWithContext(h.baseConfig, params.Context)
		if err != nil {
			return nil, fmt.Errorf("failed to create client with context %s: %w", params.Context, err)
		}
		client = contextClient
	}

	// Parse max lines
	var maxLines *int64
	if params.MaxLines != "" {
		lines, err := strconv.ParseInt(params.MaxLines, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid max_lines value: %w", err)
		}
		maxLines = &lines
	}

	// Parse since time
	sinceTime, sinceSeconds, err := logfilter.ParseSinceTime(params.Since)
	if err != nil {
		return nil, fmt.Errorf("invalid since time: %w", err)
	}

	// Parse comma-separated grep patterns
	var grepInclude []string
	if params.GrepInclude != "" {
		grepInclude = strings.Split(params.GrepInclude, ",")
		for i, pattern := range grepInclude {
			grepInclude[i] = strings.TrimSpace(pattern)
		}
	}

	var grepExclude []string
	if params.GrepExclude != "" {
		grepExclude = strings.Split(params.GrepExclude, ",")
		for i, pattern := range grepExclude {
			grepExclude[i] = strings.TrimSpace(pattern)
		}
	}

	// Validate filter options
	filterOpts := &logfilter.FilterOptions{
		GrepInclude: grepInclude,
		GrepExclude: grepExclude,
		UseRegex:    params.UseRegex,
	}
	if err := logfilter.ValidateFilterOptions(filterOpts); err != nil {
		return nil, fmt.Errorf("invalid filter options: %w", err)
	}

	// Build log options
	logOpts := &kubernetes.LogOptions{
		Container:    params.Container,
		MaxLines:     maxLines,
		SinceTime:    sinceTime,
		SinceSeconds: sinceSeconds,
		Previous:     params.Previous,
	}

	// Get logs
	logs, err := client.GetPodLogsWithOptions(ctx, params.Namespace, params.Name, logOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod logs: %w", err)
	}

	// Apply filtering
	filteredLogs, err := logfilter.FilterLogs(logs, filterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to filter logs: %w", err)
	}

	// Count matching lines for metadata
	matchingLines, err := logfilter.CountMatchingLines(logs, filterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to count matching lines: %w", err)
	}

	responseData := map[string]interface{}{
		"namespace": params.Namespace,
		"pod":       params.Name,
		"container": params.Container,
		"logs":      filteredLogs,
		"metadata": map[string]interface{}{
			"total_lines":    len(strings.Split(logs, "\n")),
			"matching_lines": matchingLines,
			"filtered":       len(grepInclude) > 0 || len(grepExclude) > 0,
			"since":          params.Since,
			"previous":       params.Previous,
			"use_regex":      params.UseRegex,
			"grep_include":   grepInclude,
			"grep_exclude":   grepExclude,
		},
	}

	return response.JSON(responseData)
}

func (h *LogHandler) GetPodContainers(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Context   string `json:"context"`
	}

	if err := request.BindArguments(&params); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.Name == "" {
		return nil, fmt.Errorf("pod name is required")
	}

	// Use the appropriate client based on context
	client := h.client
	if params.Context != "" {
		contextClient, err := kubernetes.NewClientWithContext(h.baseConfig, params.Context)
		if err != nil {
			return nil, fmt.Errorf("failed to create client with context %s: %w", params.Context, err)
		}
		client = contextClient
	}

	containers, err := client.GetPodContainers(ctx, params.Namespace, params.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod containers: %w", err)
	}

	responseData := map[string]interface{}{
		"namespace":  params.Namespace,
		"pod":        params.Name,
		"containers": containers,
	}

	return response.JSON(responseData)
}
