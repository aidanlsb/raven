package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type MoveFileRequest struct {
	SourceFile         string
	DestinationFile    string
	SourceObjectID     string
	DestinationObject  string
	ReplacementContent []byte
	UpdateRefs         bool
	Preview            bool
	PriorMoves         []mutation.Move
}

type MoveFileResult struct {
	UpdatedRefs      []string
	UpdatedRefFields []RefFieldUpdate
	WarningMessages  []string
	ChangeSet        mutation.ChangeSet
}

// RefFieldUpdate identifies a schema-typed frontmatter ref field rewritten by
// a move. FilePath is vault-relative and reflects any move of the source file.
type RefFieldUpdate struct {
	SourceID string `json:"source_id"`
	FilePath string `json:"file_path"`
	Field    string `json:"field"`
}

type refUpdatePlan struct {
	sourceID    string
	line        int
	oldBase     string
	replacement string
}

type fieldRefUpdatePlan struct {
	sourceID    string
	filePath    string
	fieldName   string
	oldBase     string
	replacement string
}

type linkUpdatePlan struct {
	sourceID    string
	filePath    string
	link        model.Link
	replacement string
}

type fileSnapshot struct {
	path    string
	content []byte
	perm    os.FileMode
}

type fileRewrite struct {
	fileSnapshot
	sourceID       string
	updatedContent []byte
}

type moveWritePlan struct {
	destinationContent []byte
	rewriteFiles       []*fileRewrite
	updatedRefs        []string
	updatedRefFields   []RefFieldUpdate
}

var (
	moveFileWriterMu sync.RWMutex
	moveFileWriter   = atomicfile.WriteFile
)

func MoveFile(rt *vaultruntime.Runtime, req MoveFileRequest) (*MoveFileResult, error) {
	if err := requireRuntime(rt); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.SourceFile) == "" || strings.TrimSpace(req.DestinationFile) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "source and destination files are required")
	}
	if strings.TrimSpace(req.SourceObjectID) == "" || strings.TrimSpace(req.DestinationObject) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "source and destination object IDs are required")
	}

	// Guard both source and destination against protected prefixes, exclude
	// patterns, and the template directory. MoveByReference already validates
	// these paths, but callers like Reclassify derive the destination from a
	// type's default_path and would otherwise bypass the guard.
	if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, req.SourceFile); err != nil {
		return nil, err
	}
	if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, req.DestinationFile); err != nil {
		return nil, err
	}

	result := &MoveFileResult{}
	objectRoot := objectsRoot(rt)
	pageRoot := pagesRoot(rt)
	dailyDir := ""
	if rt.VaultCfg != nil {
		dailyDir = rt.VaultCfg.GetDailyDirectory()
	}

	var db *index.Database
	if err := rt.OpenDB(); err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to open index database for move update: %v", err))
	} else {
		db = rt.DB
	}

	var refPlans []refUpdatePlan
	var fieldRefPlans []fieldRefUpdatePlan
	var linkPlans []linkUpdatePlan
	if req.UpdateRefs && db != nil {
		if paths.HasMDExtension(req.SourceFile) {
			refPlans, fieldRefPlans, result.WarningMessages = prepareRefUpdatePlans(db, rt, req, objectRoot, pageRoot, dailyDir, result.WarningMessages)
		}
		linkPlans, result.WarningMessages = prepareLinkUpdatePlans(db, rt, req, result.WarningMessages)
	}

	sourceSnapshot, err := readFileSnapshot(req.SourceFile)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read source file", err)
	}

	writePlan, warnings, err := prepareMoveWritePlan(rt, req, refPlans, fieldRefPlans, linkPlans, sourceSnapshot, objectRoot, pageRoot)
	result.WarningMessages = append(result.WarningMessages, warnings...)
	if err != nil {
		return nil, err
	}

	if req.Preview {
		result.UpdatedRefs = append(result.UpdatedRefs, writePlan.updatedRefs...)
		result.UpdatedRefFields = append(result.UpdatedRefFields, writePlan.updatedRefFields...)
		return result, nil
	}

	if err := os.MkdirAll(filepath.Dir(req.DestinationFile), 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create destination directory", err)
	}
	if len(writePlan.destinationContent) > 0 {
		if err := writeMoveFile(req.DestinationFile, writePlan.destinationContent, sourceSnapshot.perm); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write moved file", err)
		}
		if err := os.Remove(req.SourceFile); err != nil {
			_ = os.Remove(req.DestinationFile)
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to remove source file after move", err)
		}
	} else {
		if err := os.Rename(req.SourceFile, req.DestinationFile); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to move file", err)
		}
	}

	var appliedRewrites []*fileRewrite
	for _, rewrite := range writePlan.rewriteFiles {
		if err := writeMoveFile(rewrite.path, rewrite.updatedContent, rewrite.perm); err != nil {
			rollbackErr := rollbackMovedFiles(rt, req, sourceSnapshot, appliedRewrites)
			return nil, moveRollbackError("failed to update refs after move", err, rollbackErr)
		}
		appliedRewrites = append(appliedRewrites, rewrite)
	}
	result.UpdatedRefs = append(result.UpdatedRefs, writePlan.updatedRefs...)
	result.UpdatedRefFields = append(result.UpdatedRefFields, writePlan.updatedRefFields...)

	sourceRel, sourceRelErr := filepath.Rel(rt.VaultPath, req.SourceFile)
	destRel, destRelErr := filepath.Rel(rt.VaultPath, req.DestinationFile)
	if sourceRelErr == nil && destRelErr == nil {
		result.ChangeSet.AddMoved(sourceRel, destRel)
	}
	for _, rewrite := range appliedRewrites {
		if relPath, relErr := filepath.Rel(rt.VaultPath, rewrite.path); relErr == nil {
			result.ChangeSet.AddChanged(relPath)
		}
	}

	return result, nil
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

func rollbackMovedFiles(rt *vaultruntime.Runtime, req MoveFileRequest, sourceSnapshot *fileSnapshot, rewrites []*fileRewrite) error {
	var rollbackErr error

	for i := len(rewrites) - 1; i >= 0; i-- {
		rewrite := rewrites[i]
		if err := writeMoveFile(rewrite.path, rewrite.content, rewrite.perm); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %s: %w", rewrite.sourceID, err))
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
		return svcerr.Wrap(codes.ErrValidationFailed, message, errors.Join(cause, rollbackErr)).WithSuggestion("Inspect affected files and run 'rvn reindex' if needed; rollback was only partially successful")
	}

	return svcerr.Wrap(codes.ErrValidationFailed, message, cause).WithSuggestion("Move was rolled back; fix the underlying error and try again")
}

func writeMoveFile(path string, data []byte, perm os.FileMode) error {
	moveFileWriterMu.RLock()
	writer := moveFileWriter
	moveFileWriterMu.RUnlock()
	return writer(path, data, perm)
}
