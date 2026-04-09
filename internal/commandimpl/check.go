package commandimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
)

const checkApplyIncompleteWarningCode = "CHECK_APPLY_INCOMPLETE"

// HandleCheck executes the canonical `check` command.
func HandleCheck(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}
	if boolArg(req.Args, "fix") && boolArg(req.Args, "create-missing") {
		return commandexec.Failure("INVALID_INPUT", "cannot combine --fix with --create-missing", nil, "Use one action at a time")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

	result, err := checksvc.Run(vaultPath, vaultCfg, sch, checksvc.Options{
		PathArg:     strings.TrimSpace(stringArg(req.Args, "path")),
		TypeFilter:  strings.TrimSpace(stringArg(req.Args, "type")),
		TraitFilter: strings.TrimSpace(stringArg(req.Args, "trait")),
		Issues:      strings.TrimSpace(stringArg(req.Args, "issues")),
		Exclude:     strings.TrimSpace(stringArg(req.Args, "exclude")),
		ErrorsOnly:  boolArg(req.Args, "errors-only"),
	})
	if err != nil {
		return commandexec.Failure("VALIDATION_FAILED", err.Error(), nil, "")
	}

	switch {
	case boolArg(req.Args, "fix"):
		return handleCheckFix(vaultPath, sch, result, req.Confirm || boolArg(req.Args, "confirm"))
	case boolArg(req.Args, "create-missing"):
		return handleCheckCreateMissing(vaultPath, vaultCfg, sch, result, req.Confirm || boolArg(req.Args, "confirm"))
	default:
		data, convErr := structToMap(checksvc.BuildJSON(vaultPath, result))
		if convErr != nil {
			return commandexec.Failure("INTERNAL_ERROR", "failed to build check response", nil, "")
		}
		return commandexec.Success(data, nil)
	}
}

// HandleCheckFix executes the canonical `check_fix` command.
func HandleCheckFix(ctx context.Context, req commandexec.Request) commandexec.Result {
	req.Args = withBoolArg(req.Args, "fix")
	return HandleCheck(ctx, req)
}

// HandleCheckCreateMissing executes the canonical `check create-missing` command.
func HandleCheckCreateMissing(ctx context.Context, req commandexec.Request) commandexec.Result {
	req.Args = withBoolArg(req.Args, "create-missing")
	return HandleCheck(ctx, req)
}

func handleCheckFix(vaultPath string, sch *schema.Schema, result *checksvc.RunResult, confirm bool) commandexec.Result {
	fixes := checksvc.CollectFixableIssues(result.Issues, result.ShortRefs, sch)
	grouped := checksvc.GroupFixesByFile(fixes)

	if !confirm {
		return commandexec.Success(map[string]interface{}{
			"preview":        true,
			"fixable_issues": len(fixes),
			"files":          grouped,
			"scope":          checkScopeData(result),
			"file_count":     result.FileCount,
			"error_count":    result.ErrorCount,
			"warning_count":  result.WarningCount,
		}, nil)
	}

	applied, err := checksvc.ApplyFixes(vaultPath, fixes)
	if err != nil {
		return commandexec.Failure("VALIDATION_FAILED", err.Error(), nil, "")
	}

	data := map[string]interface{}{
		"preview":        false,
		"ok":             len(applied.Skipped) == 0,
		"fixable_issues": len(fixes),
		"fixed_issues":   applied.IssueCount,
		"fixed_files":    applied.FileCount,
		"skipped_issues": len(applied.Skipped),
		"skipped_items":  applied.Skipped,
		"scope":          checkScopeData(result),
		"file_count":     result.FileCount,
		"error_count":    result.ErrorCount,
		"warning_count":  result.WarningCount,
	}
	if len(applied.Skipped) > 0 {
		return commandexec.SuccessWithWarnings(data, []commandexec.Warning{
			{
				Code: checkApplyIncompleteWarningCode,
				Message: fmt.Sprintf(
					"Applied %d of %d planned fixes; %d fix(es) were skipped because the expected content was no longer present.",
					applied.IssueCount,
					len(fixes),
					len(applied.Skipped),
				),
			},
		}, nil)
	}
	return commandexec.Success(data, nil)
}

func handleCheckCreateMissing(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, result *checksvc.RunResult, confirm bool) commandexec.Result {
	if result.Scope.Type != "full" {
		return commandexec.Failure("INVALID_INPUT", "check create-missing only supports full-vault scope", nil, "Run without path/--type/--trait filters")
	}

	data := map[string]interface{}{
		"preview":               !confirm,
		"missing_refs":          len(result.MissingRefs),
		"undefined_traits":      len(result.UndefinedTraits),
		"requires_confirm":      true,
		"non_interactive_only":  true,
		"scope":                 checkScopeData(result),
		"missing_ref_items":     result.MissingRefs,
		"undefined_trait_items": result.UndefinedTraits,
		"file_count":            result.FileCount,
		"error_count":           result.ErrorCount,
		"warning_count":         result.WarningCount,
	}

	if !confirm {
		return commandexec.Success(data, nil)
	}

	created := checksvc.CreateMissingRefsNonInteractive(
		vaultPath,
		sch,
		result.MissingRefs,
		vaultCfg.GetObjectsRoot(),
		vaultCfg.GetPagesRoot(),
		vaultCfg.GetTemplateDirectory(),
		vaultCfg.ProtectedPrefixes,
	)
	data["preview"] = false
	data["ok"] = len(created.Failures) == 0
	data["created_pages"] = created.Created
	data["failed_pages"] = len(created.Failures)
	data["failed_page_items"] = created.Failures
	data["undefined_traits_note"] = "undefined traits are interactive-only and were not changed in JSON mode"
	if len(created.Failures) > 0 {
		return commandexec.SuccessWithWarnings(data, []commandexec.Warning{
			{
				Code: checkApplyIncompleteWarningCode,
				Message: fmt.Sprintf(
					"Created %d of %d missing page(s); %d page(s) failed to create.",
					created.Created,
					len(result.MissingRefs),
					len(created.Failures),
				),
			},
		}, nil)
	}
	return commandexec.Success(data, nil)
}

func structToMap(value any) (map[string]interface{}, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(encoded, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func checkScopeData(result *checksvc.RunResult) map[string]interface{} {
	if result == nil {
		return nil
	}
	return map[string]interface{}{
		"type":  result.Scope.Type,
		"value": result.Scope.Value,
	}
}

func withBoolArg(args map[string]interface{}, key string) map[string]interface{} {
	out := make(map[string]interface{}, len(args)+1)
	for k, v := range args {
		out[k] = v
	}
	out[key] = true
	return out
}
