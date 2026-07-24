package lsp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

// maxCompletionItems caps result size; larger sets return isIncomplete so the
// client re-queries as the user types.
const maxCompletionItems = 250

func (s *Server) handleCompletion(raw json.RawMessage) (interface{}, *ResponseError) {
	var params TextDocumentPositionParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &ResponseError{Code: codeInvalidParams, Message: err.Error()}
	}

	ws, doc, ok := s.snapshot(params.TextDocument.URI)
	if !ok {
		return CompletionList{Items: []CompletionItem{}}, nil
	}

	s.mu.Lock()
	encoding := s.encoding
	s.mu.Unlock()

	lines := documentLines(doc.content)
	line := lineAt(lines, params.Position.Line)
	byteCol := characterToByte(line, params.Position.Character, encoding)
	prefix := line[:byteCol]

	if start, refPrefix, ok := refCompletionContext(prefix); ok {
		return refCompletions(ws, line, params.Position.Line, start, byteCol, refPrefix, encoding), nil
	}

	if start, traitPrefix, ok := traitCompletionContext(prefix); ok {
		return traitCompletions(ws, line, params.Position.Line, start, byteCol, traitPrefix, encoding), nil
	}

	if start, keyPrefix, ok := frontmatterKeyContext(lines, params.Position.Line, prefix); ok {
		return frontmatterCompletions(ws, lines, line, params.Position.Line, start, byteCol, keyPrefix, encoding), nil
	}

	return CompletionList{Items: []CompletionItem{}}, nil
}

// refCompletionContext detects a cursor inside an unclosed [[wikilink target.
// Returns the byte offset where the target text begins and the typed prefix.
func refCompletionContext(prefix string) (start int, typed string, ok bool) {
	open := strings.LastIndex(prefix, "[[")
	if open < 0 {
		return 0, "", false
	}
	rest := prefix[open+2:]
	if strings.Contains(rest, "]]") {
		return 0, "", false
	}
	// Inside the display-text part of [[target|display]] there is nothing to complete.
	if strings.Contains(rest, "|") {
		return 0, "", false
	}
	return open + 2, rest, true
}

// traitCompletionPattern matches an @trait name being typed at a valid trait
// position (start of line or after a delimiter), anchored to the prefix end.
var traitCompletionPattern = regexp.MustCompile(`(?:^|[\s\-\*\(\[\{>])@([\w-]*)$`)

// traitCompletionContext detects a cursor typing an @trait name.
func traitCompletionContext(prefix string) (start int, typed string, ok bool) {
	m := traitCompletionPattern.FindStringSubmatchIndex(prefix)
	if m == nil {
		return 0, "", false
	}
	return m[2], prefix[m[2]:m[3]], true
}

