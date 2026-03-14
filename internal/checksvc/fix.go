package checksvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/schema"
)

type FixType string

const (
	FixTypeWikilink FixType = "wikilink"
	FixTypeTrait    FixType = "trait"
)

type FixableIssue struct {
	FilePath    string
	Line        int
	IssueType   check.IssueType
	FixType     FixType
	OldValue    string
	NewValue    string
	TraitName   string
	Description string
}

type FixResult struct {
	FileCount  int
	IssueCount int
}

type FileFixes struct {
	FilePath string
	Fixes    []FixableIssue
}

// CollectFixableIssues identifies issues that can be auto-fixed.
// Only truly unambiguous fixes are included.
func CollectFixableIssues(issues []check.Issue, shortRefMap map[string]string, sch *schema.Schema) []FixableIssue {
	var fixable []FixableIssue

	for _, issue := range issues {
		switch issue.Type {
		case check.IssueShortRefCouldBeFullPath:
			if fullPath, ok := shortRefMap[issue.Value]; ok {
				fixable = append(fixable, FixableIssue{
					FilePath:    issue.FilePath,
					Line:        issue.Line,
					IssueType:   issue.Type,
					FixType:     FixTypeWikilink,
					OldValue:    issue.Value,
					NewValue:    fullPath,
					Description: fmt.Sprintf("[[%s]] -> [[%s]]", issue.Value, fullPath),
				})
			}
		case check.IssueInvalidEnumValue:
			if fix := tryFixQuotedEnumValue(issue, sch); fix != nil {
				fixable = append(fixable, *fix)
			}
		}
	}

	return fixable
}

func GroupFixesByFile(fixes []FixableIssue) []FileFixes {
	grouped := make(map[string][]FixableIssue)
	for _, fix := range fixes {
		grouped[fix.FilePath] = append(grouped[fix.FilePath], fix)
	}

	var files []string
	for fp := range grouped {
		files = append(files, fp)
	}
	sort.Strings(files)

	result := make([]FileFixes, 0, len(files))
	for _, fp := range files {
		fileFixes := grouped[fp]
		sort.Slice(fileFixes, func(i, j int) bool {
			return fileFixes[i].Line < fileFixes[j].Line
		})
		result = append(result, FileFixes{
			FilePath: fp,
			Fixes:    fileFixes,
		})
	}
	return result
}

func ApplyFixes(vaultPath string, fixes []FixableIssue) (FixResult, error) {
	result := FixResult{}

	grouped := make(map[string][]FixableIssue)
	for _, fix := range fixes {
		grouped[fix.FilePath] = append(grouped[fix.FilePath], fix)
	}

	var files []string
	for fp := range grouped {
		files = append(files, fp)
	}
	sort.Strings(files)

	for _, filePath := range files {
		fileFixes := grouped[filePath]
		fullPath := filepath.Join(vaultPath, filePath)

		content, err := os.ReadFile(fullPath)
		if err != nil {
			return result, fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		newContent := string(content)
		fixedCount := 0

		sort.Slice(fileFixes, func(i, j int) bool {
			return fileFixes[i].Line > fileFixes[j].Line
		})

		for _, fix := range fileFixes {
			var oldPattern, newPattern string
			switch fix.FixType {
			case FixTypeWikilink:
				oldPattern = "[[" + fix.OldValue + "]]"
				newPattern = "[[" + fix.NewValue + "]]"
			case FixTypeTrait:
				oldPattern = "@" + fix.TraitName + "(" + fix.OldValue + ")"
				newPattern = "@" + fix.TraitName + "(" + fix.NewValue + ")"
			default:
				continue
			}
			if strings.Contains(newContent, oldPattern) {
				newContent = strings.ReplaceAll(newContent, oldPattern, newPattern)
				fixedCount++
			}
		}

		if fixedCount > 0 {
			if err := os.WriteFile(fullPath, []byte(newContent), 0o644); err != nil {
				return result, fmt.Errorf("failed to write %s: %w", filePath, err)
			}
			result.FileCount++
			result.IssueCount += fixedCount
		}
	}

	return result, nil
}

func tryFixQuotedEnumValue(issue check.Issue, sch *schema.Schema) *FixableIssue {
	if sch == nil {
		return nil
	}

	value := issue.Value
	var unquoted string
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			unquoted = value[1 : len(value)-1]
		}
	}
	if unquoted == "" {
		return nil
	}

	traitName := extractTraitNameFromMessage(issue.Message)
	if traitName == "" {
		return nil
	}

	traitDef, exists := sch.Traits[traitName]
	if !exists || traitDef.Type != schema.FieldTypeEnum {
		return nil
	}

	for _, allowed := range traitDef.Values {
		if allowed == unquoted {
			return &FixableIssue{
				FilePath:    issue.FilePath,
				Line:        issue.Line,
				IssueType:   issue.Type,
				FixType:     FixTypeTrait,
				OldValue:    value,
				NewValue:    unquoted,
				TraitName:   traitName,
				Description: fmt.Sprintf("@%s(%s) -> @%s(%s)", traitName, value, traitName, unquoted),
			}
		}
	}

	return nil
}

func extractTraitNameFromMessage(msg string) string {
	const prefix = "for trait '@"
	idx := strings.Index(msg, prefix)
	if idx == -1 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.Index(msg[start:], "'")
	if end == -1 {
		return ""
	}
	return msg[start : start+end]
}
