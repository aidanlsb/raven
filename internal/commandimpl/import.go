package commandimpl

import (
	"context"
	"io"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/importsvc"
)

// HandleImport executes the canonical `import` command.
func HandleImport(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	mappingCfg, err := importsvc.BuildMappingConfig(importsvc.BuildMappingConfigRequest{
		MappingFilePath: strings.TrimSpace(stringArg(req.Args, "mapping")),
		CLIType:         strings.TrimSpace(stringArg(req.Args, "type")),
		MapFlags:        stringSliceArg(req.Args["map"]),
		Key:             strings.TrimSpace(stringArg(req.Args, "key")),
		ContentField:    strings.TrimSpace(stringArg(req.Args, "content-field")),
	})
	if err != nil {
		return mapImportFailure(err, "")
	}

	items, err := importsvc.ReadJSONInput(strings.TrimSpace(stringArg(req.Args, "file")), stdinReader(req.Stdin))
	if err != nil {
		return mapImportFailure(err, "Expected a JSON array of objects or a single JSON object")
	}
	if len(items) == 0 {
		return commandexec.Failure("INVALID_INPUT", "no items to import", nil, "Provide a non-empty JSON array")
	}

	serviceResult, err := importsvc.Run(importsvc.RunRequest{
		VaultPath:     vaultPath,
		MappingConfig: mappingCfg,
		Items:         items,
		DryRun:        boolArg(req.Args, "dry-run"),
		CreateOnly:    boolArg(req.Args, "create-only"),
		UpdateOnly:    boolArg(req.Args, "update-only"),
	})
	if err != nil {
		return mapImportFailure(err, "")
	}

	var created, updated, skipped, errored int
	for _, item := range serviceResult.Results {
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

	if !boolArg(req.Args, "dry-run") {
		reindexed := make(map[string]struct{}, len(serviceResult.ChangedFilePaths))
		var reindexWarnings []commandexec.Warning
		for _, changedFile := range serviceResult.ChangedFilePaths {
			if changedFile == "" {
				continue
			}
			if _, seen := reindexed[changedFile]; seen {
				continue
			}
			reindexed[changedFile] = struct{}{}
			reindexWarnings = appendCommandWarnings(
				reindexWarnings,
				autoReindexWarnings(vaultPath, serviceResult.VaultConfig, changedFile),
			)
		}
		serviceWarnings := warningMessagesToCommandWarnings(serviceResult.WarningMessages, "UNKNOWN_FIELD")
		return commandexec.SuccessWithWarnings(map[string]interface{}{
			"total":   len(serviceResult.Results),
			"created": created,
			"updated": updated,
			"skipped": skipped,
			"errors":  errored,
			"results": serviceResult.Results,
		}, appendCommandWarnings(serviceWarnings, reindexWarnings), nil)
	}

	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"total":   len(serviceResult.Results),
		"created": created,
		"updated": updated,
		"skipped": skipped,
		"errors":  errored,
		"results": serviceResult.Results,
	}, warningMessagesToCommandWarnings(serviceResult.WarningMessages, "UNKNOWN_FIELD"), nil)
}

func mapImportFailure(err error, fallbackSuggestion string) commandexec.Result {
	svcErr, ok := importsvc.AsError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, fallbackSuggestion)
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

	return commandexec.Failure(string(svcErr.Code), svcErr.Error(), nil, suggestion)
}

func stdinReader(stdin []byte) io.Reader {
	return strings.NewReader(string(stdin))
}
