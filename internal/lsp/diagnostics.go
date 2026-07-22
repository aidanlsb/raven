package lsp

import (
	"regexp"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/parser"
)

// refIssueTypes are issue types whose Value field holds the raw ref target,
// letting the diagnostic range narrow to the wikilink span.
var refIssueTypes = map[check.IssueType]struct{}{
	check.IssueMissingReference:        {},
	check.IssueAmbiguousReference:      {},
	check.IssueStaleFragment:           {},
	check.IssueLocalFragmentRef:        {},
	check.IssueShortRefCouldBeFullPath: {},
	check.IssueWrongTargetType:         {},
	check.IssueMissingAsset:            {},
	check.IssueNonCanonicalRef:         {},
}

// traitIssueTypes are issue types attached to a trait annotation.
var traitIssueTypes = map[check.IssueType]struct{}{
	check.IssueUndefinedTrait:    {},
	check.IssueInvalidTraitValue: {},
	check.IssueInvalidDateFormat: {},
	check.IssueInvalidEnumValue:  {},
}

// publishDiagnostics computes and publishes diagnostics for one open document.
// The computation runs under the state lock because debounce timers invoke it
// off the main dispatch loop.
func (s *Server) publishDiagnostics(uri string) {
	s.mu.Lock()
	ws := s.ws
	doc, open := s.docs[uri]
	if ws == nil || !open {
		s.mu.Unlock()
		return
	}
	absPath := uriToPath(uri)
	if absPath == "" || ws.relativePath(absPath) == "" {
		s.mu.Unlock()
		return
	}
	s.ensureWorkspaceCachesFreshLocked(ws)
	validator := ws.newValidator()
	diagnostics := computeDiagnostics(ws, validator, doc.content, absPath, s.encoding)
	s.mu.Unlock()

	s.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	})
}

// computeDiagnostics parses buffer content and validates it against the schema
// and the cached index state. Never returns nil (an empty list clears
// previously published diagnostics).
func computeDiagnostics(ws *workspace, validator *check.Validator, content, absPath, encoding string) []Diagnostic {
	diagnostics := []Diagnostic{}
	lines := documentLines(content)

	doc, issues, err := validateBuffer(ws, validator, content, absPath)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Range:    wholeLineRange(lineAt(lines, 0), 0, encoding),
			Severity: severityError,
			Code:     string(check.IssueParseError),
			Source:   "raven",
			Message:  err.Error(),
		})
		return diagnostics
	}

	for _, item := range diagnosticsForIssues(issues, doc, lines, encoding) {
		diagnostics = append(diagnostics, item.diagnostic)
	}

	return diagnostics
}

func validateBuffer(ws *workspace, validator *check.Validator, content, absPath string) (*parser.ParsedDocument, []check.Issue, error) {
	doc, err := ws.parseBuffer(content, absPath)
	if err != nil {
		return nil, nil, err
	}

	issues := validator.ValidateDocument(doc)
	vaultCfg := ws.vaultConfig()
	if vaultCfg.HasDirectoriesConfig() {
		issues = append(issues, check.DetectNonCanonicalRefs(
			doc,
			vaultCfg.GetObjectsRoot(),
			vaultCfg.GetPagesRoot(),
		)...)
	}
	return doc, issues, nil
}

type issueDiagnostic struct {
	issue      check.Issue
	diagnostic Diagnostic
}

func diagnosticsForIssues(issues []check.Issue, doc *parser.ParsedDocument, lines []string, encoding string) []issueDiagnostic {
	items := make([]issueDiagnostic, 0, len(issues))
	ranges := issueRangeResolver{refOccurrences: make(map[refIssueKey]int)}
	for _, issue := range issues {
		severity := severityError
		if issue.Level == check.LevelWarning {
			severity = severityWarning
		}
		items = append(items, issueDiagnostic{
			issue: issue,
			diagnostic: Diagnostic{
				Range:    ranges.issueRange(issue, doc, lines, encoding),
				Severity: severity,
				Code:     string(issue.Type),
				Source:   "raven",
				Message:  issue.Message,
			},
		})
	}
	return items
}

type refIssueKey struct {
	issueType check.IssueType
	line      int
	value     string
}

type issueRangeResolver struct {
	refOccurrences map[refIssueKey]int
}

// issueRange finds the most precise range for an issue: the wikilink span for
// reference issues, the annotation span for trait issues, and the whole line
// otherwise.
func (r *issueRangeResolver) issueRange(issue check.Issue, doc *parser.ParsedDocument, lines []string, encoding string) Range {
	lineIdx := issue.Line - 1
	if lineIdx < 0 {
		lineIdx = 0
	}
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
		if lineIdx < 0 {
			lineIdx = 0
		}
	}
	line := lineAt(lines, lineIdx)

	if _, ok := refIssueTypes[issue.Type]; ok && issue.Value != "" {
		key := refIssueKey{issueType: issue.Type, line: lineIdx, value: issue.Value}
		occurrence := r.refOccurrences[key]
		r.refOccurrences[key] = occurrence + 1
		if start, end, found := refSpanOnLine(line, issue.Value, occurrence); found {
			return byteRangeToRange(line, lineIdx, start, end, encoding)
		}
	}

	if _, ok := traitIssueTypes[issue.Type]; ok {
		if start, end, found := traitSpanOnLine(line, doc, issue); found {
			return byteRangeToRange(line, lineIdx, start, end, encoding)
		}
	}

	return wholeLineRange(line, lineIdx, encoding)
}

// refSpanOnLine finds the byte span of the wikilink whose target matches value.
func refSpanOnLine(line, value string, occurrence int) (start, end int, found bool) {
	var matches []parser.Reference
	for _, ref := range parser.ExtractRefs(line, 1) {
		if ref.TargetRaw == value {
			matches = append(matches, ref)
		}
	}
	if len(matches) == 0 {
		return 0, 0, false
	}
	ref := matches[occurrence%len(matches)]
	return ref.Start, ref.End, true
}

// traitNamePattern extracts trait names from raw line text for range mapping.
// Parser trait offsets are relative to goldmark-collected segment text (which
// excludes block markup like list markers), so raw buffer lines are re-scanned.
var traitNamePattern = regexp.MustCompile(`@([\w-]+)(?:\(([^)]*)\))?`)

// traitSpanOnLine finds the byte span of the trait annotation an issue refers to.
func traitSpanOnLine(line string, doc *parser.ParsedDocument, issue check.Issue) (start, end int, found bool) {
	// Collect trait names parsed on this line so annotations inside inline
	// code (which the parser skips) are not matched.
	namesOnLine := map[string]struct{}{}
	for _, trait := range doc.Traits {
		if trait.Line == issue.Line {
			namesOnLine[trait.TraitType] = struct{}{}
		}
	}

	var fallback []int
	for _, m := range traitNamePattern.FindAllStringSubmatchIndex(line, -1) {
		name := line[m[2]:m[3]]
		if _, parsed := namesOnLine[name]; !parsed {
			continue
		}
		// For undefined_trait issues, Value holds the trait name; prefer the
		// matching annotation when there are several on the line.
		if issue.Type == check.IssueUndefinedTrait && issue.Value != "" && name != issue.Value {
			if fallback == nil {
				fallback = []int{m[0], m[1]}
			}
			continue
		}
		return m[0], m[1], true
	}
	if fallback != nil {
		return fallback[0], fallback[1], true
	}
	return 0, 0, false
}
