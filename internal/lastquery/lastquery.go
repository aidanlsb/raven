// Package lastquery provides persistence and retrieval of the most recent query results.
// This enables numbered references to query results for selective operations.
package lastquery

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LastQuery stores the results of the most recent query.
// Persisted to .raven/last-query.json for use in follow-up commands.
type LastQuery struct {
	Query     string        `json:"query"`
	Timestamp time.Time     `json:"timestamp"`
	Type      string        `json:"type"` // "trait" or "object"
	Results   []ResultEntry `json:"results"`
}

// ResultEntry represents a single result from the query.
type ResultEntry struct {
	Num      int    `json:"num"`      // 1-indexed number for user reference
	ID       string `json:"id"`       // Unique identifier (trait ID or object ID)
	Type     string `json:"type"`     // "trait" or "object"
	Content  string `json:"content"`  // Human-readable description
	Location string `json:"location"` // Short location (e.g., "daily/2026-01-25:42")
}

// Errors
var (
	ErrNoLastQuery    = errors.New("no last query available")
	ErrInvalidNumber  = errors.New("invalid result number")
	ErrNumberOutOfRange = errors.New("result number out of range")
)

// Path returns the path to the last-query.json file.
func Path(vaultPath string) string {
	return filepath.Join(vaultPath, ".raven", "last-query.json")
}

// Write saves the last query results to disk.
func Write(vaultPath string, lq *LastQuery) error {
	// Ensure .raven directory exists
	ravenDir := filepath.Join(vaultPath, ".raven")
	if err := os.MkdirAll(ravenDir, 0755); err != nil {
		return fmt.Errorf("failed to create .raven directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(lq, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal last query: %w", err)
	}

	// Write to file
	path := Path(vaultPath)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write last query: %w", err)
	}

	return nil
}

// Read loads the last query results from disk.
// Returns ErrNoLastQuery if no last query file exists.
func Read(vaultPath string) (*LastQuery, error) {
	path := Path(vaultPath)
	
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoLastQuery
		}
		return nil, fmt.Errorf("failed to read last query: %w", err)
	}

	var lq LastQuery
	if err := json.Unmarshal(data, &lq); err != nil {
		return nil, fmt.Errorf("failed to parse last query: %w", err)
	}

	return &lq, nil
}

// GetByNumbers returns the entries matching the given numbers.
// Numbers are 1-indexed (as displayed to users).
// Returns an error if any number is out of range.
func (lq *LastQuery) GetByNumbers(nums []int) ([]ResultEntry, error) {
	results := make([]ResultEntry, 0, len(nums))
	
	for _, num := range nums {
		if num < 1 || num > len(lq.Results) {
			return nil, fmt.Errorf("%w: %d (valid range: 1-%d)", ErrNumberOutOfRange, num, len(lq.Results))
		}
		// Convert 1-indexed to 0-indexed
		results = append(results, lq.Results[num-1])
	}
	
	return results, nil
}

// GetAllIDs returns the IDs of all results.
func (lq *LastQuery) GetAllIDs() []string {
	ids := make([]string, len(lq.Results))
	for i, r := range lq.Results {
		ids[i] = r.ID
	}
	return ids
}
