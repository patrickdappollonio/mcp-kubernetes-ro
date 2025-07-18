package toolfilter

import (
	"os"
	"testing"
)

func TestParseDisabledTools(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single tool",
			input:    "get_resource",
			expected: []string{"get_resource"},
		},
		{
			name:     "comma separated",
			input:    "get_resource,list_resources",
			expected: []string{"get_resource", "list_resources"},
		},
		{
			name:     "comma with spaces",
			input:    "get_resource, list_resources",
			expected: []string{"get_resource", "list_resources"},
		},
		{
			name:     "space separated",
			input:    "get_resource list_resources",
			expected: []string{"get_resource", "list_resources"},
		},
		{
			name:     "mixed separators",
			input:    "get_resource,list_resources get_logs",
			expected: []string{"get_resource", "list_resources", "get_logs"},
		},
		{
			name:     "with tabs and newlines",
			input:    "get_resource\tlist_resources\nget_logs",
			expected: []string{"get_resource", "list_resources", "get_logs"},
		},
		{
			name:     "extra whitespace",
			input:    "  get_resource  ,  list_resources  ",
			expected: []string{"get_resource", "list_resources"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDisabledTools(tt.input)
			if !slicesEqual(result, tt.expected) {
				t.Errorf("parseDisabledTools(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envValue string
		expected []string
	}{
		{
			name:     "from input value",
			input:    "get_resource,list_resources",
			envValue: "get_logs",
			expected: []string{"get_resource", "list_resources"},
		},
		{
			name:     "from environment variable",
			input:    "",
			envValue: "get_logs,get_metrics",
			expected: []string{"get_logs", "get_metrics"},
		},
		{
			name:     "empty input and env",
			input:    "",
			envValue: "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			oldEnv := os.Getenv("DISABLED_TOOLS")
			os.Setenv("DISABLED_TOOLS", tt.envValue)
			defer os.Setenv("DISABLED_TOOLS", oldEnv)

			filter := NewFilter(tt.input)
			result := filter.GetDisabledTools()
			if !slicesEqual(result, tt.expected) {
				t.Errorf("NewFilter(%q) with env %q = %v, want %v", tt.input, tt.envValue, result, tt.expected)
			}
		})
	}
}

func TestNewFilterFromList(t *testing.T) {
	input := []string{"get_resource", "list_resources"}
	filter := NewFilterFromList(input)
	result := filter.GetDisabledTools()

	if !slicesEqual(result, input) {
		t.Errorf("NewFilterFromList(%v) = %v, want %v", input, result, input)
	}

	// Verify it's a copy, not the same slice
	input[0] = "modified"
	result2 := filter.GetDisabledTools()
	if result2[0] == "modified" {
		t.Error("NewFilterFromList should create a copy, not share the slice")
	}
}

func TestFilterIsDisabled(t *testing.T) {
	filter := NewFilterFromList([]string{"get_resource", "LIST_RESOURCES", "Get_Logs"})

	tests := []struct {
		name     string
		toolName string
		expected bool
	}{
		{
			name:     "exact match",
			toolName: "get_resource",
			expected: true,
		},
		{
			name:     "case insensitive match uppercase",
			toolName: "GET_RESOURCE",
			expected: true,
		},
		{
			name:     "case insensitive match mixed case",
			toolName: "Get_Resource",
			expected: true,
		},
		{
			name:     "case insensitive match lowercase",
			toolName: "list_resources",
			expected: true,
		},
		{
			name:     "case insensitive match different case",
			toolName: "get_LOGS",
			expected: true,
		},
		{
			name:     "not disabled",
			toolName: "get_metrics",
			expected: false,
		},
		{
			name:     "partial match should not disable",
			toolName: "get_resource_info",
			expected: false,
		},
		{
			name:     "empty tool name",
			toolName: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsDisabled(tt.toolName)
			if result != tt.expected {
				t.Errorf("IsDisabled(%q) = %v, want %v", tt.toolName, result, tt.expected)
			}
		})
	}
}

func TestFilterGetDisabledTools(t *testing.T) {
	original := []string{"get_resource", "list_resources"}
	filter := NewFilterFromList(original)

	result := filter.GetDisabledTools()

	// Should return a copy
	if !slicesEqual(result, original) {
		t.Errorf("GetDisabledTools() = %v, want %v", result, original)
	}

	// Modifying the returned slice should not affect the filter
	result[0] = "modified"
	result2 := filter.GetDisabledTools()
	if result2[0] == "modified" {
		t.Error("GetDisabledTools should return a copy, not the original slice")
	}
}

// Helper function to compare string slices
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
