package commandimpl

import (
	"context"
	"io"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/importsvc"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleImport executes the canonical `import` command.
func HandleImport(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

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

	sess, failure := openSession(vaultPath, vaultsession.SchemaFirst())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()

	runReq := importsvc.RunRequest{
		MappingConfig: mappingCfg,
		Items:         items,
		DryRun:        boolArg(req.Args, "dry-run"),
		CreateOnly:    boolArg(req.Args, "create-only"),
		UpdateOnly:    boolArg(req.Args, "update-only"),
	}
	var (
		serviceResult *importsvc.RunResult
		proj          vaultsession.MutationResult
	)
	if runReq.DryRun {
		var err error
		serviceResult, err = importsvc.Run(rt, runReq)
		if err != nil {
			return mapImportFailure(err, "")
		}
	} else {
		var failure commandexec.Result
		serviceResult, proj, failure = mutateMapped(sess, func(rt *vaultruntime.Runtime) (*importsvc.RunResult, mutation.ChangeSet, error) {
			result, err := importsvc.Run(rt, runReq)
			if err != nil {
				return nil, mutation.ChangeSet{}, err
			}
			return result, result.ChangeSet, nil
		}, func(err error) commandexec.Result { return mapImportFailure(err, "") })
		if failure.Error != nil {
			return failure
		}
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

	data := commandpayload.ImportResult{
		Total:   len(serviceResult.Results),
		Created: created,
		Updated: updated,
		Skipped: skipped,
		Errors:  errored,
		Items:   serviceResult.Results,
	}
	serviceWarnings := warningMessagesToCommandWarnings(serviceResult.WarningMessages, codes.WarnUnknownField)
	if runReq.DryRun {
		return commandexec.SuccessWithWarnings(data, serviceWarnings, nil)
	}
	data.MissingReferences = missingRefsOf(proj)
	return commandexec.SuccessWithWarnings(data, appendCommandWarnings(serviceWarnings, proj.Warnings), nil)
}

func mapImportFailure(err error, fallbackSuggestion string) commandexec.Result {
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		return commandexec.FromServiceErrorWithFallback(err, fallbackSuggestion)
	}

	suggestion := fallbackSuggestion
	switch svcErr.Code {
	case codes.ErrInvalidInput:
		if suggestion == "" {
			suggestion = "Provide valid JSON input and mapping options"
		}
	case codes.ErrTypeNotFound:
		suggestion = "Check schema.yaml for available types"
	case codes.ErrSchemaInvalid:
		suggestion = "Fix schema.yaml and try again"
	case codes.ErrConfigInvalid:
		suggestion = "Fix raven.yaml and try again"
	}

	return commandexec.FromServiceErrorWithFallback(err, suggestion)
}

func stdinReader(stdin []byte) io.Reader {
	return strings.NewReader(string(stdin))
}
