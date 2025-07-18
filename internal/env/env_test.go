package env

import (
	"fmt"
	"os"
	"testing"
)

func TestFirstDefault(t *testing.T) {
	tests := []struct {
		name         string
		defaultValue string
		keys         []string
		envVars      map[string]string
		expected     string
	}{
		{
			name:         "no keys provided returns default",
			defaultValue: "default_value",
			keys:         []string{},
			envVars:      map[string]string{},
			expected:     "default_value",
		},
		{
			name:         "first key set returns first value",
			defaultValue: "default_value",
			keys:         []string{"KEY1", "KEY2", "KEY3"},
			envVars:      map[string]string{"KEY1": "value1", "KEY2": "value2", "KEY3": "value3"},
			expected:     "value1",
		},
		{
			name:         "first key empty second key set returns second value",
			defaultValue: "default_value",
			keys:         []string{"KEY1", "KEY2", "KEY3"},
			envVars:      map[string]string{"KEY1": "", "KEY2": "value2", "KEY3": "value3"},
			expected:     "value2",
		},
		{
			name:         "first two keys empty third key set returns third value",
			defaultValue: "default_value",
			keys:         []string{"KEY1", "KEY2", "KEY3"},
			envVars:      map[string]string{"KEY1": "", "KEY2": "", "KEY3": "value3"},
			expected:     "value3",
		},
		{
			name:         "all keys empty returns default",
			defaultValue: "default_value",
			keys:         []string{"KEY1", "KEY2", "KEY3"},
			envVars:      map[string]string{"KEY1": "", "KEY2": "", "KEY3": ""},
			expected:     "default_value",
		},
		{
			name:         "all keys unset returns default",
			defaultValue: "default_value",
			keys:         []string{"KEY1", "KEY2", "KEY3"},
			envVars:      map[string]string{},
			expected:     "default_value",
		},
		{
			name:         "key with leading whitespace is trimmed",
			defaultValue: "default_value",
			keys:         []string{"KEY1"},
			envVars:      map[string]string{"KEY1": "  value1"},
			expected:     "value1",
		},
		{
			name:         "key with trailing whitespace is trimmed",
			defaultValue: "default_value",
			keys:         []string{"KEY1"},
			envVars:      map[string]string{"KEY1": "value1  "},
			expected:     "value1",
		},
		{
			name:         "key with leading and trailing whitespace is trimmed",
			defaultValue: "default_value",
			keys:         []string{"KEY1"},
			envVars:      map[string]string{"KEY1": "  value1  "},
			expected:     "value1",
		},
		{
			name:         "key with only whitespace is considered empty",
			defaultValue: "default_value",
			keys:         []string{"KEY1", "KEY2"},
			envVars:      map[string]string{"KEY1": "   ", "KEY2": "value2"},
			expected:     "value2",
		},
		{
			name:         "key with only tabs and spaces is considered empty",
			defaultValue: "default_value",
			keys:         []string{"KEY1", "KEY2"},
			envVars:      map[string]string{"KEY1": "\t  \t", "KEY2": "value2"},
			expected:     "value2",
		},
		{
			name:         "key with only newlines is considered empty",
			defaultValue: "default_value",
			keys:         []string{"KEY1", "KEY2"},
			envVars:      map[string]string{"KEY1": "\n\n", "KEY2": "value2"},
			expected:     "value2",
		},
		{
			name:         "empty default value with no keys set",
			defaultValue: "",
			keys:         []string{"KEY1", "KEY2"},
			envVars:      map[string]string{},
			expected:     "",
		},
		{
			name:         "empty default value with empty keys",
			defaultValue: "",
			keys:         []string{"KEY1", "KEY2"},
			envVars:      map[string]string{"KEY1": "", "KEY2": ""},
			expected:     "",
		},
		{
			name:         "single key set returns value",
			defaultValue: "default_value",
			keys:         []string{"SINGLE_KEY"},
			envVars:      map[string]string{"SINGLE_KEY": "single_value"},
			expected:     "single_value",
		},
		{
			name:         "single key unset returns default",
			defaultValue: "default_value",
			keys:         []string{"SINGLE_KEY"},
			envVars:      map[string]string{},
			expected:     "default_value",
		},
		{
			name:         "single key empty returns default",
			defaultValue: "default_value",
			keys:         []string{"SINGLE_KEY"},
			envVars:      map[string]string{"SINGLE_KEY": ""},
			expected:     "default_value",
		},
		{
			name:         "mixed unset and empty keys returns default",
			defaultValue: "default_value",
			keys:         []string{"UNSET_KEY", "EMPTY_KEY"},
			envVars:      map[string]string{"EMPTY_KEY": ""},
			expected:     "default_value",
		},
		{
			name:         "value with internal whitespace is preserved",
			defaultValue: "default_value",
			keys:         []string{"KEY1"},
			envVars:      map[string]string{"KEY1": "  value with spaces  "},
			expected:     "value with spaces",
		},
		{
			name:         "value with tabs and newlines is trimmed but internal preserved",
			defaultValue: "default_value",
			keys:         []string{"KEY1"},
			envVars:      map[string]string{"KEY1": "\t\nvalue\twith\ttabs\n\t"},
			expected:     "value\twith\ttabs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			originalEnv := make(map[string]string)
			for key := range tt.envVars {
				originalEnv[key] = os.Getenv(key)
			}

			// Clean up any existing environment variables that might interfere
			for key := range tt.envVars {
				os.Unsetenv(key)
			}

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Act
			result := FirstDefault(tt.defaultValue, tt.keys...)

			// Assert
			if result != tt.expected {
				t.Errorf("FirstDefault(%q, %v) = %q, want %q", tt.defaultValue, tt.keys, result, tt.expected)
			}

			// Restore original environment
			for key := range tt.envVars {
				os.Unsetenv(key)
				if originalValue, existed := originalEnv[key]; existed {
					os.Setenv(key, originalValue)
				}
			}
		})
	}
}

func TestFirstDefault_EmptyVariadicArguments(t *testing.T) {
	// Test with no variadic arguments
	result := FirstDefault("default")
	expected := "default"

	if result != expected {
		t.Errorf("FirstDefault(\"default\") = %q, want %q", result, expected)
	}
}

func TestFirstDefault_NilCheck(t *testing.T) {
	// Test that the function handles nil slice correctly
	// This is mostly for completeness, as Go would convert nil to empty slice
	result := FirstDefault("default", []string{}...)
	expected := "default"

	if result != expected {
		t.Errorf("FirstDefault(\"default\", []string{}...) = %q, want %q", result, expected)
	}
}

func TestFirstDefault_LargeNumberOfKeys(t *testing.T) {
	// Test with a large number of keys to ensure performance is reasonable
	keys := make([]string, 100)
	envVars := make(map[string]string)

	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("TEST_KEY_%d", i)
		keys[i] = key
		if i == 50 {
			envVars[key] = "found_value"
		} else {
			envVars[key] = ""
		}
	}

	// Save and clean environment
	originalEnv := make(map[string]string)
	for key := range envVars {
		originalEnv[key] = os.Getenv(key)
		os.Unsetenv(key)
	}

	// Set test environment
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	result := FirstDefault("default", keys...)
	expected := "found_value"

	if result != expected {
		t.Errorf("FirstDefault with large number of keys = %q, want %q", result, expected)
	}

	// Restore environment
	for key := range envVars {
		os.Unsetenv(key)
		if originalValue, existed := originalEnv[key]; existed {
			os.Setenv(key, originalValue)
		}
	}
}
