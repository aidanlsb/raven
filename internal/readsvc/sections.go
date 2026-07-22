package readsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
)

// SectionsResult is the outline of a file's sections.
type SectionsResult struct {
	ObjectID     string
	FileObjectID string
	Path         string
	Sections     []model.Section
}

// ReadSections returns the section outline for a reference. The file is parsed
// directly so the outline is always current, even if the index is stale. When
// the reference is a section, the outline is scoped to that section's subtree.
func ReadSections(rt *Runtime, reference string) (*SectionsResult, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, fmt.Errorf("reference is required")
	}

	resolveOp, err := newResolveOperation(rt)
	if err != nil {
		return nil, err
	}
	defer resolveOp.Close()

	resolved, err := resolveOp.resolveReferenceWithDynamicDates(reference, false)
	if err != nil {
		return nil, err
	}

	contentBytes, err := os.ReadFile(resolved.FilePath)
	if err != nil {
		return nil, err
	}
	relPath, err := filepath.Rel(rt.VaultPath, resolved.FilePath)
	if err != nil {
		return nil, err
	}
	relPath = filepath.ToSlash(relPath)

	doc, err := parser.ParseDocumentWithOptions(string(contentBytes), resolved.FilePath, rt.VaultPath, parser.OptionsFromVaultConfig(rt.VaultCfg))
	if err != nil {
		return nil, err
	}

	// Scope to the requested section's subtree when a fragment was given.
	scopeStart := 0
	scopeEnd := 0
	if resolved.IsSection {
		for _, section := range doc.Sections {
			if section != nil && section.ID == resolved.ObjectID {
				scopeStart = section.LineStart
				if section.SubtreeLineEnd != nil {
					scopeEnd = *section.SubtreeLineEnd
				}
				break
			}
		}
		if scopeStart == 0 {
			return nil, &RefNotFoundError{Reference: reference, Detail: "section not found in file; run 'rvn reindex' if the index is stale"}
		}
	}

	sections := make([]model.Section, 0, len(doc.Sections))
	for _, section := range doc.Sections {
		if section == nil {
			continue
		}
		if scopeStart > 0 {
			if section.LineStart < scopeStart {
				continue
			}
			if scopeEnd > 0 && section.LineStart > scopeEnd {
				continue
			}
		}
		// Parser already stamps FilePath; keep relPath as a fallback for fixtures.
		out := *section
		if out.FilePath == "" {
			out.FilePath = relPath
		}
		sections = append(sections, out)
	}

	return &SectionsResult{
		ObjectID:     resolved.ObjectID,
		FileObjectID: resolved.FileObjectID,
		Path:         relPath,
		Sections:     sections,
	}, nil
}
