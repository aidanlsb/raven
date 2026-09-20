package objectsvc

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/linktarget"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/wikilink"
)

func prepareMoveWritePlan(rt *vaultruntime.Runtime, req MoveFileRequest, refPlans []refUpdatePlan, fieldRefPlans []fieldRefUpdatePlan, linkPlans []linkUpdatePlan, sourceSnapshot *fileSnapshot, objectRoot, pageRoot string) (*moveWritePlan, []string, error) {
	plan := &moveWritePlan{}

	destinationContent := sourceSnapshot.content
	if len(req.ReplacementContent) > 0 {
		destinationContent = req.ReplacementContent
		plan.destinationContent = append([]byte(nil), req.ReplacementContent...)
	}

	destCurrent := string(destinationContent)
	rewritesByPath := make(map[string]*fileRewrite)
	var rewriteOrder []*fileRewrite
	updatedRefSeen := make(map[string]struct{})
	updatedRefFieldSeen := make(map[string]struct{})
	var warnings []string

	addUpdatedRef := func(ref string) {
		if strings.TrimSpace(ref) == "" {
			return
		}
		if _, ok := updatedRefSeen[ref]; ok {
			return
		}
		updatedRefSeen[ref] = struct{}{}
		plan.updatedRefs = append(plan.updatedRefs, ref)
	}
	addUpdatedRefField := func(update RefFieldUpdate) {
		key := update.SourceID + "\x00" + update.Field
		if _, ok := updatedRefFieldSeen[key]; ok {
			return
		}
		updatedRefFieldSeen[key] = struct{}{}
		plan.updatedRefFields = append(plan.updatedRefFields, update)
	}

	// Apply position-based link edits from the end of each file toward the
	// beginning so replacement lengths cannot invalidate later indexed spans.
	sort.SliceStable(linkPlans, func(i, j int) bool {
		if linkPlans[i].filePath != linkPlans[j].filePath {
			return linkPlans[i].filePath < linkPlans[j].filePath
		}
		if linkPlans[i].link.Line != linkPlans[j].link.Line {
			return linkPlans[i].link.Line > linkPlans[j].link.Line
		}
		return linkPlans[i].link.PositionStart > linkPlans[j].link.PositionStart
	})
	for _, linkPlan := range linkPlans {
		rewrite, err := planRewriteForLinkSource(rt.VaultPath, rt.VaultCfg, linkPlan)
		if err != nil {
			var svcErr *svcerr.Error
			if errors.As(err, &svcErr) && svcErr.Code == codes.ErrValidationFailed {
				return nil, warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("Failed to update file link in %s: %v", linkPlan.sourceID, err))
			continue
		}

		existing, ok := rewritesByPath[rewrite.path]
		if !ok {
			rewritesByPath[rewrite.path] = rewrite
			rewriteOrder = append(rewriteOrder, rewrite)
			existing = rewrite
		}

		updated, changed := rewriteIndexedLinkTarget(existing.updatedContent, linkPlan.link, linkPlan.replacement)
		if !changed {
			warnings = append(warnings, fmt.Sprintf("Failed to update file link in %s: indexed link no longer matches source", linkPlan.sourceID))
			continue
		}
		existing.updatedContent = updated
		addUpdatedRef(linkPlan.sourceID)
	}

	for _, fieldPlan := range fieldRefPlans {
		if movedDocumentSource(fieldPlan.sourceID, req.DestinationObject) {
			updated := ReplaceRefFieldVariants(
				destCurrent,
				fieldPlan.fieldName,
				req.SourceObjectID,
				fieldPlan.oldBase,
				fieldPlan.replacement,
				objectRoot,
				pageRoot,
			)
			if updated == destCurrent {
				continue
			}
			destCurrent = updated
			plan.destinationContent = []byte(destCurrent)
			addUpdatedRef(fieldPlan.sourceID)
			addUpdatedRefField(RefFieldUpdate{
				SourceID: fieldPlan.sourceID,
				FilePath: fieldPlan.filePath,
				Field:    fieldPlan.fieldName,
			})
			continue
		}

		rewrite, err := planRewriteForSource(rt.VaultPath, rt.VaultCfg, refUpdatePlan{
			sourceID: fieldPlan.sourceID,
		})
		if err != nil {
			var svcErr *svcerr.Error
			if errors.As(err, &svcErr) && svcErr.Code == codes.ErrValidationFailed {
				return nil, warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("Failed to update ref field %s in %s: %v", fieldPlan.fieldName, fieldPlan.sourceID, err))
			continue
		}

		existing, ok := rewritesByPath[rewrite.path]
		if !ok {
			rewritesByPath[rewrite.path] = rewrite
			rewriteOrder = append(rewriteOrder, rewrite)
			existing = rewrite
		}

		updated := ReplaceRefFieldVariants(
			string(existing.updatedContent),
			fieldPlan.fieldName,
			req.SourceObjectID,
			fieldPlan.oldBase,
			fieldPlan.replacement,
			objectRoot,
			pageRoot,
		)
		if updated == string(existing.updatedContent) {
			continue
		}
		existing.updatedContent = []byte(updated)
		addUpdatedRef(fieldPlan.sourceID)
		addUpdatedRefField(RefFieldUpdate{
			SourceID: fieldPlan.sourceID,
			FilePath: fieldPlan.filePath,
			Field:    fieldPlan.fieldName,
		})
	}

	for _, refPlan := range refPlans {
		if movedDocumentSource(refPlan.sourceID, req.DestinationObject) {
			updated := ApplyAllRefVariantsAtLine(destCurrent, refPlan.line, req.SourceObjectID, refPlan.oldBase, refPlan.replacement, objectRoot, pageRoot)
			if updated == destCurrent {
				continue
			}
			destCurrent = updated
			plan.destinationContent = []byte(destCurrent)
			addUpdatedRef(refPlan.sourceID)
			continue
		}

		rewrite, err := planRewriteForSource(rt.VaultPath, rt.VaultCfg, refPlan)
		if err != nil {
			var svcErr *svcerr.Error
			if errors.As(err, &svcErr) && svcErr.Code == codes.ErrValidationFailed {
				return nil, warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("Failed to update refs in %s: %v", refPlan.sourceID, err))
			continue
		}

		existing, ok := rewritesByPath[rewrite.path]
		if !ok {
			rewritesByPath[rewrite.path] = rewrite
			rewriteOrder = append(rewriteOrder, rewrite)
			existing = rewrite
		}

		updated := ApplyAllRefVariantsAtLine(string(existing.updatedContent), refPlan.line, req.SourceObjectID, refPlan.oldBase, refPlan.replacement, objectRoot, pageRoot)
		if updated == string(existing.updatedContent) {
			continue
		}
		existing.updatedContent = []byte(updated)
		addUpdatedRef(refPlan.sourceID)
	}

	for _, rewrite := range rewriteOrder {
		if string(rewrite.updatedContent) == string(rewrite.content) {
			continue
		}
		plan.rewriteFiles = append(plan.rewriteFiles, rewrite)
	}

	return plan, warnings, nil
}

