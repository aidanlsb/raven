package sectionsvc

import (
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/refs"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type fileRewrite struct {
	path           string
	content        []byte
	perm           os.FileMode
	reportSourceID string
	updatedContent []byte
}

type inboundRewritePlan struct {
	sourceContent string
	files         []*fileRewrite
	updatedRefs   []string
	warnings      []string
}

func planInboundSectionRewrites(
	rt *vaultruntime.Runtime,
	db *index.Database,
	fileID, oldSectionID, oldSlug, newSlug, sourceContent string,
) inboundRewritePlan {
	plan := inboundRewritePlan{sourceContent: sourceContent}
	if db == nil || newSlug == oldSlug {
		return plan
	}

	rewritesByPath := make(map[string]*fileRewrite)
	updatedRefSeen := make(map[string]struct{})
	addUpdatedRef := func(ref string) {
		if strings.TrimSpace(ref) == "" {
			return
		}
		if _, seen := updatedRefSeen[ref]; seen {
			return
		}
		updatedRefSeen[ref] = struct{}{}
		plan.updatedRefs = append(plan.updatedRefs, ref)
	}

	backlinks, err := db.BacklinksWithRoots(oldSectionID, rt.VaultCfg.GetObjectsRoot(), rt.VaultCfg.GetPagesRoot())
	if err != nil {
		plan.warnings = append(plan.warnings, fmt.Sprintf("Failed to read backlinks for section rename: %v", err))
		return plan
	}

	for _, bl := range backlinks {
		oldRaw := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(bl.TargetRaw), "]]"), "[[")
		base, _, targetIsSection := paths.ParseSectionID(oldRaw)
		if !targetIsSection || base == "" {
			continue
		}
		newRaw := base + "#" + newSlug

		line := 0
		if bl.Line != nil {
			line = *bl.Line
		}

		sourceFileID := bl.SourceID
		if idx := strings.Index(sourceFileID, "#"); idx >= 0 {
			sourceFileID = sourceFileID[:idx]
		}
		if idx := strings.Index(sourceFileID, ":trait:"); idx >= 0 {
			sourceFileID = sourceFileID[:idx]
		}

		if sourceFileID == fileID {
			plan.sourceContent = rewriteSectionRefAtLine(plan.sourceContent, line, oldRaw, newRaw)
			addUpdatedRef(fileID)
			continue
		}

		refFilePath, err := vault.ResolveObjectToFileWithConfig(rt.VaultPath, sourceFileID, rt.VaultCfg)
		if err != nil {
			plan.warnings = append(plan.warnings, fmt.Sprintf("Failed to update refs in %s: %v", sourceFileID, err))
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, refFilePath); err != nil {
			plan.warnings = append(plan.warnings, fmt.Sprintf("Skipped ref update in %s: %v", sourceFileID, err))
			continue
		}

		rewrite, exists := rewritesByPath[refFilePath]
		if !exists {
			rewrite, err = readFileRewrite(refFilePath, sourceFileID)
			if err != nil {
				plan.warnings = append(plan.warnings, fmt.Sprintf("Failed to read %s for ref update: %v", sourceFileID, err))
				continue
			}
			rewritesByPath[refFilePath] = rewrite
			plan.files = append(plan.files, rewrite)
		}
		updated := rewriteSectionRefAtLine(string(rewrite.updatedContent), line, oldRaw, newRaw)
		if updated != string(rewrite.updatedContent) {
			rewrite.updatedContent = []byte(updated)
			addUpdatedRef(sourceFileID)
		}
	}

	return plan
}

func readFileRewrite(path, reportSourceID string) (*fileRewrite, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	perm := os.FileMode(0)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode()
	}
	return &fileRewrite{
		path:           path,
		content:        content,
		perm:           perm,
		reportSourceID: reportSourceID,
		updatedContent: append([]byte(nil), content...),
	}, nil
}

// rewriteSectionRefAtLine rewrites an exact section target, preserving the
// authored object base (including aliases) while replacing only its fragment.
func rewriteSectionRefAtLine(content string, line int, oldRaw, newRaw string) string {
	oldBase, oldFragment, oldIsSection := paths.ParseSectionID(strings.TrimSpace(oldRaw))
	if !oldIsSection || oldBase == "" || oldFragment == "" || oldRaw == newRaw {
		return content
	}

	decide := func(occ refs.Occurrence) (string, bool) {
		if occ.Base == oldBase && occ.HasFragment && occ.Fragment == oldFragment {
			return newRaw, true
		}
		return "", false
	}
	updated, _ := refs.RewriteContentAtLine(content, line, decide)
	return rewriteMarkdownSectionRefAtLine(updated, line, oldRaw, newRaw)
}

// rewriteMarkdownSectionRefAtLine preserves section rename's compatibility
// with direct Markdown links that share an indexed backlink line. Direct
// Markdown links are deliberately not Raven references, so refs.RewriteContent
// does not rewrite them. Limiting this fallback to the indexed line also keeps
// unindexed examples in fenced code blocks untouched.
func rewriteMarkdownSectionRefAtLine(content string, line int, oldRaw, newRaw string) string {
	if line <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return content
	}
	lines[idx] = strings.NewReplacer(
		"]("+oldRaw+")", "]("+newRaw+")",
		"](<"+oldRaw+">)", "](<"+newRaw+">)",
	).Replace(lines[idx])
	return strings.Join(lines, "\n")
}
