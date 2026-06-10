package editsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
)

type Code = codes.ErrorCode

const (
	CodeInvalidInput    Code = codes.ErrInvalidInput
	CodeStringNotFound  Code = codes.ErrStringNotFound
	CodeMultipleMatches Code = codes.ErrMultipleMatches
)

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Details    map[string]string
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

func newError(code Code, message, suggestion string, details map[string]string, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Details: details, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type EditSpec struct {
	OldStr string `json:"old_str"`
	NewStr string `json:"new_str"`
}

type editBatchInput struct {
	Edits []EditSpec `json:"edits"`
}

type EditResult struct {
	Index   int
	Line    int
	OldStr  string
	NewStr  string
	Before  string
	After   string
	Context string
}

// EditScope restricts edit matching to an inclusive line range in the file.
// EndLine <= 0 means the range continues to EOF.
type EditScope struct {
	StartLine int
	EndLine   int
}

func ParseEditsJSON(raw string) ([]EditSpec, error) {
	var input editBatchInput
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, newError(CodeInvalidInput, "invalid --edits-json payload", `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`, map[string]string{"error": err.Error()}, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, newError(CodeInvalidInput, "invalid --edits-json payload", `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`, map[string]string{"error": "unexpected trailing content"}, nil)
		}
		return nil, newError(CodeInvalidInput, "invalid --edits-json payload", `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`, map[string]string{"error": err.Error()}, err)
	}
	if len(input.Edits) == 0 {
		return nil, newError(CodeInvalidInput, "invalid --edits-json payload", `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`, map[string]string{"error": "edits must contain at least one item"}, nil)
	}
	for i, edit := range input.Edits {
		if edit.OldStr == "" {
			return nil, newError(CodeInvalidInput, "invalid --edits-json payload", `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`, map[string]string{"error": fmt.Sprintf("edits[%d].old_str must be non-empty", i)}, nil)
		}
	}
	return input.Edits, nil
}

func ApplyEditsInMemory(content, relPath string, edits []EditSpec) (string, []EditResult, error) {
	return ApplyEditsInMemoryWithScope(content, relPath, edits, nil)
}

func ApplyEditsInMemoryWithScope(content, relPath string, edits []EditSpec, scope *EditScope) (string, []EditResult, error) {
	updated := content
	results := make([]EditResult, 0, len(edits))
	activeScope := normalizeScope(scope)

	for i, edit := range edits {
		searchStart, searchEnd := 0, len(updated)
		if activeScope != nil {
			searchStart, searchEnd = lineRangeOffsets(updated, *activeScope)
		}
		searchContent := updated[searchStart:searchEnd]

		count := strings.Count(searchContent, edit.OldStr)
		editIndex := i + 1
		if count == 0 {
			message := "old_str not found in file"
			if activeScope != nil {
				message = "old_str not found in selected range"
			}
			return "", nil, newError(
				CodeStringNotFound,
				message,
				"Check the exact string including whitespace",
				errorDetails(relPath, editIndex, edit.OldStr, activeScope),
				nil,
			)
		}
		if count > 1 {
			message := fmt.Sprintf("old_str found %d times in file", count)
			if activeScope != nil {
				message = fmt.Sprintf("old_str found %d times in selected range", count)
			}
			return "", nil, newError(
				CodeMultipleMatches,
				message,
				"Include more surrounding context to make the match unique",
				errorDetails(relPath, editIndex, "", activeScope, map[string]string{
					"count": fmt.Sprintf("%d", count),
				}),
				nil,
			)
		}

		matchIndex := searchStart + strings.Index(searchContent, edit.OldStr)
		lineNumber := strings.Count(updated[:matchIndex], "\n") + 1
		beforeContext := extractContext(updated, matchIndex)
		afterContext := extractContextAfterReplace(updated, edit.OldStr, edit.NewStr, matchIndex)

		updated = replaceAtIndex(updated, matchIndex, edit.OldStr, edit.NewStr)
		if activeScope != nil {
			activeScope.EndLine = adjustEndLine(activeScope.EndLine, edit.OldStr, edit.NewStr)
		}
		results = append(results, EditResult{
			Index:   editIndex,
			Line:    lineNumber,
			OldStr:  edit.OldStr,
			NewStr:  edit.NewStr,
			Before:  beforeContext,
			After:   afterContext,
			Context: afterContext,
		})
	}

	return updated, results, nil
}

func normalizeScope(scope *EditScope) *EditScope {
	if scope == nil || scope.StartLine <= 0 {
		return nil
	}
	normalized := *scope
	if normalized.EndLine > 0 && normalized.EndLine < normalized.StartLine {
		normalized.EndLine = normalized.StartLine
	}
	return &normalized
}

func errorDetails(relPath string, editIndex int, oldStr string, scope *EditScope, extra ...map[string]string) map[string]string {
	details := map[string]string{
		"path":       relPath,
		"edit_index": fmt.Sprintf("%d", editIndex),
	}
	if oldStr != "" {
		details["old_str"] = oldStr
	}
	if scope != nil {
		details["scope_start_line"] = fmt.Sprintf("%d", scope.StartLine)
		if scope.EndLine > 0 {
			details["scope_end_line"] = fmt.Sprintf("%d", scope.EndLine)
		}
	}
	for _, values := range extra {
		for key, value := range values {
			details[key] = value
		}
	}
	return details
}

func lineRangeOffsets(content string, scope EditScope) (int, int) {
	start := lineStartOffset(content, scope.StartLine)
	end := len(content)
	if scope.EndLine > 0 {
		end = lineEndOffset(content, scope.EndLine)
	}
	if start > end {
		start = end
	}
	return start, end
}

func lineStartOffset(content string, line int) int {
	if line <= 1 {
		return 0
	}
	currentLine := 1
	for i := 0; i < len(content); i++ {
		if content[i] != '\n' {
			continue
		}
		currentLine++
		if currentLine == line {
			return i + 1
		}
	}
	return len(content)
}

func lineEndOffset(content string, line int) int {
	if line <= 0 {
		return len(content)
	}
	currentLine := 1
	for i := 0; i < len(content); i++ {
		if content[i] != '\n' {
			continue
		}
		if currentLine == line {
			return i + 1
		}
		currentLine++
	}
	return len(content)
}

func replaceAtIndex(content string, matchIndex int, oldStr, newStr string) string {
	return content[:matchIndex] + newStr + content[matchIndex+len(oldStr):]
}

func adjustEndLine(endLine int, oldStr, newStr string) int {
	if endLine <= 0 {
		return endLine
	}
	return endLine + strings.Count(newStr, "\n") - strings.Count(oldStr, "\n")
}

func extractContext(content string, matchIndex int) string {
	lines := strings.Split(content, "\n")

	charCount := 0
	startLine := 0
	for i, line := range lines {
		if charCount+len(line)+1 > matchIndex {
			startLine = i
			break
		}
		charCount += len(line) + 1
	}

	contextStart := startLine
	if contextStart > 0 {
		contextStart--
	}
	contextEnd := startLine + 3
	if contextEnd > len(lines) {
		contextEnd = len(lines)
	}

	return strings.Join(lines[contextStart:contextEnd], "\n")
}

func extractContextAfterReplace(content, oldStr, newStr string, matchIndex int) string {
	newContent := replaceAtIndex(content, matchIndex, oldStr, newStr)
	newMatchIndex := matchIndex
	if newMatchIndex > len(newContent) {
		newMatchIndex = len(newContent) - 1
	}
	if newMatchIndex < 0 {
		newMatchIndex = 0
	}
	return extractContext(newContent, newMatchIndex)
}
