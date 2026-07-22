package parser

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/schema"
)

// PositionedSchemaRef describes a schema-typed frontmatter reference and its
// source span. Line is 1-indexed; Start and End are byte offsets within it.
type PositionedSchemaRef struct {
	FieldName string
	TargetRaw string
	Line      int
	Start     int
	End       int
}

// SchemaFieldRefAtPosition returns the ref/ref[] frontmatter value under a
// source position. The position uses a 0-indexed document line and byte column.
//
// Reference semantics come from ExtractSchemaFieldRefs. The YAML node walk only
// maps those already-extracted references back to their source spans.
func SchemaFieldRefAtPosition(doc *ParsedDocument, sch *schema.Schema, lineIdx, byteCol int) (PositionedSchemaRef, bool) {
	if doc == nil || sch == nil || lineIdx <= 0 {
		return PositionedSchemaRef{}, false
	}

	lines := strings.Split(doc.RawContent, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	_, endLine, hasFrontmatter := FrontmatterBounds(lines)
	if !hasFrontmatter || endLine < 0 || lineIdx >= endLine {
		return PositionedSchemaRef{}, false
	}

	allowed := make(map[string]map[string]struct{})
	for _, ref := range ExtractSchemaFieldRefs(doc.Objects, sch) {
		targets := allowed[ref.FieldName]
		if targets == nil {
			targets = make(map[string]struct{})
			allowed[ref.FieldName] = targets
		}
		targets[ref.TargetRaw] = struct{}{}
	}
	if len(allowed) == 0 {
		return PositionedSchemaRef{}, false
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:endLine], "\n")), &root); err != nil {
		return PositionedSchemaRef{}, false
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return PositionedSchemaRef{}, false
	}

	mapping := root.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		fieldName := mapping.Content[i].Value
		targets := allowed[fieldName]
		if len(targets) == 0 {
			continue
		}
		if ref, ok := schemaRefInYAMLNode(mapping.Content[i+1], fieldName, targets, lines, lineIdx, byteCol); ok {
			return ref, true
		}
	}

	return PositionedSchemaRef{}, false
}

func schemaRefInYAMLNode(node *yaml.Node, fieldName string, allowed map[string]struct{}, lines []string, lineIdx, byteCol int) (PositionedSchemaRef, bool) {
	if node == nil {
		return PositionedSchemaRef{}, false
	}
	if node.Kind == yaml.AliasNode {
		node = node.Alias
		if node == nil {
			return PositionedSchemaRef{}, false
		}
	}

	if node.Kind == yaml.ScalarNode && node.Line == lineIdx {
		start, end, ok := yamlScalarSpan(node, lines)
		if ok && byteCol >= start && byteCol <= end {
			var raw interface{}
			if err := node.Decode(&raw); err == nil {
				refs := ExtractRefsFromFieldValue(
					FieldValueFromYAML(raw),
					RefExtractOptions{AllowBareStrings: true},
				)
				for _, ref := range refs {
					if _, ok := allowed[ref.TargetRaw]; ok {
						return PositionedSchemaRef{
							FieldName: fieldName,
							TargetRaw: ref.TargetRaw,
							Line:      lineIdx + 1,
							Start:     start,
							End:       end,
						}, true
					}
				}
			}
		}
	}

	for _, child := range node.Content {
		if ref, ok := schemaRefInYAMLNode(child, fieldName, allowed, lines, lineIdx, byteCol); ok {
			return ref, true
		}
	}
	return PositionedSchemaRef{}, false
}

func yamlScalarSpan(node *yaml.Node, lines []string) (start, end int, ok bool) {
	// The YAML input omits the opening fence, so its 1-indexed node line is
	// also the corresponding 0-indexed line in the complete document.
	lineIdx := node.Line
	if lineIdx < 0 || lineIdx >= len(lines) {
		return 0, 0, false
	}
	line := lines[lineIdx]
	start = yamlColumnToByte(line, node.Column)
	if start < 0 || start >= len(line) {
		return 0, 0, false
	}

	switch node.Style {
	case yaml.DoubleQuotedStyle:
		return quotedYAMLScalarSpan(line, start, '"')
	case yaml.SingleQuotedStyle:
		return quotedYAMLScalarSpan(line, start, '\'')
	case yaml.LiteralStyle, yaml.FoldedStyle:
		// Block scalars can span multiple source lines and do not represent one
		// precise ref value under the cursor.
		return 0, 0, false
	default:
		end = start + len(node.Value)
		if end > len(line) {
			return 0, 0, false
		}
		return start, end, true
	}
}

// YAML columns count Unicode code points and are 1-indexed.
func yamlColumnToByte(line string, column int) int {
	want := column - 1
	if want <= 0 {
		return 0
	}
	runes := 0
	for byteIdx := range line {
		if runes == want {
			return byteIdx
		}
		runes++
	}
	if runes == want {
		return len(line)
	}
	return -1
}

func quotedYAMLScalarSpan(line string, start int, quote byte) (int, int, bool) {
	if line[start] != quote {
		return 0, 0, false
	}
	for i := start + 1; i < len(line); i++ {
		if line[i] != quote {
			continue
		}
		if quote == '\'' && i+1 < len(line) && line[i+1] == quote {
			i++
			continue
		}
		if quote == '"' && escapedByBackslash(line, i) {
			continue
		}
		return start + 1, i, true
	}
	return 0, 0, false
}

func escapedByBackslash(line string, idx int) bool {
	backslashes := 0
	for i := idx - 1; i >= 0 && line[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}