func planRewriteForLinkSource(vaultPath string, vaultCfg *config.VaultConfig, linkPlan linkUpdatePlan) (*fileRewrite, error) {
	filePath := filepath.Join(vaultPath, filepath.FromSlash(linkPlan.filePath))
	if err := mutationguard.ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath); err != nil {
		return nil, err
	}

	snapshot, err := readFileSnapshot(filePath)
	if err != nil {
		return nil, err
	}
	return &fileRewrite{
		fileSnapshot:   *snapshot,
		sourceID:       linkPlan.sourceID,
		updatedContent: append([]byte(nil), snapshot.content...),
	}, nil
}

func rewriteIndexedLinkTarget(content []byte, link model.Link, replacement string) ([]byte, bool) {
	lineStart, ok := contentLineStart(content, link.Line)
	if !ok {
		return content, false
	}
	start := lineStart + link.PositionStart
	end := lineStart + link.PositionEnd
	targetStart := -1
	if start >= 0 && end <= len(content) && start < end {
		span := content[start:end]
		if targetOffset, found := indexedRawTargetOffset(span, []byte(link.RawTarget)); found {
			targetStart = start + targetOffset
		}
	}
	if targetStart < 0 {
		targetStart, ok = nearestCurrentLinkTarget(content, link.RawTarget, start)
		if !ok {
			return content, false
		}
	}
	targetEnd := targetStart + len(link.RawTarget)

	updated := make([]byte, 0, len(content)-len(link.RawTarget)+len(replacement))
	updated = append(updated, content[:targetStart]...)
	updated = append(updated, replacement...)
	updated = append(updated, content[targetEnd:]...)
	return updated, true
}

func nearestCurrentLinkTarget(content []byte, rawTarget string, expectedStart int) (int, bool) {
	extracted, err := parser.ExtractFromAST(content, 1)
	if err != nil {
		return 0, false
	}

	best := -1
	bestDistance := 0
	for _, candidate := range extracted.Links {
		if candidate.RawTarget != rawTarget {
			continue
		}
		lineStart, ok := contentLineStart(content, candidate.Line)
		if !ok {
			continue
		}
		syntaxStart := lineStart + candidate.PositionStart
		syntaxEnd := lineStart + candidate.PositionEnd
		if syntaxStart < 0 || syntaxEnd > len(content) || syntaxStart >= syntaxEnd {
			continue
		}
		offset, ok := indexedRawTargetOffset(content[syntaxStart:syntaxEnd], []byte(rawTarget))
		if !ok {
			continue
		}
		targetStart := syntaxStart + offset
		distance := targetStart - expectedStart
		if distance < 0 {
			distance = -distance
		}
		if best < 0 || distance < bestDistance {
			best = targetStart
			bestDistance = distance
		}
	}
	return best, best >= 0
}

