package lsp

import (
	"encoding/json"
	"os"
)

func (s *Server) handleReferences(raw json.RawMessage) (interface{}, *ResponseError) {
	var params ReferenceParams
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

	targetID := referenceTarget(ws, doc, params.TextDocumentPositionParams, encoding)
	if targetID == "" {
		return nil, nil
	}

	vaultCfg := ws.vaultConfig()
	refs, err := ws.db().BacklinksWithRoots(targetID, vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot())
	if err != nil {
		return nil, &ResponseError{Code: codeInternalError, Message: err.Error()}
	}

	locations := []Location{}
	if params.Context.IncludeDeclaration {
		if loc, found := locationForID(ws, targetID); found {
			locations = append(locations, loc)
		}
	}

	// Cache file contents so column offsets can be converted per line.
	fileLines := map[string][]string{}
	for _, ref := range refs {
		if ref.Line == nil {
			continue
		}
		lineIdx := *ref.Line - 1
		if lineIdx < 0 {
			continue
		}

		lines, ok := fileLines[ref.FilePath]
		if !ok {
			lines = readFileLines(ws, ref.FilePath)
			fileLines[ref.FilePath] = lines
		}
		line := lineAt(lines, lineIdx)

		rng := Range{Start: Position{Line: lineIdx}, End: Position{Line: lineIdx}}
		if ref.PositionStart != nil && ref.PositionEnd != nil && *ref.PositionEnd > *ref.PositionStart && line != "" {
			rng = byteRangeToRange(line, lineIdx, *ref.PositionStart, *ref.PositionEnd, encoding)
		} else if line != "" {
			rng = wholeLineRange(line, lineIdx, encoding)
		}

		locations = append(locations, Location{
			URI:   pathToURI(ws.absolutePath(ref.FilePath)),
			Range: rng,
		})
	}

	return locations, nil
}

// referenceTarget picks the ID to find references for: the wikilink under the
// cursor when there is an unambiguous one, otherwise the current file's object.
func referenceTarget(ws *workspace, doc document, params TextDocumentPositionParams, encoding string) string {
	lines := documentLines(doc.content)
	if ref, ok := refAtPosition(lines, params.Position, encoding); ok {
		targets := resolveTargets(ws, ref.target)
		if len(targets) == 1 {
			return targets[0]
		}
		return ""
	}

	absPath := uriToPath(params.TextDocument.URI)
	if absPath == "" || ws.relativePath(absPath) == "" {
		return ""
	}
	parsed, err := ws.parseBuffer(doc.content, absPath)
	if err != nil || len(parsed.Objects) == 0 {
		return ""
	}
	return parsed.Objects[0].ID
}

func readFileLines(ws *workspace, relPath string) []string {
	content, err := os.ReadFile(ws.absolutePath(relPath))
	if err != nil {
		return nil
	}
	return documentLines(string(content))
}
