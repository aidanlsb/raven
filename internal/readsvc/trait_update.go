package readsvc

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type TraitUpdateResult struct {
	ID       string `json:"id"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value,omitempty"`
}

type TraitUpdatePreviewItem struct {
	ID        string `json:"id"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	TraitType string `json:"trait_type"`
	Content   string `json:"content"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
}

type TraitUpdatePreview struct {
	Action  string                   `json:"action"`
	Items   []TraitUpdatePreviewItem `json:"items"`
	Skipped []TraitUpdateResult      `json:"skipped,omitempty"`
	Total   int                      `json:"total"`
}

type TraitUpdateSummary struct {
	Action   string              `json:"action"`
	Results  []TraitUpdateResult `json:"results"`
	Total    int                 `json:"total"`
	Modified int                 `json:"modified"`
	Skipped  int                 `json:"skipped"`
	Errors   int                 `json:"errors"`
}

type TraitValueValidationError struct {
	TraitType string
	Cause     error
}

func (e *TraitValueValidationError) Error() string {
	return fmt.Sprintf("invalid value for trait '@%s': %v", e.TraitType, e.Cause)
}

func (e *TraitValueValidationError) Suggestion() string {
	return fmt.Sprintf("Use a value compatible with trait '@%s' in schema.yaml", e.TraitType)
}

func PreviewTraitUpdate(traits []model.Trait, newValue string, sch *schema.Schema) (*TraitUpdatePreview, error) {
	resolvedValues, err := precomputeTraitUpdateValues(traits, newValue, sch)
	if err != nil {
		return nil, err
	}

	items := make([]TraitUpdatePreviewItem, 0, len(traits))
	skipped := make([]TraitUpdateResult, 0)

	for _, t := range traits {
		resolvedNewValue := resolvedValues[t.ID]
		oldValue := getTraitCurrentValue(t, sch)

		if oldValue == resolvedNewValue {
			skipped = append(skipped, TraitUpdateResult{
				ID:       t.ID,
				FilePath: t.FilePath,
				Line:     t.Line,
				Status:   "skipped",
				Reason:   "already has target value",
			})
			continue
		}

		items = append(items, TraitUpdatePreviewItem{
			ID:        t.ID,
			FilePath:  t.FilePath,
			Line:      t.Line,
			TraitType: t.TraitType,
			Content:   t.Content,
			OldValue:  oldValue,
			NewValue:  resolvedNewValue,
		})
	}

	return &TraitUpdatePreview{
		Action:  "update-trait",
		Items:   items,
		Skipped: skipped,
		Total:   len(items),
	}, nil
}

