package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
)

const hoverPreviewLines = 8

func (s *Server) handleHover(raw json.RawMessage) (interface{}, *ResponseError) {
	var params TextDocumentPositionParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &ResponseError{Code: codeInvalidParams, Message: err.Error()}
	}

	ws, doc, ok := s.snapshot(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	s.mu.Lock()
	encoding := s.encoding
	s.mu.Unlock()

	lines := documentLines(doc.content)
	ref, ok := refAtPosition(lines, params.Position, encoding)
	if !ok {
		return nil, nil
	}

	targets := resolveTargets(ws, ref.target)
	if len(targets) == 0 {
		return nil, nil
	}

	hoverRange := byteRangeToRange(lineAt(lines, ref.lineIdx), ref.lineIdx, ref.startByte, ref.endByte, encoding)

	if len(targets) > 1 {
		var b strings.Builder
		fmt.Fprintf(&b, "`[[%s]]` is ambiguous:\n\n", ref.target)
		for _, id := range targets {
			fmt.Fprintf(&b, "- `%s`\n", id)
		}
		return Hover{
			Contents: MarkupContent{Kind: "markdown", Value: b.String()},
			Range:    &hoverRange,
		}, nil
	}

	content := hoverContent(ws, targets[0])
	if content == "" {
		return nil, nil
	}
	return Hover{
		Contents: MarkupContent{Kind: "markdown", Value: content},
		Range:    &hoverRange,
	}, nil
}

// hoverContent renders a markdown summary of the target object or section.
func hoverContent(ws *workspace, id string) string {
	objectID := id
	fragment := ""
	if baseID, frag, isSection := paths.ParseSectionID(id); isSection && frag != "" {
		objectID = baseID
		fragment = frag
	}

	obj, err := ws.db().GetObject(objectID)
	if err != nil || obj == nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**%s**", obj.ID)
	if fragment != "" {
		fmt.Fprintf(&b, " `#%s`", fragment)
	}
	fmt.Fprintf(&b, " — `%s`\n", obj.Type)

	if fields := formatHoverFields(obj); fields != "" {
		b.WriteString("\n")
		b.WriteString(fields)
	}

	if preview := filePreview(ws, obj.FilePath); preview != "" {
		b.WriteString("\n---\n\n")
		b.WriteString(preview)
		b.WriteString("\n")
	}

	return b.String()
}

func formatHoverFields(obj *model.Object) string {
	if len(obj.Fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(obj.Fields))
	for key := range obj.Fields {
		if key == "type" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "- %s: `%v`\n", key, obj.Fields[key])
	}
	return b.String()
}

// filePreview returns the first non-empty body lines of a vault file.
func filePreview(ws *workspace, relPath string) string {
	content, err := os.ReadFile(ws.absolutePath(relPath))
	if err != nil {
		return ""
	}

	body := string(content)
	if frontmatter, err := parser.ParseFrontmatter(body); err == nil && frontmatter != nil {
		lines := strings.Split(body, "\n")
		if frontmatter.EndLine < len(lines) {
			body = strings.Join(lines[frontmatter.EndLine:], "\n")
		} else {
			body = ""
		}
	}

	var preview []string
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" && len(preview) == 0 {
			continue
		}
		preview = append(preview, line)
		if len(preview) >= hoverPreviewLines {
			break
		}
	}
	return strings.TrimRight(strings.Join(preview, "\n"), "\n ")
}
