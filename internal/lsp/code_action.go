package lsp

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/parser"
)

func (s *Server) handleCodeAction(raw json.RawMessage) (interface{}, *ResponseError) {
	var params CodeActionParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &ResponseError{Code: codeInvalidParams, Message: err.Error()}
	}

	actions := []CodeAction{}
	if !allowsQuickFix(params.Context.Only) {
		return actions, nil
	}

	ws, doc, ok := s.snapshot(params.TextDocument.URI)
	if !ok {
		return actions, nil
	}
	absPath := uriToPath(doc.uri)
	if absPath == "" || ws.relativePath(absPath) == "" {
		return actions, nil
	}

	s.mu.Lock()
	encoding := s.encoding
	s.mu.Unlock()

	parsed, issues, err := validateBuffer(ws, ws.newValidator(), doc.content, absPath)
	if err != nil {
		return actions, nil
	}

	lines := documentLines(doc.content)
	for _, item := range diagnosticsForIssues(issues, parsed, lines, encoding) {
		if !rangesOverlap(params.Range, item.diagnostic.Range) {
			continue
		}
		action, ok := quickFixForIssue(params.TextDocument.URI, item, lines, encoding)
		if ok {
			actions = append(actions, action)
		}
	}
	return actions, nil
}

func allowsQuickFix(only []string) bool {
	if len(only) == 0 {
		return true
	}
	for _, kind := range only {
		if codeActionKindQuickFix == kind || strings.HasPrefix(codeActionKindQuickFix, kind+".") {
			return true
		}
	}
	return false
}

func quickFixForIssue(uri string, item issueDiagnostic, lines []string, encoding string) (CodeAction, bool) {
	issue := item.issue
	if issue.FixReplacement == "" || issue.FixReplacement == issue.Value {
		return CodeAction{}, false
	}

	var title string
	switch issue.Type {
	case check.IssueShortRefCouldBeFullPath:
		title = fmt.Sprintf("Expand short ref to [[%s]]", issue.FixReplacement)
	case check.IssueNonCanonicalRef:
		title = fmt.Sprintf("Rewrite ref as [[%s]]", issue.FixReplacement)
	default:
		return CodeAction{}, false
	}

	editRange, ok := wikilinkTargetRange(issue, item.diagnostic.Range, lines, encoding)
	if !ok {
		return CodeAction{}, false
	}
	return CodeAction{
		Title:       title,
		Kind:        codeActionKindQuickFix,
		Diagnostics: []Diagnostic{item.diagnostic},
		IsPreferred: true,
		Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{
			uri: {{
				Range:   editRange,
				NewText: issue.FixReplacement,
			}},
		}},
	}, true
}

func wikilinkTargetRange(issue check.Issue, diagnosticRange Range, lines []string, encoding string) (Range, bool) {
	lineIdx := issue.Line - 1
	line := lineAt(lines, lineIdx)
	if lineIdx < 0 || line == "" {
		return Range{}, false
	}

	for _, ref := range parser.ExtractRefs(line, 1) {
		if ref.TargetRaw != issue.Value {
			continue
		}
		fullRange := byteRangeToRange(line, lineIdx, ref.Start, ref.End, encoding)
		if fullRange != diagnosticRange {
			continue
		}
		start, end, ok := wikilinkTargetByteSpan(line, ref)
		if !ok {
			return Range{}, false
		}
		return byteRangeToRange(line, lineIdx, start, end, encoding), true
	}
	return Range{}, false
}

func wikilinkTargetByteSpan(line string, ref parser.Reference) (start, end int, ok bool) {
	innerStart := ref.Start + len("[[")
	innerEnd := ref.End - len("]]")
	if innerStart < 0 || innerEnd < innerStart || innerEnd > len(line) {
		return 0, 0, false
	}

	targetSource := line[innerStart:innerEnd]
	if pipe := strings.IndexByte(targetSource, '|'); pipe >= 0 {
		targetSource = targetSource[:pipe]
	}
	withoutLeadingSpace := strings.TrimLeftFunc(targetSource, unicode.IsSpace)
	leadingBytes := len(targetSource) - len(withoutLeadingSpace)
	if strings.TrimSpace(withoutLeadingSpace) != ref.TargetRaw {
		return 0, 0, false
	}

	start = innerStart + leadingBytes
	end = start + len(ref.TargetRaw)
	if end > innerEnd {
		return 0, 0, false
	}
	return start, end, true
}

func rangesOverlap(a, b Range) bool {
	aEmpty := a.Start == a.End
	bEmpty := b.Start == b.End
	switch {
	case aEmpty:
		return comparePosition(b.Start, a.Start) <= 0 && comparePosition(a.Start, b.End) <= 0
	case bEmpty:
		return comparePosition(a.Start, b.Start) <= 0 && comparePosition(b.Start, a.End) <= 0
	default:
		return comparePosition(a.Start, b.End) < 0 && comparePosition(b.Start, a.End) < 0
	}
}

func comparePosition(a, b Position) int {
	if a.Line < b.Line || (a.Line == b.Line && a.Character < b.Character) {
		return -1
	}
	if a == b {
		return 0
	}
	return 1
}
