package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/linktarget"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/wikilink"
)

type MoveFileRequest struct {
	VaultPath          string
	SourceFile         string
	DestinationFile    string
	SourceObjectID     string
	DestinationObject  string
	ReplacementContent []byte
	UpdateRefs         bool
	Preview            bool
	PriorMoves         []mutation.Move
	VaultConfig        *config.VaultConfig
	Schema             *schema.Schema
	ParseOptions       *parser.ParseOptions
	Runtime            *vaultruntime.Runtime
}

type MoveFileResult struct {
	UpdatedRefs     []string
	WarningMessages []string
	ChangeSet       mutation.ChangeSet
}

type refUpdatePlan struct {
	reportSourceID string
	applySourceID  string
	line           int
	oldBase        string
	replacement    string
}

type linkUpdatePlan struct {
	reportSourceID string
	filePath       string
	link           model.Link
	replacement    string
}

type fileSnapshot struct {
	path    string
	content []byte
	perm    os.FileMode
}

type fileRewrite struct {
	fileSnapshot
	reportSourceID string
	updatedContent []byte
}

type moveWritePlan struct {
	destinationContent []byte
	rewriteFiles       []*fileRewrite
	updatedRefs        []string
}

var (
	moveFileWriterMu sync.RWMutex
	moveFileWriter   = atomicfile.WriteFile
)

func MoveFile(req MoveFileRequest) (*MoveFileResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if strings.TrimSpace(req.SourceFile) == "" || strings.TrimSpace(req.DestinationFile) == "" {
		return nil, newError(ErrorInvalidInput, "source and destination files are required", "", nil, nil)
	}
	if strings.TrimSpace(req.SourceObjectID) == "" || strings.TrimSpace(req.DestinationObject) == "" {
		return nil, newError(ErrorInvalidInput, "source and destination object IDs are required", "", nil, nil)
	}

	// Guard both source and destination against protected prefixes, exclude
	// patterns, and the template directory. MoveByReference already validates
	// these paths, but callers like Reclassify derive the destination from a
	// type's default_path and would otherwise bypass the guard.
	if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, req.SourceFile); err != nil {
		return nil, err
	}
	if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, req.DestinationFile); err != nil {
		return nil, err
	}

	result := &MoveFileResult{}
	objectRoot := ""
	pageRoot := ""
	dailyDir := ""
	if req.VaultConfig != nil {
		objectRoot = req.VaultConfig.GetObjectsRoot()
		pageRoot = req.VaultConfig.GetPagesRoot()
		dailyDir = req.VaultConfig.GetDailyDirectory()
	}

	rt, owned := requestRuntime(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if owned {
		defer rt.Close()
	}
	var db *index.Database
	if err := rt.OpenDB(); err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to open index database for move update: %v", err))
	} else {
		db = rt.DB
	}

	var refPlans []refUpdatePlan
	var linkPlans []linkUpdatePlan
	if req.UpdateRefs && db != nil {
		if paths.HasMDExtension(req.SourceFile) {
			refPlans, result.WarningMessages = prepareRefUpdatePlans(db, req, objectRoot, pageRoot, dailyDir, result.WarningMessages)
		}
		linkPlans, result.WarningMessages = prepareLinkUpdatePlans(db, req, result.WarningMessages)
	}

	sourceSnapshot, err := readFileSnapshot(req.SourceFile)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read source file", "", nil, err)
	}

	writePlan, warnings, err := prepareMoveWritePlan(req, refPlans, linkPlans, sourceSnapshot, objectRoot, pageRoot)
	result.WarningMessages = append(result.WarningMessages, warnings...)
	if err != nil {
		return nil, err
	}

	if req.Preview {
		result.UpdatedRefs = append(result.UpdatedRefs, writePlan.updatedRefs...)
		return result, nil
	}

	if err := os.MkdirAll(filepath.Dir(req.DestinationFile), 0o755); err != nil {
		return nil, newError(ErrorFileWrite, "failed to create destination directory", "", nil, err)
	}
	if len(writePlan.destinationContent) > 0 {
		if err := writeMoveFile(req.DestinationFile, writePlan.destinationContent, sourceSnapshot.perm); err != nil {
			return nil, newError(ErrorFileWrite, "failed to write moved file", "", nil, err)
		}
		if err := os.Remove(req.SourceFile); err != nil {
			_ = os.Remove(req.DestinationFile)
			return nil, newError(ErrorFileWrite, "failed to remove source file after move", "", nil, err)
		}
	} else {
		if err := os.Rename(req.SourceFile, req.DestinationFile); err != nil {
			return nil, newError(ErrorFileWrite, "failed to move file", "", nil, err)
		}
	}

	var appliedRewrites []*fileRewrite
	for _, rewrite := range writePlan.rewriteFiles {
		if err := writeMoveFile(rewrite.path, rewrite.updatedContent, rewrite.perm); err != nil {
			rollbackErr := rollbackMovedFiles(req, sourceSnapshot, appliedRewrites)
			return nil, moveRollbackError("failed to update refs after move", err, rollbackErr)
		}
		appliedRewrites = append(appliedRewrites, rewrite)
	}
	result.UpdatedRefs = append(result.UpdatedRefs, writePlan.updatedRefs...)

	sourceRel, sourceRelErr := filepath.Rel(req.VaultPath, req.SourceFile)
	destRel, destRelErr := filepath.Rel(req.VaultPath, req.DestinationFile)
	if sourceRelErr == nil && destRelErr == nil {
		result.ChangeSet.AddMoved(sourceRel, destRel)
	}
	for _, rewrite := range appliedRewrites {
		if relPath, relErr := filepath.Rel(req.VaultPath, rewrite.path); relErr == nil {
			result.ChangeSet.AddChanged(relPath)
		}
	}

	return result, nil
}

