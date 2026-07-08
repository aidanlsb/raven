package lsp

import (
	"encoding/json"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/wikilink"
)

// refUnderCursor describes the wikilink at a document position.
type refUnderCursor struct {
	target    string
	lineIdx   int // 0-indexed
	startByte int
	endByte   int
}

// refAtPosition finds the wikilink containing the given position, if any.
func refAtPosition(lines []string, pos Position, encoding string) (refUnderCursor, bool) {
	line := lineAt(lines, pos.Line)
	if line == "" {
		return refUnderCursor{}, false
	}
	byteCol := characterToByte(line, pos.Character, encoding)

	for _, match := range wikilink.FindAllInLine(line, false) {
		if byteCol >= match.Start && byteCol <= match.End {
			return refUnderCursor{
				target:    match.Target,
				lineIdx:   pos.Line,
				startByte: match.Start,
				endByte:   match.End,
			}, true
		}
	}
	return refUnderCursor{}, false
}

// resolveTargets resolves a raw ref target to object/section IDs.
// Ambiguous refs return all candidates.
func resolveTargets(ws *workspace, target string) []string {
	result := ws.resolver.Resolve(target)
	if result.Ambiguous {
		return result.Matches
	}
	if result.TargetID != "" {
		return []string{result.TargetID}
	}

	// Unresolved section ref: the file may exist while the resolver only
	// tracks object IDs. Resolve the base and reattach the fragment.
	if baseRef, fragment, isSection := paths.ParseSectionID(target); isSection && fragment != "" {
		baseResult := ws.resolver.Resolve(baseRef)
		if !baseResult.Ambiguous && baseResult.TargetID != "" {
			return []string{baseResult.TargetID + "#" + fragment}
		}
	}
	return nil
}

// locationForID returns the definition location of an object or section ID.
func locationForID(ws *workspace, id string) (Location, bool) {
	if baseID, fragment, isSection := paths.ParseSectionID(id); isSection && fragment != "" {
		if section, err := ws.db().GetSection(id); err == nil && section != nil {
			line := section.LineStart - 1
			if line < 0 {
				line = 0
			}
			return Location{
				URI:   pathToURI(ws.absolutePath(section.FilePath)),
				Range: Range{Start: Position{Line: line}, End: Position{Line: line}},
			}, true
		}
		// Section not indexed (e.g. renamed heading): fall back to the file.
		id = baseID
	}

	obj, err := ws.db().GetObject(id)
	if err != nil || obj == nil {
		return Location{}, false
	}
	line := obj.LineStart - 1
	if line < 0 {
		line = 0
	}
	return Location{
		URI:   pathToURI(ws.absolutePath(obj.FilePath)),
		Range: Range{Start: Position{Line: line}, End: Position{Line: line}},
	}, true
}

func (s *Server) handleDefinition(raw json.RawMessage) (interface{}, *ResponseError) {
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

	ref, ok := refAtPosition(documentLines(doc.content), params.Position, encoding)
	if !ok {
		return nil, nil
	}

	locations := []Location{}
	for _, id := range resolveTargets(ws, ref.target) {
		if loc, found := locationForID(ws, id); found {
			locations = append(locations, loc)
		}
	}
	if len(locations) == 0 {
		return nil, nil
	}
	return locations, nil
}
