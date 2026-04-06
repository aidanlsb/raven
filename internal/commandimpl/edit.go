package commandimpl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/editsvc"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/readsvc"
)

// HandleEdit executes the canonical `edit` command.
func HandleEdit(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	reference := strings.TrimSpace(stringArg(req.Args, "path"))
	if reference == "" {
		reference = strings.TrimSpace(stringArg(req.Args, "reference"))
	}
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference argument", nil, "Usage: rvn edit <reference> <old_str> <new_str> or --edits-json")
	}

	edits, batchMode, err := parseCanonicalEditInput(req.Args)
	if err != nil {
		return mapEditFailure(err)
	}

	rt := &readsvc.Runtime{
		VaultPath: vaultPath,
		VaultCfg:  vaultCfg,
	}
	resolved, err := readsvc.ResolveReference(reference, rt, false)
	if err != nil {
		return mapResolveFailure(err, reference)
	}

	if validation := validateEditableContentPath(vaultPath, vaultCfg, resolved.FilePath); validation != nil {
		return *validation
	}

	content, err := os.ReadFile(resolved.FilePath)
	if err != nil {
		return commandexec.Failure("FILE_READ_ERROR", err.Error(), nil, "")
	}

	relPath, _ := filepath.Rel(vaultPath, resolved.FilePath)
	newContent, results, err := editsvc.ApplyEditsInMemory(string(content), relPath, edits)
	if err != nil {
		return mapEditFailure(err)
	}
	if len(results) == 0 {
		return commandexec.Failure("INVALID_INPUT", "no edits provided", nil, "Provide at least one edit")
	}

	if !(req.Confirm || boolArg(req.Args, "confirm")) {
		if batchMode {
			editsPreview := make([]map[string]interface{}, 0, len(results))
			for _, result := range results {
				editsPreview = append(editsPreview, map[string]interface{}{
					"index":   result.Index,
					"line":    result.Line,
					"old_str": result.OldStr,
					"new_str": result.NewStr,
					"preview": map[string]string{
						"before": result.Before,
						"after":  result.After,
					},
				})
			}
			return commandexec.Success(map[string]interface{}{
				"status": "preview",
				"path":   relPath,
				"count":  len(editsPreview),
				"edits":  editsPreview,
			}, nil)
		}

		result := results[0]
		return commandexec.Success(map[string]interface{}{
			"status": "preview",
			"path":   relPath,
			"line":   result.Line,
			"preview": map[string]string{
				"before": result.Before,
				"after":  result.After,
			},
		}, nil)
	}

	if err := atomicfile.WriteFile(resolved.FilePath, []byte(newContent), 0o644); err != nil {
		return commandexec.Failure("FILE_WRITE_ERROR", err.Error(), nil, "")
	}
	autoReindexFile(vaultPath, resolved.FilePath, vaultCfg)

	if batchMode {
		applied := make([]map[string]interface{}, 0, len(results))
		for _, result := range results {
			applied = append(applied, map[string]interface{}{
				"index":   result.Index,
				"line":    result.Line,
				"old_str": result.OldStr,
				"new_str": result.NewStr,
				"context": result.Context,
			})
		}
		return commandexec.Success(map[string]interface{}{
			"status": "applied",
			"path":   relPath,
			"count":  len(applied),
			"edits":  applied,
		}, nil)
	}

	result := results[0]
	return commandexec.Success(map[string]interface{}{
		"status":  "applied",
		"path":    relPath,
		"line":    result.Line,
		"old_str": result.OldStr,
		"new_str": result.NewStr,
		"context": result.Context,
	}, nil)
}

func parseCanonicalEditInput(args map[string]any) ([]editsvc.EditSpec, bool, error) {
	if raw, ok := args["edits-json"]; ok {
		var payload string
		switch v := raw.(type) {
		case string:
			payload = v
		default:
			encoded, err := json.Marshal(v)
			if err != nil {
				return nil, false, &editsvc.Error{
					Code:       editsvc.CodeInvalidInput,
					Message:    "invalid --edits-json payload",
					Suggestion: `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`,
					Details:    map[string]string{"error": err.Error()},
					Err:        err,
				}
			}
			payload = string(encoded)
		}

		edits, err := editsvc.ParseEditsJSON(strings.TrimSpace(payload))
		if err != nil {
			return nil, false, err
		}
		return edits, true, nil
	}

	oldStr := stringArg(args, "old_str")
	newStr, hasNew := args["new_str"]
	if oldStr == "" || !hasNew {
		return nil, false, &editsvc.Error{
			Code:       editsvc.CodeInvalidInput,
			Message:    "requires old_str and new_str when --edits-json is not provided",
			Suggestion: "Usage: rvn edit <reference> <old_str> <new_str> or --edits-json",
		}
	}

	return []editsvc.EditSpec{{
		OldStr: oldStr,
		NewStr: toAnyString(newStr),
	}}, false, nil
}

func mapEditFailure(err error) commandexec.Result {
	if svcErr, ok := editsvc.AsError(err); ok {
		var details map[string]interface{}
		if len(svcErr.Details) > 0 {
			details = make(map[string]interface{}, len(svcErr.Details))
			for key, value := range svcErr.Details {
				details[key] = value
			}
		}
		return commandexec.Failure(string(svcErr.Code), svcErr.Message, details, svcErr.Suggestion)
	}
	return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
}

func toAnyString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func validateEditableContentPath(vaultPath string, vaultCfg *config.VaultConfig, filePath string) *commandexec.Result {
	relPath, err := filepath.Rel(vaultPath, filePath)
	if err != nil {
		result := commandexec.Failure("VALIDATION_FAILED", "edit only supports vault content files", nil, "Use edit for markdown content files inside the vault")
		return &result
	}

	relPath = paths.NormalizeVaultRelPath(relPath)
	templateDir := ""
	protectedPrefixes := []string(nil)
	if vaultCfg != nil {
		templateDir = vaultCfg.GetTemplateDirectory()
		protectedPrefixes = vaultCfg.ProtectedPrefixes
	}

	if paths.IsProtectedRelPath(relPath, protectedPrefixes) {
		suggestion := "Use the dedicated Raven command for this protected path"
		switch relPath {
		case "raven.yaml":
			suggestion = "Use 'rvn vault config ...' or 'rvn query saved ...' to mutate raven.yaml"
		case "schema.yaml":
			suggestion = "Use 'rvn schema ...' to mutate schema.yaml"
		}
		result := commandexec.Failure("VALIDATION_FAILED", "cannot edit protected or system-managed paths", map[string]interface{}{"path": relPath}, suggestion)
		return &result
	}

	if templateDir != "" && strings.HasPrefix(relPath, templateDir) {
		result := commandexec.Failure("VALIDATION_FAILED", "edit only supports vault content files; template files are managed separately", map[string]interface{}{"path": relPath}, "Use 'rvn template write' or 'rvn template delete' for template lifecycle changes")
		return &result
	}

	if !paths.HasMDExtension(relPath) {
		result := commandexec.Failure("VALIDATION_FAILED", "edit only supports markdown content files", map[string]interface{}{"path": relPath}, "Use dedicated Raven commands for vault config, schema, templates, and other non-content files")
		return &result
	}

	return nil
}
