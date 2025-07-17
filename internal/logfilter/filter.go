// Package logfilter provides functionality for filtering Kubernetes pod logs
// with grep-like capabilities including pattern matching, regular expressions,
// and time-based filtering. It supports both inclusion and exclusion patterns
// for flexible log analysis.
package logfilter

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FilterOptions represents the configuration for filtering log lines.
// It supports both literal string matching and regular expression patterns,
// with separate inclusion and exclusion filters that can be combined.
type FilterOptions struct {
	// GrepInclude contains patterns that lines must match to be included.
	// Works like grep - only lines containing any of these patterns are kept.
	// If empty, all lines are considered for inclusion (subject to exclusion).
	GrepInclude []string

	// GrepExclude contains patterns that exclude lines from the output.
	// Works like grep -v - lines containing any of these patterns are removed.
	// Applied after inclusion filtering.
	GrepExclude []string

	// UseRegex determines whether to treat patterns as regular expressions.
	// If false, patterns are treated as literal strings for simple substring matching.
	// If true, patterns are compiled as regular expressions for advanced matching.
	UseRegex bool
}

// FilterLogs applies the specified filtering options to log content and returns filtered lines.
// It processes the content line by line, applying inclusion and exclusion patterns
// in sequence. Empty lines at the end are automatically removed.
//
// The filtering process:
//  1. Split content into lines
//  2. For each line, check inclusion patterns (if any)
//  3. For each remaining line, check exclusion patterns (if any)
//  4. Return the filtered content as a joined string
//
// Returns an error if regular expression patterns are invalid when UseRegex is true.
func FilterLogs(content string, opts *FilterOptions) (string, error) {
	if opts == nil {
		return content, nil
	}

	lines := strings.Split(content, "\n")
	filteredLines := make([]string, 0, len(lines))

	// Compile patterns if using regex
	var includePatterns []*regexp.Regexp
	var excludePatterns []*regexp.Regexp

	if opts.UseRegex {
		// Compile include patterns
		for _, pattern := range opts.GrepInclude {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return "", fmt.Errorf("invalid include regex pattern %q: %w", pattern, err)
			}
			includePatterns = append(includePatterns, re)
		}

		// Compile exclude patterns
		for _, pattern := range opts.GrepExclude {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return "", fmt.Errorf("invalid exclude regex pattern %q: %w", pattern, err)
			}
			excludePatterns = append(excludePatterns, re)
		}
	}

	// Process each line
	for _, line := range lines {
		// Skip empty lines at the end
		if line == "" && len(filteredLines) > 0 {
			continue
		}

		// Check include patterns
		if len(opts.GrepInclude) > 0 {
			matched := false
			if opts.UseRegex {
				for _, pattern := range includePatterns {
					if pattern.MatchString(line) {
						matched = true
						break
					}
				}
			} else {
				for _, pattern := range opts.GrepInclude {
					if strings.Contains(line, pattern) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}

		// Check exclude patterns
		if len(opts.GrepExclude) > 0 {
			excluded := false
			if opts.UseRegex {
				for _, pattern := range excludePatterns {
					if pattern.MatchString(line) {
						excluded = true
						break
					}
				}
			} else {
				for _, pattern := range opts.GrepExclude {
					if strings.Contains(line, pattern) {
						excluded = true
						break
					}
				}
			}
			if excluded {
				continue
			}
		}

		filteredLines = append(filteredLines, line)
	}

	return strings.Join(filteredLines, "\n"), nil
}

// CountMatchingLines counts the number of lines that match the filter criteria
// without returning the actual filtered content. This is useful for getting
// statistics about log filtering results.
//
// Returns 0 if no lines match or if the content is empty after filtering.
// Returns an error if the filter options contain invalid regular expressions.
func CountMatchingLines(content string, opts *FilterOptions) (int, error) {
	filtered, err := FilterLogs(content, opts)
	if err != nil {
		return 0, err
	}

	if filtered == "" {
		return 0, nil
	}

	return len(strings.Split(filtered, "\n")), nil
}

// ParseSinceTime parses a "since" time string into either an absolute time or relative duration.
// It supports multiple time formats for flexible log retrieval:
//
// Duration formats (relative to now):
//   - "5m" (5 minutes ago)
//   - "1h" (1 hour ago)
//   - "2h30m" (2 hours 30 minutes ago)
//   - "1d" (1 day ago)
//
// Absolute time formats:
//   - "2023-01-01T10:00:00Z" (RFC3339)
//   - "2023-01-01T10:00:00" (without timezone)
//   - "2023-01-01 10:00:00" (space separator)
//   - "2023-01-01" (date only)
//
// Returns either a time.Time pointer for absolute times or an int64 pointer
// for relative durations in seconds. Only one return value will be non-nil.
func ParseSinceTime(since string) (*time.Time, *int64, error) {
	if since == "" {
		return nil, nil, nil
	}

	// Try to parse as duration first (e.g., "5m", "1h", "2h30m", "1d")
	if duration, err := parseDuration(since); err == nil {
		// Convert duration to seconds
		seconds := int64(duration.Seconds())
		return nil, &seconds, nil
	}

	// Try to parse as absolute time
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, since); err == nil {
			return &t, nil, nil
		}
	}

	return nil, nil, fmt.Errorf("invalid since time format: %s", since)
}

// parseDuration extends the standard time.ParseDuration to support day notation.
// It handles formats like "1d", "2d" by converting them to hour-based durations.
// Falls back to standard duration parsing for other formats.
func parseDuration(s string) (time.Duration, error) {
	// Handle days notation (e.g., "1d", "2d")
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			return days * 24, nil
		}
	}

	// Standard duration parsing
	return time.ParseDuration(s)
}

// ValidateFilterOptions validates the filter options for correctness.
// It checks that regular expression patterns are valid when UseRegex is true,
// preventing runtime errors during log filtering.
//
// Returns an error describing which pattern is invalid and why.
// Returns nil if all options are valid or if opts is nil.
func ValidateFilterOptions(opts *FilterOptions) error {
	if opts == nil {
		return nil
	}

	// Test regex patterns if regex mode is enabled
	if opts.UseRegex {
		for _, pattern := range opts.GrepInclude {
			if _, err := regexp.Compile(pattern); err != nil {
				return fmt.Errorf("invalid include regex pattern %q: %w", pattern, err)
			}
		}
		for _, pattern := range opts.GrepExclude {
			if _, err := regexp.Compile(pattern); err != nil {
				return fmt.Errorf("invalid exclude regex pattern %q: %w", pattern, err)
			}
		}
	}

	return nil
}
