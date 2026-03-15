package traitsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type Code string

const (
	CodeInvalidInput   Code = "INVALID_INPUT"
	CodeValidation     Code = "VALIDATION_FAILED"
	CodeDatabaseError  Code = "DATABASE_ERROR"
	CodeFileReadError  Code = "FILE_READ_ERROR"
	CodeFileWriteError Code = "FILE_WRITE_ERROR"
	CodeInternalError  Code = "INTERNAL_ERROR"
)

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Details    map[string]interface{}
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, message, suggestion string, details map[string]interface{}, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Details: details, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type ValueValidationError struct {
	TraitType string
	Cause     error
}

func (e *ValueValidationError) Error() string {
	return fmt.Sprintf("invalid value for trait '@%s': %v", e.TraitType, e.Cause)
}

func (e *ValueValidationError) Suggestion() string {
	return fmt.Sprintf("Use a value compatible with trait '@%s' in schema.yaml", e.TraitType)
}

type BulkResult struct {
	ID       string `json:"id"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value,omitempty"`
}

type BulkPreviewItem struct {
	ID        string `json:"id"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	TraitType string `json:"trait_type"`
	Content   string `json:"content"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
}

type BulkPreview struct {
	Action  string            `json:"action"`
	Items   []BulkPreviewItem `json:"items"`
	Skipped []BulkResult      `json:"skipped,omitempty"`
	Total   int               `json:"total"`
}

type BulkSummary struct {
	Action           string       `json:"action"`
	Results          []BulkResult `json:"results"`
	Total            int          `json:"total"`
	Modified         int          `json:"modified"`
	Skipped          int          `json:"skipped"`
	Errors           int          `json:"errors"`
	ChangedFilePaths []string     `json:"-"`
}

func ResolveTraitIDs(db *index.Database, ids []string) ([]model.Trait, []BulkResult, error) {
	results := make([]model.Trait, 0, len(ids))
	var skipped []BulkResult

	for _, id := range ids {
		if !strings.Contains(id, ":trait:") {
			skipped = append(skipped, BulkResult{ID: id, Status: "skipped", Reason: "invalid trait ID format"})
			continue
		}

		trait, err := db.GetTrait(id)
		if err != nil {
			return nil, nil, newError(CodeDatabaseError, "failed to fetch trait from index", "Run 'rvn reindex' to rebuild the database", nil, err)
		}
		if trait == nil {
			filePath := strings.SplitN(id, ":trait:", 2)[0]
			skipped = append(skipped, BulkResult{ID: id, FilePath: filePath, Status: "skipped", Reason: "trait not found in index"})
			continue
		}

		results = append(results, *trait)
	}

	return results, skipped, nil
}

func BuildPreview(traits []model.Trait, newValue string, sch *schema.Schema, extraSkipped []BulkResult) (*BulkPreview, error) {
	resolvedValues, err := precomputeResolvedValues(traits, newValue, sch)
	if err != nil {
		return nil, err
	}

	items := make([]BulkPreviewItem, 0, len(traits))
	skipped := make([]BulkResult, 0, len(extraSkipped))
	skipped = append(skipped, extraSkipped...)

	for _, t := range traits {
		resolvedNewValue := resolvedValues[t.ID]
		oldValue := traitExistingValue(sch, t)
		if oldValue == resolvedNewValue {
			skipped = append(skipped, BulkResult{
				ID:       t.ID,
				FilePath: t.FilePath,
				Line:     t.Line,
				Status:   "skipped",
				Reason:   "already has target value",
			})
			continue
		}

		items = append(items, BulkPreviewItem{
			ID:        t.ID,
			FilePath:  t.FilePath,
			Line:      t.Line,
			TraitType: t.TraitType,
			Content:   t.Content,
			OldValue:  oldValue,
			NewValue:  resolvedNewValue,
		})
	}

	return &BulkPreview{Action: "update-trait", Items: items, Skipped: skipped, Total: len(items)}, nil
}

