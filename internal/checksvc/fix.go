package checksvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

type FixType string

const (
	FixTypeWikilink FixType = "wikilink"
	FixTypeTrait    FixType = "trait"
	FixTypeMoveFile FixType = "move_file"
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

	// Move-only fields (FixType == FixTypeMoveFile).
	NewFilePath    string
	SourceObjectID string
	DestObjectID   string
}

type FixResult struct {
	FileCount  int
	IssueCount int
	Skipped    []SkippedFix
}

type SkippedFix struct {
	FilePath    string          `json:"file_path"`
	Line        int             `json:"line"`
	IssueType   check.IssueType `json:"issue_type"`
	Description string          `json:"description"`
	Reason      string          `json:"reason"`
}

type FileFixes struct {
	FilePath string
	Fixes    []FixableIssue
}

// CollectFixableIssues identifies issues that can be auto-fixed.
// Only truly unambiguous fixes are included.
func CollectFixableIssues(issues []check.Issue, shortRefMap map[string]string, sch *schema.Schema, vaultCfg *config.VaultConfig) []FixableIssue {
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
		case check.IssueNonCanonicalRef:
			if fix := tryFixNonCanonicalRef(issue, vaultCfg); fix != nil {
				fixable = append(fixable, *fix)
			}
		case check.IssueNonCanonicalPath:
			if fix := tryFixNonCanonicalPath(issue, vaultCfg); fix != nil {
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

// ApplyFixes applies the given fixes to the vault. Text fixes (wikilink,
// trait) are batched per file and replaced in place. File moves are applied
// one at a time via objectsvc.MoveFile with reference updates and a per-file
// re-index. Failures are collected as Skipped entries and processing continues
// past them; an error is returned only for unrecoverable I/O issues against
// known-good files.
func ApplyFixes(vaultPath string, fixes []FixableIssue, vaultCfg *config.VaultConfig, sch *schema.Schema) (FixResult, error) {
	result := FixResult{}

	textFixes := make([]FixableIssue, 0, len(fixes))
	moveFixes := make([]FixableIssue, 0)
	for _, fix := range fixes {
		if fix.FixType == FixTypeMoveFile {
			moveFixes = append(moveFixes, fix)
			continue
		}
		textFixes = append(textFixes, fix)
	}

	textResult, err := applyTextFixes(vaultPath, textFixes)
	if err != nil {
		return result, err
	}
	result.FileCount += textResult.FileCount
	result.IssueCount += textResult.IssueCount
	result.Skipped = append(result.Skipped, textResult.Skipped...)

	moveResult := applyMoveFixes(vaultPath, vaultCfg, sch, moveFixes)
	result.FileCount += moveResult.FileCount
	result.IssueCount += moveResult.IssueCount
	result.Skipped = append(result.Skipped, moveResult.Skipped...)

	return result, nil
}

func applyTextFixes(vaultPath string, fixes []FixableIssue) (FixResult, error) {
	result := FixResult{}
	if len(fixes) == 0 {
		return result, nil
	}

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
				result.Skipped = append(result.Skipped, skippedFix(fix, "unsupported fix type"))
				continue
			}
			if !strings.Contains(newContent, oldPattern) {
				result.Skipped = append(result.Skipped, skippedFix(fix, "expected content no longer present in file"))
				continue
			}
			newContent = strings.ReplaceAll(newContent, oldPattern, newPattern)
			fixedCount++
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

func applyMoveFixes(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, fixes []FixableIssue) FixResult {
	result := FixResult{}
	if len(fixes) == 0 {
		return result
	}

	parseOpts := parserOptionsFor(vaultCfg)

	sort.Slice(fixes, func(i, j int) bool {
		return fixes[i].FilePath < fixes[j].FilePath
	})

	for _, fix := range fixes {
		sourceAbs := filepath.Join(vaultPath, fix.FilePath)
		destAbs := filepath.Join(vaultPath, fix.NewFilePath)

		if _, err := os.Stat(sourceAbs); err != nil {
			result.Skipped = append(result.Skipped, skippedFix(fix, fmt.Sprintf("source file no longer exists: %v", err)))
			continue
		}
		if _, err := os.Stat(destAbs); err == nil {
			result.Skipped = append(result.Skipped, skippedFix(fix, fmt.Sprintf("destination already exists: %s", fix.NewFilePath)))
			continue
		}

		if err := objectsvc.ValidateContentMutationRelPath(vaultCfg, fix.NewFilePath); err != nil {
			result.Skipped = append(result.Skipped, skippedFix(fix, err.Error()))
			continue
		}

		_, err := objectsvc.MoveFile(objectsvc.MoveFileRequest{
			VaultPath:         vaultPath,
			SourceFile:        sourceAbs,
			DestinationFile:   destAbs,
			SourceObjectID:    fix.SourceObjectID,
			DestinationObject: fix.DestObjectID,
			UpdateRefs:        true,
			FailOnIndexError:  true,
			VaultConfig:       vaultCfg,
			Schema:            sch,
			ParseOptions:      parseOpts,
		})
		if err != nil {
			result.Skipped = append(result.Skipped, skippedFix(fix, fmt.Sprintf("move failed: %v", err)))
			continue
		}

		result.FileCount++
		result.IssueCount++
	}

	return result
}

func parserOptionsFor(vaultCfg *config.VaultConfig) *parser.ParseOptions {
	if vaultCfg == nil {
		return &parser.ParseOptions{}
	}
	return &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}
}

// tryFixNonCanonicalRef builds a wikilink text fix that strips the configured
// root prefix from a ref target. Returns nil if no configured root matches the
// ref value (defensive — detection should have already filtered).
func tryFixNonCanonicalRef(issue check.Issue, vaultCfg *config.VaultConfig) *FixableIssue {
	if vaultCfg == nil {
		return nil
	}
	roots := uniqueNonEmpty(
		paths.NormalizeDirRoot(vaultCfg.GetObjectsRoot()),
		paths.NormalizeDirRoot(vaultCfg.GetPagesRoot()),
	)
	stripped, matched := stripRootPrefix(issue.Value, roots)
	if !matched || stripped == "" || stripped == issue.Value {
		return nil
	}
	return &FixableIssue{
		FilePath:    issue.FilePath,
		Line:        issue.Line,
		IssueType:   issue.Type,
		FixType:     FixTypeWikilink,
		OldValue:    issue.Value,
		NewValue:    stripped,
		Description: fmt.Sprintf("[[%s]] -> [[%s]]", issue.Value, stripped),
	}
}

// tryFixNonCanonicalPath turns a non_canonical_path finding into a fix
// instruction that ApplyFixes can execute as a file move. The Value field of
// the issue is encoded as "<source> -> <dest>" by the detector when a
// canonical destination was computed; if the value is the bare source path
// (no arrow), no auto-fix is possible and we return nil so the issue surfaces
// as a manual-review finding.
func tryFixNonCanonicalPath(issue check.Issue, vaultCfg *config.VaultConfig) *FixableIssue {
	if vaultCfg == nil {
		return nil
	}
	source, dest, ok := splitMoveValue(issue.Value)
	if !ok || source == "" || dest == "" || source == dest {
		return nil
	}
	srcID := vaultCfg.FilePathToObjectID(strings.TrimSuffix(source, ".md"))
	destID := vaultCfg.FilePathToObjectID(strings.TrimSuffix(dest, ".md"))
	if srcID == "" || destID == "" {
		return nil
	}
	return &FixableIssue{
		FilePath:       source,
		Line:           issue.Line,
		IssueType:      issue.Type,
		FixType:        FixTypeMoveFile,
		OldValue:       source,
		NewValue:       dest,
		NewFilePath:    dest,
		SourceObjectID: srcID,
		DestObjectID:   destID,
		Description:    fmt.Sprintf("%s -> %s", source, dest),
	}
}

func splitMoveValue(value string) (source, dest string, ok bool) {
	const sep = " -> "
	idx := strings.Index(value, sep)
	if idx == -1 {
		return "", "", false
	}
	return strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx+len(sep):]), true
}

func skippedFix(fix FixableIssue, reason string) SkippedFix {
	return SkippedFix{
		FilePath:    fix.FilePath,
		Line:        fix.Line,
		IssueType:   fix.IssueType,
		Description: fix.Description,
		Reason:      reason,
	}
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
	if !exists || traitDef == nil || traitDef.Type != schema.FieldTypeEnum {
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
