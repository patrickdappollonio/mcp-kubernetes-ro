package toolfilter

import (
	"os"
	"strings"
)

// Filter handles checking if tools should be disabled based on configuration.
type Filter struct {
	disabledTools []string
}

// NewFilter creates a new Filter from a disabled tools value.
// It first checks the provided value, then falls back to the DISABLED_TOOLS environment variable.
func NewFilter(disabledToolsValue string) *Filter {
	// Check environment variable if value not provided
	if disabledToolsValue == "" {
		disabledToolsValue = os.Getenv("DISABLED_TOOLS")
	}

	return &Filter{
		disabledTools: parseDisabledTools(disabledToolsValue),
	}
}

// NewFilterFromList creates a new Filter from a pre-parsed list of disabled tools.
func NewFilterFromList(disabledTools []string) *Filter {
	// Create a copy of the input slice to avoid sharing references
	copied := make([]string, len(disabledTools))
	copy(copied, disabledTools)

	return &Filter{
		disabledTools: copied,
	}
}

// IsDisabled checks if a tool name should be disabled.
// The comparison is case-insensitive.
func (f *Filter) IsDisabled(toolName string) bool {
	for _, disabled := range f.disabledTools {
		if strings.EqualFold(toolName, disabled) {
			return true
		}
	}
	return false
}

// GetDisabledTools returns a copy of the disabled tools list.
func (f *Filter) GetDisabledTools() []string {
	result := make([]string, len(f.disabledTools))
	copy(result, f.disabledTools)
	return result
}

// parseDisabledTools parses a comma/space-separated string of disabled tool names.
func parseDisabledTools(value string) []string {
	if value == "" {
		return nil
	}

	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}
