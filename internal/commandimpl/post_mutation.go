package commandimpl

import (
	"fmt"
	"path"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// applyChangeSet maps the lower-layer projection result onto the canonical
// command response. All index projection and missing-reference detection is
// owned by reindexsvc.
func applyChangeSet(rt *vaultruntime.Runtime, changes mutation.ChangeSet, journalOperations ...string) (commandpayload.MissingReferences, []commandexec.Warning) {
	journalOperation := ""
	if len(journalOperations) > 0 {
		journalOperation = journalOperations[0]
	}
	result := reindexsvc.ProjectChanges(rt, changes, journalOperation)
	missing := commandpayload.MissingReferences{
		MissingRefs:     len(result.MissingRefs),
		MissingRefItems: result.MissingRefs,
	}
	warnings := projectionCommandWarnings(result.Warnings)
	for _, ref := range result.MissingRefs {
		warnings = append(warnings, missingRefWarning(ref))
	}
	return missing, warnings
}

func projectionCommandWarnings(serviceWarnings []reindexsvc.ProjectionWarning) []commandexec.Warning {
	if len(serviceWarnings) == 0 {
		return nil
	}
	warnings := make([]commandexec.Warning, 0, len(serviceWarnings))
	for _, warning := range serviceWarnings {
		warnings = append(warnings, commandexec.Warning{
			Code:    warning.Code,
			Message: warning.Message,
			Ref:     warning.Ref,
		})
	}
	return warnings
}

func missingRefWarning(ref *check.MissingRef) commandexec.Warning {
	warning := commandexec.Warning{
		Code:    codes.WarnRefTargetMissing,
		Message: fmt.Sprintf("Reference [[%s]] does not exist yet", ref.TargetPath),
		Ref:     "Run 'rvn check create-missing' to create missing referenced pages",
	}
	if ref.InferredType != "" {
		title := missingRefTitle(ref.TargetPath)
		warning.SuggestedType = ref.InferredType
		warning.CreateCommand = fmt.Sprintf("rvn new %s %q --path %q --json", ref.InferredType, title, ref.TargetPath)
		warning.CreateInvoke = &commandexec.Invoke{
			Command: "new",
			Args: map[string]interface{}{
				"type":  ref.InferredType,
				"title": title,
				"path":  ref.TargetPath,
			},
		}
	} else {
		warning.CreateCommand = "rvn check create-missing"
		warning.CreateInvoke = &commandexec.Invoke{
			Command: "check create-missing",
			Args:    map[string]interface{}{"confirm": true},
		}
	}
	return warning
}

func missingRefTitle(targetPath string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(targetPath), "/")
	if trimmed == "" {
		return "new-object"
	}
	base := path.Base(trimmed)
	if base == "" || base == "." || base == "/" {
		return "new-object"
	}
	return base
}
