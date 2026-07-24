package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
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
	VaultConfig        *config.VaultConfig
	Schema             *schema.Schema
	ParseOptions       *parser.ParseOptions
	IsAsset            bool
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
	if req.UpdateRefs && db != nil {
		refPlans, result.WarningMessages = prepareRefUpdatePlans(db, req, objectRoot, pageRoot, dailyDir, result.WarningMessages)
	}

	sourceSnapshot, err := readFileSnapshot(req.SourceFile)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read source file", "", nil, err)
	}

	writePlan, warnings, err := prepareMoveWritePlan(req, refPlans, sourceSnapshot, objectRoot, pageRoot)
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

func prepareMoveWritePlan(req MoveFileRequest, refPlans []refUpdatePlan, sourceSnapshot *fileSnapshot, objectRoot, pageRoot string) (*moveWritePlan, []string, error) {
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

	resolverOpts := index.ResolverOptions{DailyDirectory: dailyDir}
	if req.IsAsset {
		resolverOpts.ExtraAssetIDs = []string{req.DestinationObject}
	} else {
		resolverOpts.ExtraIDs = []string{req.DestinationObject}
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
