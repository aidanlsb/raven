package readsvc

import (
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/refresolve"
)

const (
	LinkStatusResolved   = "resolved"
	LinkStatusUnresolved = "unresolved"
	LinkStatusAmbiguous  = "ambiguous"
)

// ReadLink is a wikilink located in the returned read body. Offsets are
// 0-based and end-exclusive into the same `content` string:
//
//   - byte_start / byte_end are UTF-8 byte indexes (unambiguous for slicing
//     the Go/JSON UTF-8 string)
//   - utf16_start / utf16_end are UTF-16 code-unit indexes (JavaScript
//     string indexes, including surrogate pairs)
type ReadLink struct {
	ByteStart  int      `json:"byte_start"`
	ByteEnd    int      `json:"byte_end"`
	UTF16Start int      `json:"utf16_start"`
	UTF16End   int      `json:"utf16_end"`
	Text       string   `json:"text"`
	Target     string   `json:"target"`
	Status     string   `json:"status"`
	ObjectID   string   `json:"object_id,omitempty"`
	Matches    []string `json:"matches,omitempty"`
}

func collectReadLinks(content string, resolveOp *refresolve.Operation) ([]ReadLink, error) {
	spans, err := parser.ExtractWikilinkSpans(content)
	if err != nil {
		return nil, err
	}

	links := make([]ReadLink, 0, len(spans))
	for _, span := range spans {
		if span.ByteStart < 0 || span.ByteEnd > len(content) || span.ByteStart >= span.ByteEnd {
			continue
		}
		if content[span.ByteStart:span.ByteEnd] != span.Literal {
			continue
		}

		link := ReadLink{
			ByteStart:  span.ByteStart,
			ByteEnd:    span.ByteEnd,
			UTF16Start: utf16Units(content[:span.ByteStart]),
			UTF16End:   utf16Units(content[:span.ByteEnd]),
			Text:       span.Literal,
			Target:     span.Target,
			Status:     LinkStatusUnresolved,
		}
		if resolveOp != nil {
			lookedUp, err := resolveOp.Lookup(span.Target)
			if err != nil {
				return nil, err
			}
			applyLinkResolution(&link, lookedUp)
		}
		links = append(links, link)
	}
	return links, nil
}

func applyLinkResolution(link *ReadLink, lookedUp refresolve.LookupResult) {
	switch {
	case lookedUp.Ambiguous:
		link.Status = LinkStatusAmbiguous
		if len(lookedUp.Matches) > 0 {
			link.Matches = lookedUp.Matches
		}
	case lookedUp.ObjectID != "":
		link.Status = LinkStatusResolved
		link.ObjectID = lookedUp.ObjectID
	default:
		link.Status = LinkStatusUnresolved
	}
}

func utf16Units(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
			continue
		}
		n++
	}
	return n
}
