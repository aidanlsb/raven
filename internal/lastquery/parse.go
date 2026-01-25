package lastquery

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseNumbers parses a string of result numbers into a slice of ints.
// Supports multiple formats:
//   - Single number: "1"
//   - Comma-separated: "1,3,5"
//   - Range: "1-5"
//   - Mixed: "1,3-5,7"
//   - Space-separated (multiple args): handled by caller joining with commas
//
// Returns 1-indexed numbers as provided by user.
func ParseNumbers(input string) ([]int, error) {
	if input == "" {
		return nil, fmt.Errorf("%w: empty input", ErrInvalidNumber)
	}

	// Normalize: replace spaces with commas for consistent parsing
	input = strings.ReplaceAll(input, " ", ",")
	
	var result []int
	seen := make(map[int]bool)
	
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		
		// Check if it's a range (contains "-")
		if strings.Contains(part, "-") {
			nums, err := parseRange(part)
			if err != nil {
				return nil, err
			}
			for _, n := range nums {
				if !seen[n] {
					seen[n] = true
					result = append(result, n)
				}
			}
		} else {
			// Single number
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("%w: %q is not a valid number", ErrInvalidNumber, part)
			}
			if n < 1 {
				return nil, fmt.Errorf("%w: %d must be positive", ErrInvalidNumber, n)
			}
			if !seen[n] {
				seen[n] = true
				result = append(result, n)
			}
		}
	}
	
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: no valid numbers found", ErrInvalidNumber)
	}
	
	return result, nil
}

// parseRange parses a range like "1-5" into [1, 2, 3, 4, 5].
func parseRange(s string) ([]int, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: invalid range %q", ErrInvalidNumber, s)
	}
	
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid range start %q", ErrInvalidNumber, parts[0])
	}
	
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid range end %q", ErrInvalidNumber, parts[1])
	}
	
	if start < 1 {
		return nil, fmt.Errorf("%w: range start %d must be positive", ErrInvalidNumber, start)
	}
	
	if end < start {
		return nil, fmt.Errorf("%w: range end %d must be >= start %d", ErrInvalidNumber, end, start)
	}
	
	// Limit range size to prevent accidental huge ranges
	const maxRangeSize = 1000
	if end-start+1 > maxRangeSize {
		return nil, fmt.Errorf("%w: range %d-%d is too large (max %d)", ErrInvalidNumber, start, end, maxRangeSize)
	}
	
	result := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		result = append(result, i)
	}
	
	return result, nil
}

// ParseNumberArgs parses multiple string arguments as numbers.
// Joins args with commas and delegates to ParseNumbers.
// This allows: "1" "3" "5" to be parsed as [1, 3, 5]
func ParseNumberArgs(args []string) ([]int, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%w: no numbers provided", ErrInvalidNumber)
	}
	return ParseNumbers(strings.Join(args, ","))
}
