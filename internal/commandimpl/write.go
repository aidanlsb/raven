package commandimpl

import (
	"context"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/objectsvc"
)

// HandleWrite executes the canonical `write` command (create-or-replace).
func HandleWrite(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg

	typeName := strings.TrimSpace(stringArg(req.Args, "type"))
	title := strings.TrimSpace(stringArg(req.Args, "title"))
	// Leave targetPath empty when no explicit --object-path is given so the service
	// derives the filename/slug from the title (which may contain "/").
	targetPath := strings.TrimSpace(stringArg(req.Args, "object-path"))

	fieldValues, err := parseKeyValueArgs(req.Args["field"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --field payload", nil, err.Error())
	}

	typedFieldValues, err := parseTypedFieldValues(req.Args["fields-json"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --fields-json payload", nil, "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}
	allFieldValues := mergeFieldInputs(fieldValues, typedFieldValues)

	content, hasContent, contentErr := writeBodyContent(req)
	if contentErr != nil {
		return *contentErr
	}

	result, err := objectsvc.Write(rt, objectsvc.WriteRequest{
		TypeName:    typeName,
		Title:       title,
		TargetPath:  targetPath,
		ReplaceBody: hasContent,
		Content:     content,
		FieldValues: allFieldValues,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := warningMessagesToCommandWarnings(result.WarningMessages, codes.WarnUnknownField)
	missingRefs, postWarnings := applyChangeSet(rt, result.ChangeSet, req.IndexJournalOperation)
	data := commandpayload.WriteResult{
		Status: result.Status,
		ObjectMutation: commandpayload.ObjectMutation{
			ID:    vaultCfg.FilePathToObjectID(result.RelativePath),
			File:  result.RelativePath,
			Type:  typeName,
			Title: title,
		},
		MissingReferences: missingRefs,
	}
	warnings = appendCommandWarnings(warnings, postWarnings)

	return commandexec.SuccessWithWarnings(
		data,
		warnings,
		nil,
	)
}

func writeBodyContent(req commandexec.Request) (string, bool, *commandexec.Result) {
	_, hasContent := req.Args["content"]
	content := stringArg(req.Args, "content")

	_, hasContentFile := req.Args["content-file"]
	contentFile := strings.TrimSpace(stringArg(req.Args, "content-file"))
	if hasContent && hasContentFile {
		result := commandexec.Failure(
			codes.ErrInvalidInput,
			"--content and --content-file are mutually exclusive",
			nil,
			"Use only one body input mode",
		)
		return "", false, &result
	}
	if !hasContentFile {
		return content, hasContent, nil
	}
	if contentFile == "" {
		result := commandexec.Failure(
			codes.ErrInvalidInput,
			"--content-file requires a path or '-'",
			nil,
			"Provide a file path or use --content-file - to read from stdin",
		)
		return "", false, &result
	}
	if contentFile == "-" {
		return string(req.Stdin), true, nil
	}

	data, err := os.ReadFile(contentFile)
	if err != nil {
		result := commandexec.Failure(
			codes.ErrFileRead,
			"failed to read --content-file",
			map[string]interface{}{"path": contentFile},
			err.Error(),
		)
		return "", false, &result
	}
	return string(data), true, nil
}
