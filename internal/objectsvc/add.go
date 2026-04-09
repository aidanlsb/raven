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
		return "", addFileReadError(destPath, "failed to read target file", "Check that the target file exists and is readable", err)
	}
	doc, err := parser.ParseDocumentWithOptions(string(contentBytes), destPath, vaultPath, parseOpts)
	if err != nil {
		return "", newError(ErrorInvalidInput, "failed to parse target file", "Fix the target file content and try again", nil, err)
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
		return "", newError(ErrorRefNotFound, fmt.Sprintf("target file has no embedded sections: %s", fileObjectID), "Use an existing section slug/id or heading text", nil, nil)
	}

	if headingText, ok := parseHeadingTextFromSpec(spec); ok {
		return resolveSectionByHeadingText(candidates, headingText)
	}
	if strings.Contains(spec, " ") {
		return resolveSectionByHeadingText(candidates, spec)
	}
	if strings.Contains(spec, "/") && strings.Contains(spec, "#") {
		if !strings.HasPrefix(spec, prefix) {
			return "", newError(ErrorInvalidInput, fmt.Sprintf("section %q does not belong to %s", spec, fileObjectID), "Use a section ID from the target file or change --to", nil, nil)
		}
		for _, obj := range candidates {
			if obj.ID == spec {
				return obj.ID, nil
			}
		}
		return "", newError(ErrorRefNotFound, fmt.Sprintf("section not found: %s", spec), "Use an existing section slug/id or heading text", nil, nil)
	}

	fragment := strings.TrimSpace(strings.TrimPrefix(spec, "#"))
	if fragment == "" {
		return "", newError(ErrorInvalidInput, "section fragment cannot be empty", "Pass a non-empty section slug or ID", nil, nil)
	}
	for _, obj := range candidates {
		if strings.TrimPrefix(obj.ID, prefix) == fragment {
			return obj.ID, nil
		}
	}
	return "", newError(ErrorRefNotFound, fmt.Sprintf("section fragment not found: %s", fragment), "Use an existing section slug/id or heading text", nil, nil)
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
			return 0, addFileNotFoundError(destPath, nil)
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
			return 0, newError(ErrorValidationFailed, "failed to load schema", "Fix schema.yaml and try again", nil, err)
		}
		if _, err := pages.CreateDailyNoteWithSchema(vaultPath, dailyDir, dateStr, friendlyTitle, s, vaultCfg.GetTemplateDirectory(), vaultCfg.ProtectedPrefixes); err != nil {
			return 0, addFileWriteError(destPath, "failed to create daily note", "Check the daily note path and try again", err)
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
		return 0, addFileWriteError(destPath, "failed to open target file", "Check that the target file is writable", err)
	}
	defer f.Close()

	if err := filelock.LockExclusive(f); err != nil {
		return 0, addFileWriteError(destPath, "failed to lock target file", "Close other writers and try again", err)
	}
	defer func() {
		_ = filelock.Unlock(f)
	}()

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, addFileReadError(destPath, "failed to seek target file", "Try again", err)
	}
	content, err := io.ReadAll(f)
	if err != nil {
		return 0, addFileReadError(destPath, "failed to read target file", "Check that the target file is readable", err)
	}
	insertedLine := appendedLineNumber(content)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return 0, addFileWriteError(destPath, "failed to write capture newline", "Check that the target file is writable", err)
		}
	}

	if _, err := f.WriteString(line + "\n"); err != nil {
		return 0, addFileWriteError(destPath, "failed to write capture", "Check that the target file is writable", err)
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
		return 0, addFileReadError(destPath, "failed to read target file", "Check that the target file exists and is readable", err)
	}
	content := string(contentBytes)
	lines := strings.Split(content, "\n")

	doc, err := parser.ParseDocumentWithOptions(content, destPath, vaultPath, parseOpts)
	if err != nil {
		return 0, newError(ErrorInvalidInput, "failed to parse target file", "Fix the target file content and try again", nil, err)
	}

	var target *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj != nil && obj.ID == objectID {
			target = obj
			break
		}
	}
	if target == nil {
		return 0, newError(ErrorRefNotFound, fmt.Sprintf("target section not found: %s", objectID), "Use an existing section slug/id or heading text", nil, nil)
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
		return 0, addFileWriteError(destPath, "failed to write updated file", "Check that the target file is writable", err)
	}
	return insertedLine, nil
}

func appendUnderHeading(destPath, line, heading string) (int, error) {
	content, err := os.ReadFile(destPath)
	if err != nil {
		return 0, addFileReadError(destPath, "failed to read target file", "Check that the target file exists and is readable", err)
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
		return 0, addFileWriteError(destPath, "failed to write updated file", "Check that the target file is writable", err)
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
		return "", newError(ErrorInvalidInput, "heading text cannot be empty", "Pass a non-empty heading", nil, nil)
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
		return "", newError(ErrorRefNotFound, fmt.Sprintf("heading not found: %q", text), "Use an existing section slug/id or heading text", nil, nil)
	case 1:
		return matches[0], nil
	default:
		return "", newError(ErrorRefAmbiguous, fmt.Sprintf("heading %q is ambiguous; use a section slug/id", text), "Use a unique section slug/id instead of heading text", nil, nil)
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

func addFileNotFoundError(destPath string, cause error) error {
	return newError(
		ErrorFileNotFound,
		fmt.Sprintf("file does not exist: %s", destPath),
		"Create the file first or choose an existing file",
		nil,
		cause,
	)
}

func addFileReadError(destPath, message, suggestion string, cause error) error {
	if os.IsNotExist(cause) {
		return addFileNotFoundError(destPath, cause)
	}
	return newError(ErrorFileRead, message, suggestion, nil, cause)
}

func addFileWriteError(destPath, message, suggestion string, cause error) error {
	if os.IsNotExist(cause) {
		return addFileNotFoundError(destPath, cause)
	}
	return newError(ErrorFileWrite, message, suggestion, nil, cause)
}