func prepareMoveWritePlan(req MoveFileRequest, refPlans []refUpdatePlan, linkPlans []linkUpdatePlan, sourceSnapshot *fileSnapshot, objectRoot, pageRoot string) (*moveWritePlan, []string, error) {
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
		rewrite, err := planRewriteForLinkSource(req.VaultPath, req.VaultConfig, linkPlan)
		if err != nil {
			var svcErr *Error
			if errors.As(err, &svcErr) && svcErr.Code == ErrorValidationFailed {
				return nil, warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("Failed to update file link in %s: %v", linkPlan.reportSourceID, err))
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
			warnings = append(warnings, fmt.Sprintf("Failed to update file link in %s: indexed link no longer matches source", linkPlan.reportSourceID))
			continue
		}
		existing.updatedContent = updated
		addUpdatedRef(linkPlan.reportSourceID)
	}

	for _, refPlan := range refPlans {
		if movedDocumentSource(refPlan.applySourceID, req.DestinationObject) {
			updated := ApplyAllRefVariantsAtLine(destCurrent, refPlan.line, req.SourceObjectID, refPlan.oldBase, refPlan.replacement, objectRoot, pageRoot)
			if updated == destCurrent {
				continue
			}
			destCurrent = updated
			plan.destinationContent = []byte(destCurrent)
			addUpdatedRef(refPlan.reportSourceID)
			continue
		}

		rewrite, err := planRewriteForSource(req.VaultPath, req.VaultConfig, refPlan)
		if err != nil {
			var svcErr *Error
			if errors.As(err, &svcErr) && svcErr.Code == ErrorValidationFailed {
				return nil, warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("Failed to update refs in %s: %v", refPlan.reportSourceID, err))
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
		addUpdatedRef(refPlan.reportSourceID)
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
	if err := ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath); err != nil {
		return nil, err
	}

	snapshot, err := readFileSnapshot(filePath)
	if err != nil {
		return nil, err
	}
	return &fileRewrite{
		fileSnapshot:   *snapshot,
		reportSourceID: linkPlan.reportSourceID,
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
	fileSourceID, _, _ := paths.ParseSectionID(refPlan.applySourceID)

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileSourceID, vaultCfg)
	if err != nil {
		return nil, err
	}
	if err := ValidateContentMutationFilePath(vaultPath, vaultCfg, filePath); err != nil {
		return nil, err
	}

	snapshot, err := readFileSnapshot(filePath)
	if err != nil {
		return nil, err
	}

	return &fileRewrite{
		fileSnapshot:   *snapshot,
		reportSourceID: refPlan.reportSourceID,
		updatedContent: append([]byte(nil), snapshot.content...),
	}, nil
}

func readFileSnapshot(path string) (*fileSnapshot, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	perm := os.FileMode(0)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode()
	}

	return &fileSnapshot{
		path:    path,
		content: content,
		perm:    perm,
	}, nil
}

