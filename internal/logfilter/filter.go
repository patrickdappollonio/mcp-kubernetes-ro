package logfilter

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FilterOptions represents options for filtering log lines
type FilterOptions struct {
	GrepInclude []string // Include patterns (like grep)
	GrepExclude []string // Exclude patterns (like grep -v)
	UseRegex    bool     // Whether to treat patterns as regular expressions
}

// FilterLogs applies filtering options to log content and returns filtered lines
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

// ParseSinceTime parses a "since" time string into a time.Time
// Supports formats like: "5m", "1h", "2h30m", "1d", "2023-01-01T10:00:00Z"
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

// parseDuration parses extended duration strings including days
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

// ValidateFilterOptions validates the filter options
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