func contentLineStart(content []byte, line int) (int, bool) {
	if line < 1 {
		return 0, false
	}
	if line == 1 {
		return 0, true
	}
	current := 1
	for i, b := range content {
		if b != '\n' {
			continue
		}
		current++
		if current == line {
			return i + 1, true
		}
	}
	return 0, false
}

func indexedRawTargetOffset(linkSyntax, rawTarget []byte) (int, bool) {
	if len(rawTarget) == 0 {
		return 0, false
	}
	for searchFrom := 0; searchFrom <= len(linkSyntax)-len(rawTarget); {
		relative := strings.Index(string(linkSyntax[searchFrom:]), string(rawTarget))
		if relative < 0 {
			break
		}
		offset := searchFrom + relative
		open := strings.LastIndex(string(linkSyntax[:offset]), "](")
		if open >= 0 && strings.TrimSpace(string(linkSyntax[open+2:offset])) == "" {
			return offset, true
		}
		searchFrom = offset + 1
	}
	return 0, false
}

func planRewriteForSource(vaultPath string, vaultCfg *config.VaultConfig, refPlan refUpdatePlan) (*fileRewrite, error) {
	fileSourceID, _, _ := paths.ParseSectionID(refPlan.sourceID)

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileSourceID, vaultCfg)
	if err != nil {
		return nil, err
	}
	if err := mutationguard.ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath); err != nil {
		return nil, err
	}

	snapshot, err := readFileSnapshot(filePath)
	if err != nil {
		return nil, err
	}

	return &fileRewrite{
		fileSnapshot:   *snapshot,
		sourceID:       refPlan.sourceID,
		updatedContent: append([]byte(nil), snapshot.content...),
	}, nil
}

func movedDocumentSource(sourceID, destinationObject string) bool {
	if sourceID == destinationObject {
		return true
	}
	return strings.HasPrefix(sourceID, destinationObject+"#")
}

func prepareRefUpdatePlans(db *index.Database, rt *vaultruntime.Runtime, req MoveFileRequest, objectRoot, pageRoot, dailyDir string, warnings []string) ([]refUpdatePlan, []fieldRefUpdatePlan, []string) {
	backlinks, err := db.BacklinksWithRoots(req.SourceObjectID, objectRoot, pageRoot)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to read backlinks for move update: %v", err))
		backlinks = nil
	}
	fieldBacklinks, err := db.FieldBacklinksWithRoots(req.SourceObjectID, objectRoot, pageRoot)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to read ref fields for move update: %v", err))
		fieldBacklinks = nil
	}
	if len(backlinks) == 0 && len(fieldBacklinks) == 0 {
		return nil, nil, warnings
	}

	aliases, err := db.AllAliases()
	if err != nil {
		return nil, nil, append(warnings, fmt.Sprintf("Failed to read aliases for move update: %v", err))
	}

	resolverOpts := indexschema.ResolverOptions{
		DailyDirectory: dailyDir,
		ExtraIDs:       []string{req.DestinationObject},
	}
	res, err := db.Resolver(resolverOpts)
	if err != nil {
		return nil, nil, append(warnings, fmt.Sprintf("Failed to build resolver for move update: %v", err))
	}

	aliasSlugToID := make(map[string]string, len(aliases))
	for alias, oid := range aliases {
		aliasSlugToID[slugs.PathSlug(alias)] = oid
	}

	plans := make([]refUpdatePlan, 0, len(backlinks))
	for _, bl := range backlinks {
		base := refBaseFromTargetRaw(bl.TargetRaw)
		if base == "" {
			continue
		}

		line := 0
		if bl.Line != nil {
			line = *bl.Line
		}

		sourceID := remapSourceIDThroughMoves(bl.SourceID, rt, req)
		plans = append(plans, refUpdatePlan{
			sourceID:    sourceID,
			line:        line,
			oldBase:     base,
			replacement: ChooseReplacementRefBase(base, req.SourceObjectID, req.DestinationObject, aliasSlugToID, res),
		})
	}

	fieldPlans := make([]fieldRefUpdatePlan, 0, len(fieldBacklinks))
	for _, fieldRef := range fieldBacklinks {
		base := refBaseFromTargetRaw(fieldRef.TargetRaw)
		if base == "" {
			continue
		}

		sourceID := remapSourceIDThroughMoves(fieldRef.SourceID, rt, req)
		fieldPlans = append(fieldPlans, fieldRefUpdatePlan{
			sourceID:    sourceID,
			filePath:    remapRefFieldFilePath(fieldRef.FilePath, rt, req),
			fieldName:   fieldRef.FieldName,
			oldBase:     base,
			replacement: ChooseReplacementRefBase(base, req.SourceObjectID, req.DestinationObject, aliasSlugToID, res),
		})
	}

	return plans, fieldPlans, warnings
}

