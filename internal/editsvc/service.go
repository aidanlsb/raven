package editsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Code string

const (
	CodeInvalidInput    Code = "INVALID_INPUT"
	CodeStringNotFound  Code = "STRING_NOT_FOUND"
	CodeMultipleMatches Code = "MULTIPLE_MATCHES"
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
	updated := content
	results := make([]EditResult, 0, len(edits))

	for i, edit := range edits {
		count := strings.Count(updated, edit.OldStr)
		editIndex := i + 1
		if count == 0 {
			return "", nil, newError(
				CodeStringNotFound,
				"old_str not found in file",
				"Check the exact string including whitespace",
				map[string]string{
					"path":       relPath,
					"edit_index": fmt.Sprintf("%d", editIndex),
					"old_str":    edit.OldStr,
				},
				nil,
			)
		}
		if count > 1 {
			return "", nil, newError(
				CodeMultipleMatches,
				fmt.Sprintf("old_str found %d times in file", count),
				"Include more surrounding context to make the match unique",
				map[string]string{
					"path":       relPath,
					"edit_index": fmt.Sprintf("%d", editIndex),
					"count":      fmt.Sprintf("%d", count),
				},
				nil,
			)
		}

		matchIndex := strings.Index(updated, edit.OldStr)
		lineNumber := strings.Count(updated[:matchIndex], "\n") + 1
		beforeContext := extractContext(updated, matchIndex)
		afterContext := extractContextAfterReplace(updated, edit.OldStr, edit.NewStr, matchIndex)

		updated = strings.Replace(updated, edit.OldStr, edit.NewStr, 1)
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
	newContent := strings.Replace(content, oldStr, newStr, 1)
	newMatchIndex := matchIndex
	if newMatchIndex > len(newContent) {
		newMatchIndex = len(newContent) - 1
	}
	if newMatchIndex < 0 {
		newMatchIndex = 0
	}
	return extractContext(newContent, newMatchIndex)
}
