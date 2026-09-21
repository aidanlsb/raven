package checksvc

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/parseopts"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type scopedWalk struct {
	docs        []*parser.ParsedDocument
	objectInfos []check.ObjectInfo
	parseErrors []check.Issue
	fileCount   int
	walkPath    string
	targetFiles map[string]bool
}

func walkScopedDocuments(rt *vaultruntime.Runtime, scope *Scope, excludeMatcher *ravenignore.Matcher) (scopedWalk, error) {
	setup := prepareScopeWalkSetup(rt.VaultPath, scope)
	walked := scopedWalk{
		walkPath:    setup.walkPath,
		targetFiles: setup.targetFileSet,
	}

	walkOpts := &vault.WalkOptions{
		ParseOptions:   parseopts.FromVaultConfig(rt.VaultCfg),
		ExcludeMatcher: excludeMatcher,
	}
	walkErr := vault.WalkMarkdownFilesWithOptions(rt.VaultPath, walkOpts, func(walkResult vault.WalkResult) error {
		if walkResult.Error != nil {
			if isFileInScope(walkResult.Path, scope, walked.walkPath, walked.targetFiles) {
				walked.fileCount++
				walked.parseErrors = append(walked.parseErrors, check.Issue{
					Level:    check.LevelError,
					Type:     check.IssueParseError,
					FilePath: walkResult.RelativePath,
					Line:     1,
					Message:  walkResult.Error.Error(),
					FixHint:  "Fix the YAML frontmatter or markdown syntax",
				})
			}
			return nil
		}

		for _, obj := range walkResult.Document.Objects {
			walked.objectInfos = append(walked.objectInfos, check.ObjectInfo{ID: obj.ID, Type: obj.Type})
		}

		if isFileInScope(walkResult.Path, scope, walked.walkPath, walked.targetFiles) {
			walked.fileCount++
			walked.docs = append(walked.docs, walkResult.Document)
		}

		return nil
	})
	if walkErr != nil {
		return scopedWalk{}, svcerr.ValidationError(fmt.Errorf("error walking vault: %w", walkErr))
	}
	return walked, nil
}

type scopeWalkSetup struct {
	walkPath      string
	targetFileSet map[string]bool
}

func prepareScopeWalkSetup(vaultPath string, scope *Scope) scopeWalkSetup {
	setup := scopeWalkSetup{
		walkPath:      vaultPath,
		targetFileSet: make(map[string]bool),
	}
	switch scope.Type {
	case "file":
		for _, f := range scope.targetFiles {
			setup.targetFileSet[f] = true
		}
	case "directory":
		setup.walkPath = filepath.Join(vaultPath, scope.Value)
	}
	return setup
}

func isFileInScope(filePath string, scope *Scope, walkPath string, targetFileSet map[string]bool) bool {
	switch scope.Type {
	case "file":
		return targetFileSet[filePath]
	case "directory":
		rel, err := filepath.Rel(walkPath, filePath)
		if err != nil || filepath.IsAbs(rel) {
			return false
		}
		return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	default:
		return true
	}
}