func prepareLinkUpdatePlans(db *index.Database, rt *vaultruntime.Runtime, req MoveFileRequest, warnings []string) ([]linkUpdatePlan, []string) {
	sourceRel, err := filepath.Rel(rt.VaultPath, req.SourceFile)
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to resolve source path for file-link updates: %v", err))
	}
	sourceKey := paths.NormalizeVaultRelPath(sourceRel)

	links, err := db.FileLinksByNormalizedKey(sourceKey)
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to read inbound file links for move update: %v", err))
	}

	destRel, err := filepath.Rel(rt.VaultPath, req.DestinationFile)
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to resolve destination path for file-link updates: %v", err))
	}
	destKey := paths.NormalizeVaultRelPath(destRel)

	plans := make([]linkUpdatePlan, 0, len(links))
	for _, link := range links {
		filePath := paths.NormalizeVaultRelPath(link.FilePath)
		sourceID := remapSourceIDThroughMoves(link.SourceID, rt, req)
		for _, priorMove := range req.PriorMoves {
			filePath = remapMovedFilePath(filePath, priorMove)
		}
		plans = append(plans, linkUpdatePlan{
			sourceID:    sourceID,
			filePath:    filePath,
			link:        link,
			replacement: linktarget.RetargetFile(link.RawTarget, filePath, rt.VaultPath, destKey),
		})
	}
	return plans, warnings
}

func remapSourceIDThroughMoves(sourceID string, rt *vaultruntime.Runtime, req MoveFileRequest) string {
	sourceID = remapMovedSourceID(sourceID, req.SourceObjectID, req.DestinationObject)
	for _, priorMove := range req.PriorMoves {
		sourceID = remapMovedSourceID(
			sourceID,
			movePathObjectID(priorMove.From, rt.VaultCfg),
			movePathObjectID(priorMove.To, rt.VaultCfg),
		)
	}
	return sourceID
}

func remapMovedFilePath(filePath string, move mutation.Move) string {
	if paths.NormalizeVaultRelPath(filePath) == paths.NormalizeVaultRelPath(move.From) {
		return paths.NormalizeVaultRelPath(move.To)
	}
	return filePath
}

func remapRefFieldFilePath(filePath string, rt *vaultruntime.Runtime, req MoveFileRequest) string {
	sourceRel, sourceErr := filepath.Rel(rt.VaultPath, req.SourceFile)
	destRel, destErr := filepath.Rel(rt.VaultPath, req.DestinationFile)
	if sourceErr == nil && destErr == nil {
		filePath = remapMovedFilePath(filePath, mutation.Move{From: sourceRel, To: destRel})
	}
	for _, priorMove := range req.PriorMoves {
		filePath = remapMovedFilePath(filePath, priorMove)
	}
	return paths.NormalizeVaultRelPath(filePath)
}

func movePathObjectID(relPath string, vaultCfg *config.VaultConfig) string {
	relPath = paths.NormalizeVaultRelPath(relPath)
	if !paths.HasMDExtension(relPath) {
		return relPath
	}
	if vaultCfg != nil {
		return vaultCfg.FilePathToObjectID(relPath)
	}
	return strings.TrimSuffix(relPath, filepath.Ext(relPath))
}

// refBaseFromTargetRaw extracts the base target (without any section fragment)
// from a backlink's raw target. The stored target is normally a bare target,
// but a wikilink literal is tolerated for robustness.
func refBaseFromTargetRaw(targetRaw string) string {
	raw := strings.TrimSpace(targetRaw)
	if target, _, ok := wikilink.ParseExact(raw); ok {
		raw = target
	}
	base, _, _ := paths.ParseSectionID(raw)
	return strings.TrimSpace(base)
}

func remapMovedSourceID(sourceID, oldID, newID string) string {
	if sourceID == oldID {
		return newID
	}
	prefix := oldID + "#"
	if strings.HasPrefix(sourceID, prefix) {
		return newID + sourceID[len(oldID):]
	}
	return sourceID
}
