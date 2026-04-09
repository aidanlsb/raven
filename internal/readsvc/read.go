package readsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/wikilink"
)

type ReadRequest struct {
	Reference string
	Raw       bool
	Lines     bool
	StartLine int
	EndLine   int
}

type ReadLine struct {
	Num  int    `json:"num"`
	Text string `json:"text"`
}

type ReadReference struct {
	Text string  `json:"text"`
	Path *string `json:"path,omitempty"`
}

type ReadBacklinkGroup struct {
	Source string   `json:"source"`
	Lines  []string `json:"lines"`
}

type ReadResult struct {
	ObjectID string
	Path     string
	Content  string

	LineCount int
	StartLine int
	EndLine   int
	Lines     []ReadLine

	References     []ReadReference
	Backlinks      []ReadBacklinkGroup
	BacklinksCount int
}

type InvalidLineRangeError struct {
	StartLine int
	EndLine   int
	LineCount int
}

func (e *InvalidLineRangeError) Error() string {
	if e == nil {
		return "invalid line range"
	}
	return fmt.Sprintf(
		"invalid line range: start_line=%d end_line=%d (file has %d lines)",
		e.StartLine,
		e.EndLine,
		e.LineCount,
	)
}

func (e *InvalidLineRangeError) Suggestion() string {
	return "Use 1-indexed inclusive line numbers within the file's line_count"
}

func Read(rt *Runtime, req ReadRequest) (*ReadResult, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}

	reference := strings.TrimSpace(req.Reference)
	if reference == "" {
		return nil, fmt.Errorf("reference is required")
	}

	resolveOp, err := newResolveOperation(rt)
	if err != nil {
		return nil, err
	}
	defer resolveOp.Close()

	resolved, err := resolveOp.resolveReferenceWithDynamicDates(reference, false)
	if err != nil {
		return nil, err
	}

	contentBytes, err := os.ReadFile(resolved.FilePath)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(rt.VaultPath, resolved.FilePath)
	if err != nil {
		return nil, err
	}

	content := string(contentBytes)
	lineCount := strings.Count(content, "\n")
	if len(contentBytes) > 0 && contentBytes[len(contentBytes)-1] != '\n' {
		lineCount++
	}

	result := &ReadResult{
		ObjectID:  resolved.ObjectID,
		Path:      relPath,
		Content:   content,
		LineCount: lineCount,
	}

	rawMode := req.Raw || req.Lines || req.StartLine > 0 || req.EndLine > 0
	if rawMode {
		rawResult, err := readRawRange(content, lineCount, req.StartLine, req.EndLine, req.Lines)
		if err != nil {
			return nil, err
		}
		result.Content = rawResult.Content
		result.StartLine = rawResult.StartLine
		result.EndLine = rawResult.EndLine
		result.Lines = rawResult.Lines
		return result, nil
	}

	if err := ensureReadDB(rt); err != nil {
		return nil, err
	}

	_, body := splitFrontmatterBody(content)
	refs := collectReadReferences(body, rt, resolveOp)

	backlinkGroups, backlinksCount, err := readBacklinksWithContext(rt, resolved.ObjectID)
	if err != nil {
		return nil, err
	}

	result.References = refs
	result.Backlinks = backlinkGroups
	result.BacklinksCount = backlinksCount
	return result, nil
}

type rawReadResult struct {
	Content   string
	StartLine int
	EndLine   int
	Lines     []ReadLine
}

func readRawRange(content string, lineCount, reqStartLine, reqEndLine int, includeLines bool) (*rawReadResult, error) {
	startLine := 1
	endLine := lineCount
	if reqStartLine > 0 {
		startLine = reqStartLine
	}
	if reqEndLine > 0 {
		endLine = reqEndLine
	}

	if startLine < 1 || endLine < 1 || startLine > endLine || endLine > lineCount {
		return nil, &InvalidLineRangeError{
			StartLine: startLine,
			EndLine:   endLine,
			LineCount: lineCount,
		}
	}

	type lineRange struct {
		start int
		end   int
	}

	ranges := make([]lineRange, 0, lineCount)
	lineStart := 0
	for i := 0; i < len(content) && len(ranges) < lineCount; i++ {
		if content[i] == '\n' {
			ranges = append(ranges, lineRange{start: lineStart, end: i + 1})
			lineStart = i + 1
		}
	}
	if len(ranges) < lineCount {
		ranges = append(ranges, lineRange{start: lineStart, end: len(content)})
	}

	rangeStart := ranges[startLine-1].start
	rangeEnd := ranges[endLine-1].end
	contentRange := content[rangeStart:rangeEnd]

	result := &rawReadResult{
		Content: contentRange,
	}
	if startLine != 1 || endLine != lineCount || includeLines {
		result.StartLine = startLine
		result.EndLine = endLine
	}
	if includeLines {
		lines := make([]ReadLine, 0, endLine-startLine+1)
		for n := startLine; n <= endLine; n++ {
			seg := content[ranges[n-1].start:ranges[n-1].end]
			seg = strings.TrimSuffix(seg, "\n")
			lines = append(lines, ReadLine{Num: n, Text: seg})
		}
		result.Lines = lines
	}
	return result, nil
}

