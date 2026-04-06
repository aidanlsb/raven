package objectsvc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/filelock"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

// ResolveAddHeadingTarget resolves an add --heading spec to an embedded target object ID.
func ResolveAddHeadingTarget(
	vaultPath string,
	destPath string,
	fileObjectID string,
	headingSpec string,
	parseOpts *parser.ParseOptions,
) (string, error) {
	spec := strings.TrimSpace(headingSpec)
	if spec == "" {
		return "", nil
	}

	contentBytes, err := os.ReadFile(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to read target file")
	}
	doc, err := parser.ParseDocumentWithOptions(string(contentBytes), destPath, vaultPath, parseOpts)
	if err != nil {
		return "", fmt.Errorf("failed to parse target file")
	}

	prefix := fileObjectID + "#"
	candidates := make([]*parser.ParsedObject, 0, len(doc.Objects))
	for _, obj := range doc.Objects {
		if obj == nil {
			continue
		}
		if strings.HasPrefix(obj.ID, prefix) {
			candidates = append(candidates, obj)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("target file has no embedded sections: %s", fileObjectID)
	}

	if headingText, ok := parseHeadingTextFromSpec(spec); ok {
		return resolveSectionByHeadingText(candidates, headingText)
	}
	if strings.Contains(spec, " ") {
		return resolveSectionByHeadingText(candidates, spec)
	}
	if strings.Contains(spec, "/") && strings.Contains(spec, "#") {
		if !strings.HasPrefix(spec, prefix) {
			return "", fmt.Errorf("section %q does not belong to %s", spec, fileObjectID)
		}
		for _, obj := range candidates {
			if obj.ID == spec {
				return obj.ID, nil
			}
		}
		return "", fmt.Errorf("section not found: %s", spec)
	}

	fragment := strings.TrimSpace(strings.TrimPrefix(spec, "#"))
	if fragment == "" {
		return "", fmt.Errorf("section fragment cannot be empty")
	}
	for _, obj := range candidates {
		if strings.TrimPrefix(obj.ID, prefix) == fragment {
			return obj.ID, nil
		}
	}
	return "", fmt.Errorf("section fragment not found: %s", fragment)
}

// AppendToFile appends a capture line to the target file, creating daily notes when needed.
func AppendToFile(
	vaultPath string,
	destPath string,
	line string,
	cfg *config.CaptureConfig,
	vaultCfg *config.VaultConfig,
	isDailyNote bool,
	targetObjectID string,
	parseOpts *parser.ParseOptions,
) (int, error) {
	fileExists := true
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		fileExists = false
	}

	if !fileExists {
		if !isDailyNote {
			return 0, fmt.Errorf("file does not exist: %s", destPath)
		}

		base := filepath.Base(destPath)
		dateStr := strings.TrimSuffix(base, ".md")
		t, err := time.Parse(dates.DateLayout, dateStr)
		if err != nil {
			t = time.Now()
			dateStr = vault.FormatDateISO(t)
		}
		friendlyTitle := vault.FormatDateFriendly(t)
		dailyDir := vaultCfg.GetDailyDirectory()
		if dailyDir == "" {
			dailyDir = "daily"
		}
		s, err := schema.Load(vaultPath)
		if err != nil {
			return 0, fmt.Errorf("failed to load schema: %w", err)
		}
		if _, err := pages.CreateDailyNoteWithSchema(vaultPath, dailyDir, dateStr, friendlyTitle, s, vaultCfg.GetTemplateDirectory(), vaultCfg.ProtectedPrefixes); err != nil {
			return 0, fmt.Errorf("failed to create daily note: %w", err)
		}
	}

	if targetObjectID != "" && strings.Contains(targetObjectID, "#") {
		return appendWithinObject(vaultPath, destPath, line, targetObjectID, parseOpts)
	}

	if cfg != nil && cfg.Heading != "" {
		return appendUnderHeading(destPath, line, cfg.Heading)
	}

	f, err := os.OpenFile(destPath, os.O_APPEND|os.O_RDWR, 0o644)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	if err := filelock.LockExclusive(f); err != nil {
		return 0, fmt.Errorf("failed to lock file: %w", err)
	}
	defer func() {
		_ = filelock.Unlock(f)
	}()

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek file: %w", err)
	}
	content, err := io.ReadAll(f)
	if err != nil {
		return 0, fmt.Errorf("failed to read file: %w", err)
	}
	insertedLine := appendedLineNumber(content)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return 0, fmt.Errorf("failed to write newline: %w", err)
		}
	}

	if _, err := f.WriteString(line + "\n"); err != nil {
		return 0, fmt.Errorf("failed to write capture: %w", err)
	}

	return insertedLine, nil
}