func ApplyUpdates(vaultPath string, traits []model.Trait, newValue string, sch *schema.Schema, extraSkipped []BulkResult) (*BulkSummary, error) {
	resolvedValues, err := precomputeResolvedValues(traits, newValue, sch)
	if err != nil {
		return nil, err
	}

	traitsByFile := make(map[string][]model.Trait)
	for _, t := range traits {
		traitsByFile[t.FilePath] = append(traitsByFile[t.FilePath], t)
	}

	results := make([]BulkResult, 0, len(traits)+len(extraSkipped))
	results = append(results, extraSkipped...)

	modified := 0
	skipped := len(extraSkipped)
	errored := 0
	changed := make([]string, 0, len(traitsByFile))

	for filePath, fileTraits := range traitsByFile {
		fullPath := filePath
		if !filepath.IsAbs(filePath) {
			fullPath = filepath.Join(vaultPath, filePath)
		}

		content, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			for _, t := range fileTraits {
				results = append(results, BulkResult{ID: t.ID, FilePath: t.FilePath, Line: t.Line, Status: "error", Reason: fmt.Sprintf("failed to read file: %v", readErr)})
				errored++
			}
			continue
		}

		lines := strings.Split(string(content), "\n")
		fileModified := false

		for _, t := range fileTraits {
			resolvedNewValue := resolvedValues[t.ID]
			lineIdx := t.Line - 1
			if lineIdx < 0 || lineIdx >= len(lines) {
				results = append(results, BulkResult{ID: t.ID, FilePath: t.FilePath, Line: t.Line, Status: "error", Reason: "line number out of range"})
				errored++
				continue
			}

			oldValue := traitExistingValue(sch, t)
			if oldValue == resolvedNewValue {
				results = append(results, BulkResult{ID: t.ID, FilePath: t.FilePath, Line: t.Line, Status: "skipped", Reason: "already has target value"})
				skipped++
				continue
			}

			newLine, ok := rewriteTraitValue(lines[lineIdx], t.TraitType, resolvedNewValue)
			if !ok {
				results = append(results, BulkResult{ID: t.ID, FilePath: t.FilePath, Line: t.Line, Status: "error", Reason: "trait not found on line"})
				errored++
				continue
			}

			lines[lineIdx] = newLine
			fileModified = true
			results = append(results, BulkResult{ID: t.ID, FilePath: t.FilePath, Line: t.Line, Status: "modified", OldValue: oldValue, NewValue: resolvedNewValue})
			modified++
		}

		if fileModified {
			if err := atomicfile.WriteFile(fullPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				for i, r := range results {
					if r.FilePath == filePath && r.Status == "modified" {
						results[i].Status = "error"
						results[i].Reason = fmt.Sprintf("failed to write file: %v", err)
						modified--
						errored++
					}
				}
				continue
			}
			changed = append(changed, fullPath)
		}
	}

	return &BulkSummary{
		Action:           "update-trait",
		Results:          results,
		Total:            len(traits) + len(extraSkipped),
		Modified:         modified,
		Skipped:          skipped,
		Errors:           errored,
		ChangedFilePaths: changed,
	}, nil
}

func precomputeResolvedValues(traits []model.Trait, newValue string, sch *schema.Schema) (map[string]string, error) {
	resolved := make(map[string]string, len(traits))
	for _, t := range traits {
		value, err := resolvedAndValidatedTraitValue(newValue, t.TraitType, sch)
		if err != nil {
			return nil, err
		}
		resolved[t.ID] = value
	}
	return resolved, nil
}

func traitExistingValue(sch *schema.Schema, t model.Trait) string {
	if t.Value != nil {
		return *t.Value
	}
	if sch == nil {
		return ""
	}
	if traitDef, ok := sch.Traits[t.TraitType]; ok {
		if def, ok := traitDef.Default.(string); ok {
			return def
		}
	}
	return ""
}

func resolvedAndValidatedTraitValue(rawValue, traitType string, sch *schema.Schema) (string, error) {
	if sch == nil {
		return rawValue, nil
	}
	traitDef, ok := sch.Traits[traitType]
	if !ok || traitDef == nil {
		return rawValue, nil
	}

	resolved := rawValue
	if traitDef.Type == schema.FieldTypeDate {
		if dateValue, ok := resolveRelativeDateKeyword(rawValue); ok {
			resolved = dateValue
		}
	}

	parsed := parser.ParseTraitValue(resolved)
	if err := schema.ValidateTraitValue(traitDef, parsed); err != nil {
		return "", &ValueValidationError{TraitType: traitType, Cause: err}
	}
	return resolved, nil
}

func resolveRelativeDateKeyword(value string) (string, bool) {
	resolved, ok := dates.ResolveRelativeDateKeyword(value, time.Now(), time.Monday)
	if !ok || resolved.Kind != dates.RelativeDateInstant {
		return "", false
	}
	return resolved.Date.Format(dates.DateLayout), true
}

func rewriteTraitValue(line, traitType, newValue string) (string, bool) {
	pattern := regexp.MustCompile(`@` + regexp.QuoteMeta(traitType) + `(?:\s*\([^)]*\))?`)
	if !pattern.MatchString(line) {
		return line, false
	}
	newTrait := fmt.Sprintf("@%s(%s)", traitType, newValue)
	newLine := pattern.ReplaceAllString(line, newTrait)
	return newLine, true
}
