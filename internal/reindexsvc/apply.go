package reindexsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
)

// indexSink receives successfully parsed vault files during apply.
type indexSink interface {
	indexParsed(walkResult vault.WalkResult) bool
	stats() (index.IndexStats, error)
}

type writerSink struct {
	db     *index.Database
	schema *schema.Schema
	result *RunResult
}

func (s *writerSink) indexParsed(walkResult vault.WalkResult) bool {
	if idxErr := s.db.IndexDocumentWithMtime(walkResult.Document, s.schema, walkResult.FileMtime); idxErr != nil {
		s.result.Errors = append(s.result.Errors, fmt.Sprintf("%s: %v", walkResult.RelativePath, idxErr))
		return false
	}
	s.result.WarningMessages = append(s.result.WarningMessages, index.UnknownFrontmatterWarnings(walkResult.Document, s.schema)...)
	s.result.FilesIndexed++
	return true
}

func (s *writerSink) stats() (index.IndexStats, error) {
	stats, err := s.db.Stats()
	if err != nil {
		return index.IndexStats{}, err
	}
	return *stats, nil
}

// countingSink records parsed-document stats for dry-run without writing.
type countingSink struct {
	db           *index.Database
	incremental  bool
	removedFiles []string
	fileStats    map[string]index.IndexStats
	accumulated  index.IndexStats
	result       *RunResult
}

func (s *countingSink) indexParsed(walkResult vault.WalkResult) bool {
	s.result.FilesIndexed++
	docStats := parsedDocumentStats(walkResult.Document)
	s.fileStats[walkResult.RelativePath] = docStats
	s.accumulated.ObjectCount += docStats.ObjectCount
	s.accumulated.TraitCount += docStats.TraitCount
	s.accumulated.RefCount += docStats.RefCount
	return true
}

func (s *countingSink) stats() (index.IndexStats, error) {
	if !s.incremental {
		return s.accumulated, nil
	}
	stats, err := s.db.Stats()
	if err != nil {
		return index.IndexStats{}, err
	}
	projected := *stats

	for _, filePath := range s.removedFiles {
		current, err := s.db.StatsForFile(filePath)
		if err != nil {
			return index.IndexStats{}, err
		}
		projected.ObjectCount -= current.ObjectCount
		projected.TraitCount -= current.TraitCount
		projected.RefCount -= current.RefCount
	}

	for filePath, next := range s.fileStats {
		current, err := s.db.StatsForFile(filePath)
		if err != nil {
			return index.IndexStats{}, err
		}
		projected.ObjectCount += next.ObjectCount - current.ObjectCount
		projected.TraitCount += next.TraitCount - current.TraitCount
		projected.RefCount += next.RefCount - current.RefCount
	}

	return projected, nil
}