func ApplyTraitUpdate(
	vaultPath string,
	traits []model.Trait,
	newValue string,
	sch *schema.Schema,
	onModified func(filePath string),
) (*TraitUpdateSummary, error) {
	resolvedValues, err := precomputeTraitUpdateValues(traits, newValue, sch)
	if err != nil {
		return nil, err
	}

	traitsByFile := make(map[string][]model.Trait)
	for _, t := range traits {
		traitsByFile[t.FilePath] = append(traitsByFile[t.FilePath], t)
	}

	results := make([]TraitUpdateResult, 0, len(traits))
	modified := 0
	skipped := 0
	errorCount := 0

	for filePath, fileTraits := range traitsByFile {
		fullPath := filePath
		if !strings.HasPrefix(filePath, vaultPath) {
			fullPath = vaultPath + "/" + filePath
		}

		content, err := osReadFile(fullPath)
		if err != nil {
			for _, t := range fileTraits {
				results = append(results, TraitUpdateResult{
					ID:       t.ID,
					FilePath: t.FilePath,
					Line:     t.Line,
					Status:   "error",
					Reason:   fmt.Sprintf("failed to read file: %v", err),
				})
				errorCount++
			}
			continue
		}

		lines := strings.Split(string(content), "\n")
		fileModified := false

		for _, t := range fileTraits {
			resolvedNewValue := resolvedValues[t.ID]
			lineIdx := t.Line - 1
			if lineIdx < 0 || lineIdx >= len(lines) {
				results = append(results, TraitUpdateResult{
					ID:       t.ID,
					FilePath: t.FilePath,
					Line:     t.Line,
					Status:   "error",
					Reason:   "line number out of range",
				})
				errorCount++
				continue
			}

			oldValue := getTraitCurrentValue(t, sch)
			if oldValue == resolvedNewValue {
				results = append(results, TraitUpdateResult{
					ID:       t.ID,
					FilePath: t.FilePath,
					Line:     t.Line,
					Status:   "skipped",
					Reason:   "already has target value",
				})
				skipped++
				continue
			}

			oldLine := lines[lineIdx]
			newLine, ok := rewriteTraitValue(oldLine, t.TraitType, resolvedNewValue)
			if !ok {
				results = append(results, TraitUpdateResult{
					ID:       t.ID,
					FilePath: t.FilePath,
					Line:     t.Line,
					Status:   "error",
					Reason:   "trait not found on line",
				})
				errorCount++
				continue
			}

			lines[lineIdx] = newLine
			fileModified = true
			results = append(results, TraitUpdateResult{
				ID:       t.ID,
				FilePath: t.FilePath,
				Line:     t.Line,
				Status:   "modified",
				OldValue: oldValue,
				NewValue: resolvedNewValue,
			})
			modified++
		}

		if fileModified {
			newContent := strings.Join(lines, "\n")
			if err := atomicfile.WriteFile(fullPath, []byte(newContent), 0o644); err != nil {
				for i, r := range results {
					if r.FilePath == filePath && r.Status == "modified" {
						results[i].Status = "error"
						results[i].Reason = fmt.Sprintf("failed to write file: %v", err)
						modified--
						errorCount++
					}
				}
				continue
			}
			if onModified != nil {
				onModified(fullPath)
			}
		}
	}

	return &TraitUpdateSummary{
		Action:   "update-trait",
		Results:  results,
		Total:    len(traits),
		Modified: modified,
		Skipped:  skipped,
		Errors:   errorCount,
	}, nil
}

func precomputeTraitUpdateValues(traits []model.Trait, newValue string, sch *schema.Schema) (map[string]string, error) {
	resolved := make(map[string]string, len(traits))
	for _, t := range traits {
		val, err := resolvedAndValidatedTraitValue(newValue, t.TraitType, sch)
		if err != nil {
			return nil, err
		}
		resolved[t.ID] = val
	}
	return resolved, nil
}

func resolvedAndValidatedTraitValue(rawValue, traitType string, sch *schema.Schema) (string, error) {
	if sch == nil {
		return rawValue, nil
	}

	traitDef, ok := sch.Traits[traitType]
	if !ok || traitDef == nil {
		return rawValue, nil
	}

	resolved := resolveDateKeywordForTraitValue(rawValue, traitDef)
	parsed := parser.ParseTraitValue(resolved)
	if err := schema.ValidateTraitValue(traitDef, parsed); err != nil {
		return "", &TraitValueValidationError{
			TraitType: traitType,
			Cause:     err,
		}
	}

	return resolved, nil
}

func resolveDateKeywordForTraitValue(value string, traitDef *schema.TraitDefinition) string {
	if traitDef == nil || traitDef.Type != schema.FieldTypeDate {
		return value
	}
	resolved, ok := dates.ResolveRelativeDateKeyword(value, timeNow(), timeMonday())
	if !ok || resolved.Kind != dates.RelativeDateInstant {
		return value
	}
	return resolved.Date.Format(dates.DateLayout)
}

func getTraitCurrentValue(t model.Trait, sch *schema.Schema) string {
	if t.Value != nil {
		return *t.Value
	}
	if sch == nil {
		return ""
	}
	if traitDef, ok := sch.Traits[t.TraitType]; ok && traitDef != nil {
		if def, ok := traitDef.Default.(string); ok {
			return def
		}
	}
	return ""
}

func rewriteTraitValue(line, traitType, newValue string) (string, bool) {
	pattern := regexp.MustCompile(`@` + regexp.QuoteMeta(traitType) + `(?:\s*\([^)]*\))?`)
	if !pattern.MatchString(line) {
		return line, false
	}
	newTrait := fmt.Sprintf("@%s(%s)", traitType, newValue)
	return pattern.ReplaceAllString(line, newTrait), true
}

// Test seams.
var (
	osReadFile = os.ReadFile
	timeNow    = time.Now
	timeMonday = func() time.Weekday { return time.Monday }
)