// frontmatterKeyContext detects a cursor typing a top-level frontmatter key.
func frontmatterKeyContext(lines []string, lineIdx int, prefix string) (start int, typed string, ok bool) {
	if !inFrontmatter(lines, lineIdx) {
		return 0, "", false
	}
	// Key position: nothing but a bare identifier before the cursor.
	trimmed := strings.TrimRight(prefix, " \t")
	if trimmed != prefix && trimmed != "" {
		return 0, "", false
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]*$`).MatchString(prefix) {
		return 0, "", false
	}
	return 0, prefix, true
}

// inFrontmatter reports whether a 0-indexed line sits inside the YAML
// frontmatter block (strictly between the opening and closing delimiters).
func inFrontmatter(lines []string, lineIdx int) bool {
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return false
	}
	if lineIdx <= 0 {
		return false
	}
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimRight(lines[i], " \t")
		if trimmed == "---" || trimmed == "..." {
			return lineIdx < i
		}
	}
	return true // unclosed frontmatter: everything below the opening --- counts
}

func refCompletions(ws *workspace, line string, lineIdx, startByte, endByte int, typed string, encoding string) CompletionList {
	editRange := byteRangeToRange(line, lineIdx, startByte, endByte, encoding)

	type candidate struct {
		label  string
		detail string
		kind   int
	}
	var candidates []candidate
	for _, obj := range ws.catalog.Objects {
		candidates = append(candidates, candidate{label: obj.ID, detail: obj.Type, kind: completionKindFile})
	}

	aliasNames := make([]string, 0, len(ws.catalog.Aliases))
	for alias := range ws.catalog.Aliases {
		aliasNames = append(aliasNames, alias)
	}
	sort.Strings(aliasNames)
	for _, alias := range aliasNames {
		candidates = append(candidates, candidate{
			label:  alias,
			detail: fmt.Sprintf("alias → %s", ws.catalog.Aliases[alias]),
			kind:   completionKindReference,
		})
	}

	needle := strings.ToLower(typed)
	items := []CompletionItem{}
	incomplete := false
	for _, c := range candidates {
		if needle != "" && !strings.Contains(strings.ToLower(c.label), needle) {
			continue
		}
		if len(items) >= maxCompletionItems {
			incomplete = true
			break
		}
		items = append(items, CompletionItem{
			Label:      c.label,
			Kind:       c.kind,
			Detail:     c.detail,
			FilterText: c.label,
			TextEdit:   &TextEdit{Range: editRange, NewText: c.label},
		})
	}

	return CompletionList{IsIncomplete: incomplete, Items: items}
}

func traitCompletions(ws *workspace, line string, lineIdx, startByte, endByte int, typed string, encoding string) CompletionList {
	editRange := byteRangeToRange(line, lineIdx, startByte, endByte, encoding)

	names := make([]string, 0, len(ws.schema().Traits))
	for name := range ws.schema().Traits {
		names = append(names, name)
	}
	sort.Strings(names)

	needle := strings.ToLower(typed)
	items := []CompletionItem{}
	for _, name := range names {
		if needle != "" && !strings.Contains(strings.ToLower(name), needle) {
			continue
		}
		items = append(items, CompletionItem{
			Label:    name,
			Kind:     completionKindKeyword,
			Detail:   traitDetail(ws.schema().Traits[name]),
			TextEdit: &TextEdit{Range: editRange, NewText: name},
		})
	}
	return CompletionList{Items: items}
}

func traitDetail(def *schema.TraitDefinition) string {
	if def == nil {
		return ""
	}
	if def.IsBoolean() {
		return "boolean trait"
	}
	if def.Type == schema.FieldTypeEnum && len(def.Values) > 0 {
		return fmt.Sprintf("enum: %s", strings.Join(def.Values, ", "))
	}
	return string(def.Type)
}

func frontmatterCompletions(ws *workspace, lines []string, line string, lineIdx, startByte, endByte int, typed string, encoding string) CompletionList {
	editRange := byteRangeToRange(line, lineIdx, startByte, endByte, encoding)

	type key struct {
		name   string
		detail string
	}
	keys := []key{
		{name: "type", detail: "object type"},
		{name: "alias", detail: "reference alias"},
	}

	if typeDef := frontmatterTypeDef(ws, lines); typeDef != nil {
		fieldNames := make([]string, 0, len(typeDef.Fields))
		for name := range typeDef.Fields {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)
		for _, name := range fieldNames {
			keys = append(keys, key{name: name, detail: fieldDetail(typeDef.Fields[name])})
		}
	}

	needle := strings.ToLower(typed)
	items := []CompletionItem{}
	for _, k := range keys {
		if needle != "" && !strings.Contains(strings.ToLower(k.name), needle) {
			continue
		}
		items = append(items, CompletionItem{
			Label:    k.name,
			Kind:     completionKindField,
			Detail:   k.detail,
			TextEdit: &TextEdit{Range: editRange, NewText: k.name + ": "},
		})
	}
	return CompletionList{Items: items}
}

// frontmatterTypeDef finds the schema definition for the buffer's declared type.
func frontmatterTypeDef(ws *workspace, lines []string) *schema.TypeDefinition {
	typeName := "page"
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimRight(lines[i], " \t")
		if trimmed == "---" || trimmed == "..." {
			break
		}
		if value, ok := strings.CutPrefix(trimmed, "type:"); ok {
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"'`)
			if value != "" {
				typeName = value
			}
			break
		}
	}
	return ws.schema().Types[typeName]
}

func fieldDetail(def *schema.FieldDefinition) string {
	if def == nil {
		return ""
	}
	detail := string(def.Type)
	if def.Type == schema.FieldTypeEnum && len(def.Values) > 0 {
		detail = fmt.Sprintf("enum: %s", strings.Join(def.Values, ", "))
	}
	if def.Type == schema.FieldTypeRef && def.Target != "" {
		detail = fmt.Sprintf("ref → %s", def.Target)
	}
	if def.Required {
		detail += " (required)"
	}
	return detail
}