func apply(prep *preparedRun, plan *filePlan) (*RunResult, error) {
	result := &RunResult{
		SchemaRebuilt:   prep.wasRebuilt,
		Incremental:     prep.incremental,
		DryRun:          prep.dryRun,
		Errors:          plan.errors,
		StaleFiles:      []string{},
		DeletedFiles:    plan.deletedFiles,
		ExcludedFiles:   plan.excludedFiles,
		WarningMessages: plan.warningMessages,
		HasRefResult:    false,
		RefsResolved:    0,
		RefsUnresolved:  0,
		FilesIndexed:    0,
		FilesSkipped:    0,
		FilesDeleted:    plan.filesDeleted,
		FilesExcluded:   plan.filesExcluded,
		Objects:         0,
		Traits:          0,
		References:      0,
	}
	recoveryComplete := plan.recoveryComplete

	var sink indexSink
	if prep.dryRun {
		sink = &countingSink{
			db:           prep.db,
			incremental:  prep.incremental,
			removedFiles: uniqueStrings(plan.deletedFiles, plan.excludedFiles),
			fileStats:    make(map[string]index.IndexStats),
			result:       result,
		}
	} else {
		sink = &writerSink{db: prep.db, schema: prep.schema, result: result}
	}

	walkOpts := &vault.WalkOptions{ParseOptions: prep.parseOptions, ExcludeMatcher: prep.excludeMatcher}
	if prep.incremental {
		walkOpts.ShouldParse = plan.shouldParse
	}
	walkErr := vault.WalkMarkdownFilesWithOptions(prep.vaultPath, walkOpts, func(walkResult vault.WalkResult) error {
		select {
		case <-prep.ctx.Done():
			return prep.ctx.Err()
		default:
		}

		if walkResult.ParseSkipped {
			result.FilesSkipped++
			return nil
		}

		if walkResult.Error != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", walkResult.RelativePath, walkResult.Error))
			return nil //nolint:nilerr // keep walking to collect all per-file errors
		}

		if prep.incremental {
			result.StaleFiles = append(result.StaleFiles, walkResult.RelativePath)
		}

		if !sink.indexParsed(walkResult) {
			return nil
		}

		if prep.incremental && !prep.dryRun {
			if clearErr := indexjournal.ClearRecoveredPath(prep.vaultPath, plan.pending, walkResult.RelativePath); clearErr != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear recovered index path %s: %v", walkResult.RelativePath, clearErr))
				recoveryComplete = false
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, fmt.Sprintf("error walking vault: %v", walkErr), walkErr)
	}

	if prep.incremental && !prep.dryRun {
		for _, relPath := range plan.pending.Paths() {
			if paths.HasMDExtension(relPath) {
				continue
			}
			if clearErr := indexjournal.ClearRecoveredPath(prep.vaultPath, plan.pending, relPath); clearErr != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear unindexed file path %s: %v", relPath, clearErr))
				recoveryComplete = false
			}
		}
	}

	if !prep.dryRun && !prep.incremental && len(result.Errors) > 0 {
		err := fmt.Errorf("%d file(s) failed during full reindex; first failure: %s", len(result.Errors), result.Errors[0])
		return nil, svcerr.Wrap(codes.ErrFileRead, err.Error(), err).WithSuggestion("Fix invalid or unreadable vault files and run 'rvn reindex --full' again")
	}

	if prep.dryRun {
		projected, err := sink.stats()
		if err != nil {
			return nil, wrapDB(err, "failed to project dry-run stats")
		}
		result.Objects = projected.ObjectCount
		result.Traits = projected.TraitCount
		result.References = projected.RefCount
		if rev, revErr := prep.db.Revision(); revErr != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to read index revision: %v", revErr))
		} else {
			result.Revision = rev
		}
		return result, nil
	}

	// Run the resolve pass even when no files were reindexed: earlier
	// writes may have added targets for refs that are still recorded as
	// unresolved, and the pass short-circuits cheaply when nothing is
	// unresolved.
	refResult, refErr := prep.db.ResolveReferencesWithSchema(prep.dailyDir, prep.schema)
	if refErr != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to resolve references: %v", refErr))
	} else if refResult != nil {
		result.RefsResolved = refResult.Resolved
		result.RefsUnresolved = refResult.Unresolved
		result.HasRefResult = true
	}

	if result.FilesIndexed > 0 {
		if analyzeErr := prep.db.Analyze(); analyzeErr != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to analyze database: %v", analyzeErr))
		}
	}

	stats, err := sink.stats()
	if err != nil {
		return nil, wrapDB(err, "failed to get stats")
	}
	result.Objects = stats.ObjectCount
	result.Traits = stats.TraitCount
	result.References = stats.RefCount

	if prep.rebuildSession != nil {
		if err := prep.rebuildSession.Complete(); err != nil {
			return nil, wrapDB(err, "failed to complete index rebuild").WithSuggestion("Run 'rvn reindex --full' to rebuild the database")
		}
	}
	switch {
	case !prep.incremental:
		if err := indexjournal.CompleteRecoveredSnapshot(prep.vaultPath, plan.pending); err != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear recovered index journal: %v", err))
		}
	case plan.forceFullScan && recoveryComplete && len(result.Errors) == 0:
		if err := indexjournal.CompleteRecoveredUnknown(prep.vaultPath, plan.pending); err != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear recovered index journal: %v", err))
		}
	}

	if rev, revErr := prep.db.Revision(); revErr != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to read index revision: %v", revErr))
	} else {
		result.Revision = rev
	}

	return result, nil
}

func parsedDocumentStats(doc *parser.ParsedDocument) index.IndexStats {
	if doc == nil {
		return index.IndexStats{}
	}
	return index.IndexStats{
		ObjectCount: len(doc.Objects),
		TraitCount:  len(doc.Traits),
		RefCount:    len(doc.Refs),
	}
}
