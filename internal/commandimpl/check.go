package commandimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checkfixsvc"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

const checkApplyIncompleteWarningCode = codes.WarnCheckIncomplete

// HandleCheck executes the canonical `check` command.
func HandleCheck(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if boolArg(req.Args, "fix") && boolArg(req.Args, "create-missing") {
		return commandexec.Failure("INVALID_INPUT", "cannot combine --fix with --create-missing", nil, "Use one action at a time")
	}

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	sch := rt.Schema

	result, err := checksvc.Run(rt, checksvc.Options{
		PathArg:     strings.TrimSpace(stringArg(req.Args, "reference")),
		TypeFilter:  strings.TrimSpace(stringArg(req.Args, "type")),
		TraitFilter: strings.TrimSpace(stringArg(req.Args, "trait")),
		Issues:      strings.TrimSpace(stringArg(req.Args, "issues")),
		Exclude:     strings.TrimSpace(stringArg(req.Args, "exclude")),
		ErrorsOnly:  boolArg(req.Args, "errors-only"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	switch {
	case boolArg(req.Args, "fix"):
		return handleCheckFix(rt, vaultCfg, sch, result, req.Confirm, req.IndexJournalOperation)
	case boolArg(req.Args, "create-missing"):
		return handleCheckCreateMissing(vaultPath, vaultCfg, sch, result, req.Confirm)
	default:
		data, convErr := structToMap(checksvc.BuildJSON(vaultPath, result))
		if convErr != nil {
			return commandexec.Failure("INTERNAL_ERROR", "failed to build check response", nil, "")
		}
		if warnings := checkIncompleteWarnings(result); len(warnings) > 0 {
			return commandexec.SuccessWithWarnings(data, warnings, nil)
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

func handleCheckFix(rt *vaultruntime.Runtime, vaultCfg *config.VaultConfig, sch *schema.Schema, result *checksvc.RunResult, confirm bool, journalOperation string) commandexec.Result {
	vaultPath := rt.VaultPath
	fixes := checkfixsvc.CollectFixableIssues(result.Issues, result.ShortRefs, sch, vaultCfg)
	grouped := checkfixsvc.GroupFixesByFile(fixes)

	if !confirm {
		return commandexec.Success(commandpayload.CheckFixPreviewResult{
			Preview:       true,
			FixableIssues: len(fixes),
			Files:         grouped,
			Scope:         checkScopeData(result),
			FileCount:     result.FileCount,
			ErrorCount:    result.ErrorCount,
			WarningCount:  result.WarningCount,
		}, nil)
	}

	applied, err := checkfixsvc.ApplyFixes(vaultPath, fixes, vaultCfg, sch)
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	missingRefs, postWarnings := applyChangeSet(rt, applied.ChangeSet, journalOperation)
	data := commandpayload.CheckFixResult{
		Preview:           false,
		OK:                len(applied.Skipped) == 0,
		FixableIssues:     len(fixes),
		FixedIssues:       applied.IssueCount,
		FixedFiles:        applied.FileCount,
		SkippedIssues:     len(applied.Skipped),
		SkippedItems:      applied.Skipped,
		Scope:             checkScopeData(result),
		FileCount:         result.FileCount,
		ErrorCount:        result.ErrorCount,
		WarningCount:      result.WarningCount,
		MissingReferences: missingRefs,
	}
	if len(applied.Skipped) > 0 {
		postWarnings = appendCommandWarnings(postWarnings, []commandexec.Warning{
			{
				Code: checkApplyIncompleteWarningCode,
				Message: fmt.Sprintf(
					"Applied %d of %d planned fixes; %d fix(es) were skipped because the expected content was no longer present.",
					applied.IssueCount,
					len(fixes),
					len(applied.Skipped),
				),
			},
		})
	}
	return commandexec.SuccessWithWarnings(data, postWarnings, nil)
}

func handleCheckCreateMissing(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, result *checksvc.RunResult, confirm bool) commandexec.Result {
	if result.Scope.Type != "full" {
		return commandexec.Failure("INVALID_INPUT", "check create-missing only supports full-vault scope", nil, "Run without path/--type/--trait filters")
	}

	data := commandpayload.CheckCreateMissingResult{
		Preview:             !confirm,
		MissingRefs:         len(result.MissingRefs),
		UndefinedTraits:     len(result.UndefinedTraits),
		RequiresConfirm:     true,
		NonInteractiveOnly:  true,
		Scope:               checkScopeData(result),
		MissingRefItems:     result.MissingRefs,
		UndefinedTraitItems: result.UndefinedTraits,
		FileCount:           result.FileCount,
		ErrorCount:          result.ErrorCount,
		WarningCount:        result.WarningCount,
	}

	if !confirm {
		return commandexec.Success(data, nil)
	}

	created := checkfixsvc.CreateMissingRefsNonInteractive(
		vaultPath,
		sch,
		result.MissingRefs,
		vaultCfg.GetObjectsRoot(),
		vaultCfg.GetPagesRoot(),
		vaultCfg.GetDailyDirectory(),
		vaultCfg.GetTemplateDirectory(),
		vaultCfg.ProtectedPrefixes,
	)
	data.Preview = false
	ok := len(created.Failures) == 0
	failedPages := len(created.Failures)
	data.OK = &ok
	data.CreatedPages = &created.Created
	data.FailedPages = &failedPages
	data.FailedPageItems = &created.Failures
	data.UndefinedTraitsNote = "undefined traits are interactive-only and were not changed in JSON mode"
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

func checkScopeData(result *checksvc.RunResult) commandpayload.CheckScope {
	if result == nil {
		return commandpayload.CheckScope{}
	}
	return commandpayload.CheckScope{
		Type:  result.Scope.Type,
		Value: result.Scope.Value,
	}
}

func checkIncompleteWarnings(result *checksvc.RunResult) []commandexec.Warning {
	if result == nil {
		return nil
	}
	var warnings []commandexec.Warning
	for _, issue := range result.Issues {
		if issue.Type != check.IssueCheckIncomplete {
			continue
		}
		warnings = append(warnings, commandexec.Warning{
			Code:    codes.WarnCheckRunIncomplete,
			Message: issue.Message,
			Ref:     "Index-backed checks did not fully run; treat results as incomplete until the subsystem is fixed",
		})
	}
	return warnings
}

func withBoolArg(args map[string]interface{}, key string) map[string]interface{} {
	out := make(map[string]interface{}, len(args)+1)
	for k, v := range args {
		out[k] = v
	}
	out[key] = true
	return out
}
