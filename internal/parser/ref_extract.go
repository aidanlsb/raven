package parser

import (
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/wikilink"
)

// RefExtractOptions controls how refs are extracted from a FieldValue.
type RefExtractOptions struct {
	// AllowBareStrings treats plain strings as ref targets.
	AllowBareStrings bool
	// AllowWikilinksInString scans string values for wikilink references.
	AllowWikilinksInString bool
	// AllowTripleBrackets passes allowTriple=true to the wikilink parser.
	AllowTripleBrackets bool
}

// ExtractedRef represents a resolved ref target and optional display text.
type ExtractedRef struct {
	TargetRaw   string
	DisplayText *string
}

// ExtractRefsFromFieldValue extracts refs from a FieldValue using the provided options.
func ExtractRefsFromFieldValue(fv schema.FieldValue, opts RefExtractOptions) []ExtractedRef {
	var refs []ExtractedRef

	if target, ok := fv.AsRef(); ok {
		return []ExtractedRef{{TargetRaw: target}}
	}

	if arr, ok := fv.AsArray(); ok {
		for _, item := range arr {
			refs = append(refs, ExtractRefsFromFieldValue(item, opts)...)
		}
		return refs
	}

	if s, ok := fv.AsString(); ok {
		if opts.AllowWikilinksInString {
			matches := wikilink.FindAllInLine(s, opts.AllowTripleBrackets)
			for _, match := range matches {
				refs = append(refs, ExtractedRef{
					TargetRaw:   match.Target,
					DisplayText: match.DisplayText,
				})
			}
		}
		if opts.AllowBareStrings && s != "" && !strings.Contains(s, "[[") {
			refs = append(refs, ExtractedRef{TargetRaw: s})
		}
	}

	return refs
}
