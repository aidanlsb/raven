// Package lastresults provides persistence and retrieval of the most recent
// retrieval results (query, search, backlinks) for follow-up commands.
package lastresults

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aidanlsb/raven/internal/lastquery"
	"github.com/aidanlsb/raven/internal/model"
)

// Source identifies the command that produced the results.
type Source string

const (
	SourceQuery     Source = "query"
	SourceSearch    Source = "search"
	SourceBacklinks Source = "backlinks"
	SourceOutlinks  Source = "outlinks"
)

// LastResults stores the results of the most recent retrieval command.
// Persisted to .raven/last-results.json for use in follow-up commands.
type LastResults struct {
	Source    Source         `json:"source"`
	Query     string         `json:"query,omitempty"`
	Target    string         `json:"target,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Results   []StoredResult `json:"results"`
}

// StoredResult wraps a result with its kind for decoding.
type StoredResult struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// Errors
var (
	ErrNoLastResults    = errors.New("no last results available")
	ErrNumberOutOfRange = errors.New("result number out of range")
)

// Path returns the path to the last-results.json file.
func Path(vaultPath string) string {
	return filepath.Join(vaultPath, ".raven", "last-results.json")
}

// Write saves the last results to disk.
func Write(vaultPath string, lr *LastResults) error {
	// Ensure .raven directory exists
	ravenDir := filepath.Join(vaultPath, ".raven")
	if err := os.MkdirAll(ravenDir, 0755); err != nil {
		return fmt.Errorf("failed to create .raven directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(lr, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal last results: %w", err)
	}

	// Write to file
	path := Path(vaultPath)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write last results: %w", err)
	}

	return nil
}

// Read loads the last results from disk.
// Falls back to legacy last-query.json if needed.
func Read(vaultPath string) (*LastResults, error) {
	path := Path(vaultPath)

	data, err := os.ReadFile(path)
	if err == nil {
		var lr LastResults
		if err := json.Unmarshal(data, &lr); err != nil {
			return nil, fmt.Errorf("failed to parse last results: %w", err)
		}
		return &lr, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read last results: %w", err)
	}

	// Fallback: legacy last-query.json
	lq, err := lastquery.Read(vaultPath)
	if err != nil {
		if errors.Is(err, lastquery.ErrNoLastQuery) {
			return nil, ErrNoLastResults
		}
		return nil, fmt.Errorf("failed to read last query: %w", err)
	}

	lr, err := fromLegacyLastQuery(lq)
	if err != nil {
		return nil, err
	}
	return lr, nil
}

// NewFromResults builds a LastResults from model results.
func NewFromResults(source Source, query, target string, results []model.Result) (*LastResults, error) {
	encoded, err := encodeResults(results)
	if err != nil {
		return nil, err
	}

	return &LastResults{
		Source:    source,
		Query:     query,
		Target:    target,
		Timestamp: time.Now(),
		Results:   encoded,
	}, nil
}

// GetByNumbers returns the results matching the given numbers (1-indexed).
func (lr *LastResults) GetByNumbers(nums []int) ([]model.Result, error) {
	results := make([]model.Result, 0, len(nums))

	for _, num := range nums {
		if num < 1 || num > len(lr.Results) {
			return nil, fmt.Errorf("%w: %d (valid range: 1-%d)", ErrNumberOutOfRange, num, len(lr.Results))
		}
		decoded, err := lr.Results[num-1].Decode()
		if err != nil {
			return nil, err
		}
		results = append(results, decoded)
	}

	return results, nil
}

// DecodeAll returns all results decoded into model types.
func (lr *LastResults) DecodeAll() ([]model.Result, error) {
	results := make([]model.Result, len(lr.Results))
	for i, stored := range lr.Results {
		decoded, err := stored.Decode()
		if err != nil {
			return nil, err
		}
		results[i] = decoded
	}
	return results, nil
}

// DecodeObjects decodes results as objects (errors if any result is not an object).
func (lr *LastResults) DecodeObjects() ([]model.Object, error) {
	return decodeResults[model.Object](lr.Results, "object")
}

// DecodeTraits decodes results as traits (errors if any result is not a trait).
func (lr *LastResults) DecodeTraits() ([]model.Trait, error) {
	return decodeResults[model.Trait](lr.Results, "trait")
}

// DecodeReferences decodes results as references (errors if any result is not a reference).
func (lr *LastResults) DecodeReferences() ([]model.Reference, error) {
	return decodeResults[model.Reference](lr.Results, "reference")
}

// DecodeSearchMatches decodes results as search matches (errors if any result is not a search match).
func (lr *LastResults) DecodeSearchMatches() ([]model.SearchMatch, error) {
	return decodeResults[model.SearchMatch](lr.Results, "search")
}

// Decode converts a stored result into the appropriate model type.
func (sr StoredResult) Decode() (model.Result, error) {
	switch sr.Kind {
	case "object":
		var obj model.Object
		if err := json.Unmarshal(sr.Data, &obj); err != nil {
			return nil, fmt.Errorf("failed to parse object result: %w", err)
		}
		return obj, nil
	case "trait":
		var trait model.Trait
		if err := json.Unmarshal(sr.Data, &trait); err != nil {
			return nil, fmt.Errorf("failed to parse trait result: %w", err)
		}
		return trait, nil
	case "reference":
		var ref model.Reference
		if err := json.Unmarshal(sr.Data, &ref); err != nil {
			return nil, fmt.Errorf("failed to parse reference result: %w", err)
		}
		return ref, nil
	case "search":
		var match model.SearchMatch
		if err := json.Unmarshal(sr.Data, &match); err != nil {
			return nil, fmt.Errorf("failed to parse search result: %w", err)
		}
		return match, nil
	default:
		return nil, fmt.Errorf("unknown result kind: %s", sr.Kind)
	}
}

func encodeResults(results []model.Result) ([]StoredResult, error) {
	encoded := make([]StoredResult, len(results))
	for i, result := range results {
		item, err := newStoredResult(result)
		if err != nil {
			return nil, err
		}
		encoded[i] = item
	}
	return encoded, nil
}

func newStoredResult(result model.Result) (StoredResult, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return StoredResult{}, fmt.Errorf("failed to marshal %s result: %w", result.GetKind(), err)
	}
	return StoredResult{
		Kind: result.GetKind(),
		Data: data,
	}, nil
}

func decodeResults[T any](results []StoredResult, kind string) ([]T, error) {
	decoded := make([]T, 0, len(results))
	for _, stored := range results {
		if stored.Kind != kind {
			return nil, fmt.Errorf("expected %s result, found %s", kind, stored.Kind)
		}
		var item T
		if err := json.Unmarshal(stored.Data, &item); err != nil {
			return nil, fmt.Errorf("failed to parse %s result: %w", kind, err)
		}
		decoded = append(decoded, item)
	}
	return decoded, nil
}

func fromLegacyLastQuery(lq *lastquery.LastQuery) (*LastResults, error) {
	results := make([]model.Result, len(lq.Results))
	for i, entry := range lq.Results {
		result, err := legacyEntryToResult(entry, lq.Type)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}

	encoded, err := encodeResults(results)
	if err != nil {
		return nil, err
	}

	return &LastResults{
		Source:    SourceQuery,
		Query:     lq.Query,
		Timestamp: lq.Timestamp,
		Results:   encoded,
	}, nil
}

func legacyEntryToResult(entry lastquery.ResultEntry, fallbackType string) (model.Result, error) {
	kind := entry.Kind
	if kind == "" {
		kind = fallbackType
	}

	switch kind {
	case "trait":
		return model.Trait{
			ID:             entry.ID,
			TraitType:      entry.TraitType,
			Value:          entry.TraitValue,
			Content:        entry.Content,
			FilePath:       entry.FilePath,
			Line:           entry.Line,
			ParentObjectID: "",
		}, nil
	case "object":
		return model.Object{
			ID:        entry.ID,
			Type:      entry.ObjectType,
			Fields:    entry.Fields,
			FilePath:  entry.FilePath,
			LineStart: entry.Line,
			ParentID:  nil,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported legacy result kind: %s", kind)
	}
}
