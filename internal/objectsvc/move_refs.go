package objectsvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/refs"
	"github.com/aidanlsb/raven/internal/resolver"
)

// ReplaceAllRefVariants rewrites every reference to the moved object across the
// whole content, covering wikilinks, markdown link/image destinations, and YAML
// frontmatter values.
//
// Matching is structural: a reference is rewritten when its target base equals
// oldID, oldBase, or a root-prefixed form of oldID. Section fragments, display
// text, link titles, and query/anchor suffixes are preserved.
func ReplaceAllRefVariants(content, oldID, oldBase, newRef, objectRoot, pageRoot string) string {
	out, _ := refs.RewriteContent(content, moveRefDecider(oldID, oldBase, newRef, objectRoot, pageRoot))
	return out
}

// ApplyAllRefVariantsAtLine rewrites references to the moved object, preferring
// the given 1-based line and falling back to a whole-content rewrite when that
// line contains no matching reference.
func ApplyAllRefVariantsAtLine(content string, line int, oldID, oldBase, newRef, objectRoot, pageRoot string) string {
	out, _ := refs.RewriteContentAtLine(content, line, moveRefDecider(oldID, oldBase, newRef, objectRoot, pageRoot))
	return out
}

// moveRefDecider builds a refs.Decider that maps any recognized written form of
// the moved object's base target to newRef. The recognized forms mirror the
// variants a reference may take: the canonical object ID, the base as written
// in a specific backlink, and the object ID under the configured objects/pages
// roots.
func moveRefDecider(oldID, oldBase, newRef, objectRoot, pageRoot string) refs.Decider {
	oldSet := make(map[string]struct{}, 4)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p != "" {
			oldSet[p] = struct{}{}
		}
	}

	add(oldID)
	add(oldBase)
	if objectRoot != "" {
		add(objectRoot + oldID)
	}
	if pageRoot != "" && pageRoot != objectRoot {
		add(pageRoot + oldID)
	}

	return func(occ refs.Occurrence) (string, bool) {
		if _, ok := oldSet[occ.Base]; ok {
			return newRef, true
		}
		return "", false
	}
}

// ChooseReplacementRefBase decides which written form of the destination ID to
// use when rewriting a reference, preserving short names and aliases when they
// still resolve unambiguously to the destination.
func ChooseReplacementRefBase(oldBase, sourceID, destID string, aliasSlugToID map[string]string, res *resolver.Resolver) string {
	if strings.Contains(oldBase, "/") {
		return destID
	}

	if aliasSlugToID != nil {
		if aliasSlugToID[pages.SlugifyPath(oldBase)] == sourceID {
			return oldBase
		}
	}

	candidate := paths.ShortNameFromID(destID)
	if candidate != "" && res != nil {
		r := res.Resolve(candidate)
		if !r.Ambiguous && r.TargetID == destID {
			return candidate
		}
	}

	return destID
}
