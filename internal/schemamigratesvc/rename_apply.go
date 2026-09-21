package schemamigratesvc

import (
	"path/filepath"

	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func applyFieldRenamePlan(rt *vaultruntime.Runtime, plan *fieldRenamePlan) (int, error) {
	fileSets := []map[string][]byte{plan.TemplateFiles}
	if len(plan.RavenYAML) > 0 {
		fileSets = append(fileSets, map[string][]byte{
			filepath.Join(rt.VaultPath, "raven.yaml"): plan.RavenYAML,
		})
	}
	fileSets = append(fileSets, plan.MarkdownFiles)

	schemaApplied := 0
	if len(plan.SchemaYAML) > 0 {
		schemaApplied = 1
	}
	return applyStagedFilesThenInvalidate(rt, stagedApply{
		SchemaYAML:    plan.SchemaYAML,
		SchemaApplied: schemaApplied,
		FileSets:      fileSets,
		ReloadContext: "rename",
	})
}

func applyTypeRenamePlan(rt *vaultruntime.Runtime, plan *typeRenamePlan, applyDefaultPathRename bool) (int, int, int, error) {
	schemaBytes := plan.SchemaPlan.SchemaYAML
	schemaApplied := plan.SchemaPlan.CoreSchemaMutations
	if applyDefaultPathRename && plan.SchemaPlan.SchemaYAMLWithDefaultPath != nil {
		schemaBytes = plan.SchemaPlan.SchemaYAMLWithDefaultPath
		if plan.SchemaPlan.DefaultPathMutation {
			schemaApplied++
		}
	}

	movedFiles := 0
	referenceFilesUpdated := 0
	applied, err := applyStagedFilesThenInvalidate(rt, stagedApply{
		SchemaYAML:    schemaBytes,
		SchemaApplied: schemaApplied,
		FileSets:      []map[string][]byte{plan.MarkdownFiles},
		ReloadContext: "rename",
		AfterFiles: func() (int, error) {
			if !applyDefaultPathRename {
				return 0, nil
			}
			moved, moveErr := applyTypeDirectoryMoves(rt.VaultPath, plan.DefaultPathPlan.Moves)
			if moveErr != nil {
				return 0, wrapRenameWriteError(moveErr)
			}
			movedFiles = moved
			n, writeErr := writeStagedFileMaps(
				"Some files may have been renamed; review the vault and run 'rvn reindex --full'",
				plan.ReferenceFiles,
			)
			if writeErr != nil {
				return 0, writeErr
			}
			referenceFilesUpdated = n
			return moved + n, nil
		},
	})
	if err != nil {
		return 0, 0, 0, err
	}
	return applied, movedFiles, referenceFilesUpdated, nil
}
