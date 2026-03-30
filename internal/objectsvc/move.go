package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type MoveFileRequest struct {
	VaultPath          string
	SourceFile         string
	DestinationFile    string
	SourceObjectID     string
	DestinationObject  string
	ReplacementContent []byte
	UpdateRefs         bool
	FailOnIndexError   bool
	VaultConfig        *config.VaultConfig
	Schema             *schema.Schema
	ParseOptions       *parser.ParseOptions
}

type MoveFileResult struct {
	UpdatedRefs     []string
	WarningMessages []string
}

type refUpdatePlan struct {
	reportSourceID string
	applySourceID  string
	line           int
	oldBase        string
	replacement    string
}

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

	var db *index.Database
	var err error
	db, err = index.Open(req.VaultPath)
	if err != nil {
		if req.FailOnIndexError {
			return nil, newError(ErrorValidationFailed, "failed to open index database for move", "Run 'rvn reindex' to rebuild the database", nil, err)
		}
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to open index database for move update: %v", err))
	} else {
		defer db.Close()
		db.SetDailyDirectory(dailyDir)
	}

	var refPlans []refUpdatePlan
	if req.UpdateRefs && db != nil {
		refPlans, result.WarningMessages = prepareRefUpdatePlans(db, req, objectRoot, pageRoot, dailyDir, result.WarningMessages)
	}

	if err := os.MkdirAll(filepath.Dir(req.DestinationFile), 0o755); err != nil {
		return nil, newError(ErrorFileWrite, "failed to create destination directory", "", nil, err)
	}
	if len(req.ReplacementContent) > 0 {
		perm := os.FileMode(0)
		if st, err := os.Stat(req.SourceFile); err == nil {
			perm = st.Mode()
		}
		if err := atomicfile.WriteFile(req.DestinationFile, req.ReplacementContent, perm); err != nil {
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

	for _, plan := range refPlans {
		if err := UpdateAllRefVariantsAtLine(req.VaultPath, req.VaultConfig, plan.applySourceID, plan.line, req.SourceObjectID, plan.oldBase, plan.replacement, objectRoot, pageRoot); err != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to update refs in %s: %v", plan.reportSourceID, err))
			continue
		}
		result.UpdatedRefs = append(result.UpdatedRefs, plan.reportSourceID)
	}

	if db == nil {
		return result, nil
	}

	if err := db.RemoveDocument(req.SourceObjectID); err != nil {
		if errors.Is(err, index.ErrObjectNotFound) {
			result.WarningMessages = append(result.WarningMessages, "Object not found in index while updating move; consider running 'rvn reindex'")
		} else if req.FailOnIndexError {
			return nil, newError(ErrorValidationFailed, "failed to remove old index entry", "Run 'rvn reindex' to rebuild the database", nil, err)
		} else {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to remove old index entry: %v", err))
		}
	}

	newContent, err := os.ReadFile(req.DestinationFile)
	if err != nil {
		if req.FailOnIndexError {
			return nil, newError(ErrorFileRead, "failed to read moved file for indexing", "Run 'rvn reindex' to rebuild the database", nil, err)
		}
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to read moved file for indexing: %v", err))
		return result, nil
	}

	newDoc, err := parser.ParseDocumentWithOptions(string(newContent), req.DestinationFile, req.VaultPath, req.ParseOptions)
	if err != nil {
		if req.FailOnIndexError {
			return nil, newError(ErrorValidationFailed, "failed to parse moved file for indexing", "Run 'rvn reindex' to rebuild the database", nil, err)
		}
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to parse moved file for indexing: %v", err))
		return result, nil
	}
	if newDoc == nil {
		msg := "Failed to parse moved file for indexing: got nil document"
		if req.FailOnIndexError {
			return nil, newError(ErrorValidationFailed, msg, "Run 'rvn reindex' to rebuild the database", nil, nil)
		}
		result.WarningMessages = append(result.WarningMessages, msg)
		return result, nil
	}

	if req.Schema != nil {
		if err := db.IndexDocument(newDoc, req.Schema); err != nil {
			if req.FailOnIndexError {
				return nil, newError(ErrorValidationFailed, "failed to index moved file", "Run 'rvn reindex' to rebuild the database", nil, err)
			}
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to index moved file: %v", err))
		}
	}

	return result, nil
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

	res, err := db.Resolver(index.ResolverOptions{DailyDirectory: dailyDir, ExtraIDs: []string{req.DestinationObject}})
	if err != nil {
		return nil, append(warnings, fmt.Sprintf("Failed to build resolver for move update: %v", err))
	}

	aliasSlugToID := make(map[string]string, len(aliases))
	for alias, oid := range aliases {
		aliasSlugToID[pages.SlugifyPath(alias)] = oid
	}

	plans := make([]refUpdatePlan, 0, len(backlinks))
	for _, bl := range backlinks {
		oldRaw := strings.TrimSpace(bl.TargetRaw)
		oldRaw = strings.TrimPrefix(strings.TrimSuffix(oldRaw, "]]"), "[[")
		base := oldRaw
		if i := strings.Index(base, "#"); i >= 0 {
			base = base[:i]
		}
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