// FileLineCount returns the number of lines in a file.
func FileLineCount(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(content), "\n")
}

func appendWithinObject(vaultPath, destPath, line, objectID string, parseOpts *parser.ParseOptions) (int, error) {
	contentBytes, err := os.ReadFile(destPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read file: %w", err)
	}
	content := string(contentBytes)
	lines := strings.Split(content, "\n")

	doc, err := parser.ParseDocumentWithOptions(content, destPath, vaultPath, parseOpts)
	if err != nil {
		return 0, fmt.Errorf("failed to parse document: %w", err)
	}

	var target *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj != nil && obj.ID == objectID {
			target = obj
			break
		}
	}
	if target == nil {
		return 0, fmt.Errorf("target section not found: %s", objectID)
	}

	insertIdx := len(lines)
	if target.LineEnd != nil {
		insertIdx = *target.LineEnd
		if insertIdx < 0 {
			insertIdx = 0
		}
		if insertIdx > len(lines) {
			insertIdx = len(lines)
		}
	}

	minInsertIdx := target.LineStart
	if minInsertIdx < 0 {
		minInsertIdx = 0
	}
	if minInsertIdx > len(lines) {
		minInsertIdx = len(lines)
	}
	for insertIdx > minInsertIdx && strings.TrimSpace(lines[insertIdx-1]) == "" {
		insertIdx--
	}
	insertedLine := insertIdx + 1

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, line)
	newLines = append(newLines, lines[insertIdx:]...)

	if err := atomicfile.WriteFile(destPath, []byte(strings.Join(newLines, "\n")), 0o644); err != nil {
		return 0, err
	}
	return insertedLine, nil
}

func appendUnderHeading(destPath, line, heading string) (int, error) {
	content, err := os.ReadFile(destPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	headingIdx := -1
	nextHeadingIdx := -1
	headingLevel := strings.Count(strings.Split(heading, " ")[0], "#")

	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == heading {
			headingIdx = i
			continue
		}
		if headingIdx >= 0 && strings.HasPrefix(trimmed, "#") {
			level := 0
			for _, c := range trimmed {
				if c == '#' {
					level++
				} else {
					break
				}
			}
			if level <= headingLevel {
				nextHeadingIdx = i
				break
			}
		}
	}

	var (
		newLines     []string
		insertedLine int
	)
	if headingIdx == -1 {
		newLines = append(lines, "", heading, line)
		insertedLine = len(newLines)
	} else if nextHeadingIdx == -1 {
		insertIdx := len(lines)
		for insertIdx > headingIdx+1 && strings.TrimSpace(lines[insertIdx-1]) == "" {
			insertIdx--
		}
		insertedLine = insertIdx + 1
		newLines = append(lines[:insertIdx], line)
		newLines = append(newLines, lines[insertIdx:]...)
	} else {
		insertIdx := nextHeadingIdx
		for insertIdx > headingIdx+1 && strings.TrimSpace(lines[insertIdx-1]) == "" {
			insertIdx--
		}
		insertedLine = insertIdx + 1
		newLines = append(lines[:insertIdx], line)
		newLines = append(newLines, lines[insertIdx:]...)
	}

	if err := atomicfile.WriteFile(destPath, []byte(strings.Join(newLines, "\n")), 0o644); err != nil {
		return 0, err
	}
	return insertedLine, nil
}

func appendedLineNumber(content []byte) int {
	if len(content) == 0 {
		return 1
	}
	lineCount := strings.Count(string(content), "\n")
	if content[len(content)-1] != '\n' {
		return lineCount + 2
	}
	return lineCount + 1
}

func resolveSectionByHeadingText(candidates []*parser.ParsedObject, headingText string) (string, error) {
	text := strings.TrimSpace(headingText)
	if text == "" {
		return "", fmt.Errorf("heading text cannot be empty")
	}

	matches := make([]string, 0, 2)
	for _, obj := range candidates {
		if obj == nil || obj.Heading == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(*obj.Heading), text) {
			matches = append(matches, obj.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("heading not found: %q", text)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("heading %q is ambiguous; use a section slug/id", text)
	}
}

func parseHeadingTextFromSpec(spec string) (string, bool) {
	trimmed := strings.TrimSpace(spec)
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	if i == 0 || i >= len(trimmed) || trimmed[i] != ' ' {
		return "", false
	}
	headingText := strings.TrimSpace(trimmed[i:])
	if headingText == "" {
		return "", false
	}
	return headingText, true
}
