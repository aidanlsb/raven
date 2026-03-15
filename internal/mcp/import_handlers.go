package mcp

import (
	"io"
	"strings"

	"github.com/aidanlsb/raven/internal/importsvc"
)

func mapImportSvcError(err error, fallbackSuggestion string) (string, bool) {
	svcErr, ok := importsvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), fallbackSuggestion, nil), true
	}

	suggestion := fallbackSuggestion
	switch svcErr.Code {
	case importsvc.CodeInvalidInput:
		if suggestion == "" {
			suggestion = "Provide valid JSON input and mapping options"
		}
	case importsvc.CodeTypeNotFound:
		suggestion = "Check schema.yaml for available types"
	case importsvc.CodeSchemaInvalid:
		suggestion = "Fix schema.yaml and try again"
	case importsvc.CodeConfigInvalid:
		suggestion = "Fix raven.yaml and try again"
	}

	return errorEnvelope(string(svcErr.Code), svcErr.Error(), suggestion, nil), true
}

func (s *Server) callDirectImport(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)

	typeName := strings.TrimSpace(toString(normalized["type"]))
	mappingFile := strings.TrimSpace(toString(normalized["mapping"]))
	mapFlags := keyValuePairs(normalized["map"])
	keyField := strings.TrimSpace(toString(normalized["key"]))
	contentField := strings.TrimSpace(toString(normalized["content-field"]))
	filePath := strings.TrimSpace(toString(normalized["file"]))
	dryRun := boolValue(normalized["dry-run"])
	createOnly := boolValue(normalized["create-only"])
	updateOnly := boolValue(normalized["update-only"])

	mappingCfg, err := importsvc.BuildMappingConfig(importsvc.BuildMappingConfigRequest{
		MappingFilePath: mappingFile,
		CLIType:         typeName,
		MapFlags:        mapFlags,
		Key:             keyField,
		ContentField:    contentField,
	})
	if err != nil {
		return mapImportSvcError(err, "")
	}

	items, err := importsvc.ReadJSONInput(filePath, io.Reader(strings.NewReader("")))
	if err != nil {
		suggestion := "Provide --file with JSON input (stdin is not available in MCP tools)"
		return mapImportSvcError(err, suggestion)
	}
	if len(items) == 0 {
		return errorEnvelope("INVALID_INPUT", "no items to import", "Provide a non-empty JSON array or object", nil), true
	}

	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("CONFIG_INVALID", err.Error(), "Configure a vault path in raven config or set --vault-path", nil), true
	}
	result, err := importsvc.Run(importsvc.RunRequest{
		VaultPath:     vaultPath,
		MappingConfig: mappingCfg,
		Items:         items,
		DryRun:        dryRun,
		CreateOnly:    createOnly,
		UpdateOnly:    updateOnly,
	})
	if err != nil {
		return mapImportSvcError(err, "")
	}

	if !dryRun {
		reindexed := make(map[string]struct{}, len(result.ChangedFilePaths))
		for _, changedFile := range result.ChangedFilePaths {
			if changedFile == "" {
				continue
			}
			if _, seen := reindexed[changedFile]; seen {
				continue
			}
			reindexed[changedFile] = struct{}{}
			maybeDirectReindexFile(vaultPath, changedFile, result.VaultConfig)
		}
	}

	var created, updated, skipped, errored int
	for _, item := range result.Results {
		switch item.Action {
		case "created", "create":
			created++
		case "updated", "update":
			updated++
		case "skipped":
			skipped++
		case "error":
			errored++
		}
	}

	data := map[string]interface{}{
		"total":   len(result.Results),
		"created": created,
		"updated": updated,
		"skipped": skipped,
		"errors":  errored,
		"results": result.Results,
	}

	return successEnvelope(data, warningMessagesToDirectWarnings(result.WarningMessages, "UNKNOWN_FIELD")), false
}
