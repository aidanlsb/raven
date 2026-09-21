package schemamigratesvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func planTypeDirectoryMove(
	relPath, newName string,
	plan *typeDefaultPathRenamePlan,
	vaultCfg *config.VaultConfig,
) (typeDirectoryMove, bool) {
	if plan == nil || vaultCfg == nil {
		return typeDirectoryMove{}, false
	}
	sourceRel := paths.NormalizeVaultRelPath(relPath)
	sourceID := vaultCfg.FilePathToObjectID(sourceRel)
	if !strings.HasPrefix(sourceID, plan.OldDefaultPath) {
		return typeDirectoryMove{}, false
	}
	suffix := strings.TrimPrefix(sourceID, plan.OldDefaultPath)
	if suffix == "" {
		return typeDirectoryMove{}, false
	}
	destinationID := plan.NewDefaultPath + suffix
	destinationRel := filepath.ToSlash(vaultCfg.ObjectIDToFilePath(destinationID, newName))
	if sourceRel == destinationRel {
		return typeDirectoryMove{}, false
	}
	return typeDirectoryMove{
		SourceRelPath:      sourceRel,
		DestinationRelPath: destinationRel,
		SourceID:           sourceID,
		DestinationID:      destinationID,
	}, true
}

func validateTypeDirectoryMoves(vaultPath string, moves []typeDirectoryMove) error {
	if len(moves) == 0 {
		return nil
	}

	destinations := make(map[string]string, len(moves))
	sources := make(map[string]struct{}, len(moves))
	for _, move := range moves {
		sourceAbs := filepath.Join(vaultPath, move.SourceRelPath)
		destinationAbs := filepath.Join(vaultPath, move.DestinationRelPath)
		sources[filepath.Clean(sourceAbs)] = struct{}{}

		if _, err := os.Stat(sourceAbs); err != nil {
			return fmt.Errorf("source file does not exist: %s", move.SourceRelPath)
		}
		if existingSource, exists := destinations[filepath.Clean(destinationAbs)]; exists && existingSource != move.SourceRelPath {
			return fmt.Errorf("multiple files would move to '%s'", move.DestinationRelPath)
		}
		destinations[filepath.Clean(destinationAbs)] = move.SourceRelPath
	}

	for _, move := range moves {
		destinationAbs := filepath.Clean(filepath.Join(vaultPath, move.DestinationRelPath))
		if _, isSource := sources[destinationAbs]; isSource {
			continue
		}
		if _, err := os.Stat(destinationAbs); err == nil {
			return fmt.Errorf("destination already exists: %s", move.DestinationRelPath)
		}
	}
	return nil
}

func applyTypeDirectoryMoves(vaultPath string, moves []typeDirectoryMove) (int, error) {
	orderedMoves := append([]typeDirectoryMove(nil), moves...)
	sort.SliceStable(orderedMoves, func(i, j int) bool {
		return len(orderedMoves[i].SourceRelPath) > len(orderedMoves[j].SourceRelPath)
	})
	for _, move := range orderedMoves {
		sourceAbs := filepath.Join(vaultPath, move.SourceRelPath)
		destinationAbs := filepath.Join(vaultPath, move.DestinationRelPath)
		if err := os.MkdirAll(filepath.Dir(destinationAbs), 0o755); err != nil {
			return 0, err
		}
		if err := os.Rename(sourceAbs, destinationAbs); err != nil {
			return 0, err
		}
	}
	return len(orderedMoves), nil
}

func wrapRenameWriteError(err error) error {
	return svcerr.Wrap(codes.ErrFileWrite, err.Error(), err).WithSuggestion("Some files may have been renamed; review the vault and run 'rvn reindex --full'")
}

func hintForTypeApply(hasDefaultPathPlan, applied bool) string {
	if hasDefaultPathPlan && !applied {
		return "Run 'rvn reindex --full' to update the index. Use --rename-default-path to also rename the default directory."
	}
	return "Run 'rvn reindex --full' to update the index"
}

func defaultPathValue(plan *typeDefaultPathRenamePlan, old bool) string {
	if plan == nil {
		return ""
	}
	if old {
		return plan.OldDefaultPath
	}
	return plan.NewDefaultPath
}