func splitFrontmatterBody(content string) (frontmatter, body string) {
	lines := strings.Split(content, "\n")
	_, endLine, ok := parser.FrontmatterBounds(lines)
	if !ok || endLine == -1 {
		return "", content
	}

	frontmatter = strings.Join(lines[:endLine+1], "\n") + "\n"
	if endLine+1 < len(lines) {
		body = strings.Join(lines[endLine+1:], "\n")
	}
	return frontmatter, body
}

func collectReadReferences(body string, rt *Runtime, resolveOp *resolveOperation) []ReadReference {
	lines := strings.Split(body, "\n")
	refs := make([]ReadReference, 0)
	fence := parser.FenceState{}

	for _, line := range lines {
		if fence.UpdateFenceState(line) {
			continue
		}
		if fence.InFence {
			continue
		}

		sanitized := parser.RemoveInlineCode(line)
		matches := wikilink.FindAllInLine(sanitized, false)
		if len(matches) == 0 {
			continue
		}

		for _, match := range matches {
			ref := ReadReference{Text: match.Target}
			if relPath, ok := resolveReferenceTargetToRelPath(match.Target, rt, resolveOp); ok {
				ref.Path = &relPath
			}
			refs = append(refs, ref)
		}
	}

	return refs
}

func resolveReferenceTargetToRelPath(target string, rt *Runtime, resolveOp *resolveOperation) (string, bool) {
	var (
		resolved *ResolveResult
		err      error
	)
	if resolveOp != nil {
		resolved, err = resolveOp.resolveReference(target, false)
	} else {
		resolved, err = ResolveReference(target, rt, false)
	}
	if err != nil {
		return "", false
	}
	relPath, err := filepath.Rel(rt.VaultPath, resolved.FilePath)
	if err != nil {
		return "", false
	}
	return relPath, true
}

func readBacklinksWithContext(rt *Runtime, targetObjectID string) ([]ReadBacklinkGroup, int, error) {
	if err := ensureReadDB(rt); err != nil {
		return nil, 0, err
	}

	links, err := Backlinks(rt, targetObjectID)
	if err != nil {
		return nil, 0, err
	}

	grouped := make(map[string][]model.Reference)
	order := make([]string, 0)
	for _, link := range links {
		if _, exists := grouped[link.FilePath]; !exists {
			order = append(order, link.FilePath)
		}
		grouped[link.FilePath] = append(grouped[link.FilePath], link)
	}

	fileCache := make(map[string][]string)
	out := make([]ReadBacklinkGroup, 0, len(order))
	for _, filePath := range order {
		lines, ok := fileCache[filePath]
		if !ok {
			fullPath := filepath.Join(rt.VaultPath, filePath)
			content, readErr := os.ReadFile(fullPath)
			if readErr != nil {
				out = append(out, ReadBacklinkGroup{
					Source: filePath,
					Lines:  []string{fmt.Sprintf("(failed to read: %v)", readErr)},
				})
				continue
			}
			lines = strings.Split(string(content), "\n")
			fileCache[filePath] = lines
		}

		contextLines := make([]string, 0, len(grouped[filePath]))
		for _, ref := range grouped[filePath] {
			if ref.Line == nil || *ref.Line <= 0 {
				contextLines = append(contextLines, "(frontmatter)")
				continue
			}
			index := *ref.Line - 1
			if index < 0 || index >= len(lines) {
				contextLines = append(contextLines, fmt.Sprintf("(line %d out of range)", *ref.Line))
				continue
			}
			contextLines = append(contextLines, strings.TrimRight(lines[index], "\r"))
		}

		out = append(out, ReadBacklinkGroup{
			Source: filePath,
			Lines:  dedupeStringsPreserveOrder(contextLines),
		})
	}

	return out, len(links), nil
}

func dedupeStringsPreserveOrder(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func ensureReadDB(rt *Runtime) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	if rt.DB != nil {
		return nil
	}

	db, err := index.Open(rt.VaultPath)
	if err != nil {
		return err
	}
	db.SetDailyDirectory(rt.VaultCfg.GetDailyDirectory())
	rt.DB = db
	return nil
}