func rollbackMovedFiles(req MoveFileRequest, sourceSnapshot *fileSnapshot, rewrites []*fileRewrite) error {
	var rollbackErr error

	for i := len(rewrites) - 1; i >= 0; i-- {
		rewrite := rewrites[i]
		if err := writeMoveFile(rewrite.path, rewrite.content, rewrite.perm); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %s: %w", rewrite.reportSourceID, err))
		}
	}

	if err := writeMoveFile(req.SourceFile, sourceSnapshot.content, sourceSnapshot.perm); err != nil {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore source file: %w", err))
	}
	if err := os.Remove(req.DestinationFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove destination file: %w", err))
	}

	return rollbackErr
}

func moveRollbackError(message string, cause, rollbackErr error) error {
	if rollbackErr != nil {
		return newError(
			ErrorValidationFailed,
			message,
			"Inspect affected files and run 'rvn reindex' if needed; rollback was only partially successful",
			nil,
			errors.Join(cause, rollbackErr),
		)
	}

	return newError(
		ErrorValidationFailed,
		message,
		"Move was rolled back; fix the underlying error and try again",
		nil,
		cause,
	)
}

func movedDocumentSource(sourceID, destinationObject string) bool {
	if sourceID == destinationObject {
		return true
	}
	return strings.HasPrefix(sourceID, destinationObject+"#")
}

func writeMoveFile(path string, data []byte, perm os.FileMode) error {
	moveFileWriterMu.RLock()
	writer := moveFileWriter
	moveFileWriterMu.RUnlock()
	return writer(path, data, perm)
}

func prepareRefUpdatePlans(db *index.Database, req MoveFileRequest, objectRoot, pageRoot, dailyDir string, warnings []string) ([]refUpdatePlan, []string) {
	backlinks, err := db.BacklinksWithRoots(req.SourceObjectID, objectRoot, pageRoot)
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to read backlinks for move update: %v", err))
	}

	aliases, err := db.AllAliases()
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to read aliases for move update: %v", err))
	}

	resolverOpts := indexschema.ResolverOptions{
		DailyDirectory: dailyDir,
		ExtraIDs:       []string{req.DestinationObject},
	}
	res, err := db.Resolver(resolverOpts)
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to build resolver for move update: %v", err))
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

		reportSourceID := remapMovedSourceID(bl.SourceID, req.SourceObjectID, req.DestinationObject)
		for _, priorMove := range req.PriorMoves {
			reportSourceID = remapMovedSourceID(
				reportSourceID,
				movePathObjectID(priorMove.From, req.VaultConfig),
				movePathObjectID(priorMove.To, req.VaultConfig),
			)
		}
		plans = append(plans, refUpdatePlan{
			reportSourceID: reportSourceID,
			applySourceID:  reportSourceID,
			line:           line,
			oldBase:        base,
			replacement:    ChooseReplacementRefBase(base, req.SourceObjectID, req.DestinationObject, aliasSlugToID, res),
		})
	}

	return plans, warnings
}

func prepareLinkUpdatePlans(db *index.Database, req MoveFileRequest, warnings []string) ([]linkUpdatePlan, []string) {
	sourceRel, err := filepath.Rel(req.VaultPath, req.SourceFile)
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to resolve source path for file-link updates: %v", err))
	}
	sourceKey := paths.NormalizeVaultRelPath(sourceRel)

	links, err := db.FileLinksByNormalizedKey(sourceKey)
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to read inbound file links for move update: %v", err))
	}

	destRel, err := filepath.Rel(req.VaultPath, req.DestinationFile)
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to resolve destination path for file-link updates: %v", err))
	}
	destKey := paths.NormalizeVaultRelPath(destRel)

	plans := make([]linkUpdatePlan, 0, len(links))
	for _, link := range links {
		filePath := paths.NormalizeVaultRelPath(link.FilePath)
		reportSourceID := remapMovedSourceID(link.SourceID, req.SourceObjectID, req.DestinationObject)
		for _, priorMove := range req.PriorMoves {
			filePath = remapMovedFilePath(filePath, priorMove)
			reportSourceID = remapMovedSourceID(
				reportSourceID,
				movePathObjectID(priorMove.From, req.VaultConfig),
				movePathObjectID(priorMove.To, req.VaultConfig),
			)
		}
		plans = append(plans, linkUpdatePlan{
			reportSourceID: reportSourceID,
			filePath:       filePath,
			link:           link,
			replacement:    linktarget.RetargetFile(link.RawTarget, filePath, req.VaultPath, destKey),
		})
	}
	return plans, warnings
}

func remapMovedFilePath(filePath string, move mutation.Move) string {
	if paths.NormalizeVaultRelPath(filePath) == paths.NormalizeVaultRelPath(move.From) {
		return paths.NormalizeVaultRelPath(move.To)
	}
	return filePath
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
