package commandimpl

import (
	"context"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleWrite executes the canonical `write` command (create-or-replace).
func HandleWrite(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	vaultCfg := sess.Runtime().VaultCfg

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

	writeReq := objectsvc.WriteRequest{
		TypeName:    typeName,
		Title:       title,
		TargetPath:  targetPath,
		ReplaceBody: hasContent,
		Content:     content,
		FieldValues: allFieldValues,
	}
	result, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.WriteResult, mutation.ChangeSet, error) {
		result, err := objectsvc.Write(rt, writeReq)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return result, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}

	warnings := warningMessagesToCommandWarnings(result.WarningMessages, codes.WarnUnknownField)
	data := commandpayload.WriteResult{
		Status: result.Status,
		ObjectMutation: commandpayload.ObjectMutation{
			ID:    vaultCfg.FilePathToObjectID(result.RelativePath),
			File:  result.RelativePath,
			Type:  typeName,
			Title: title,
		},
		MissingReferences: missingRefsOf(proj),
	}
	warnings = appendCommandWarnings(warnings, proj.Warnings)

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
