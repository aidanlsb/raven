package commandimpl

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

const indexUpdateFailedWarningCode = codes.WarnIndexUpdateFailed

func openSession(vaultPath string, mode vaultsession.Mode) (*vaultsession.Session, commandexec.Result) {
	sess, err := vaultsession.Open(vaultPath, mode)
	if err != nil {
		return nil, commandexec.FromServiceError(err)
	}
	return sess, commandexec.Result{}
}

func refreshPolicy(args map[string]any) vaultsession.RefreshPolicy {
	policy := vaultsession.RefreshPolicy{NoRefresh: boolArg(args, "no-refresh")}
	if _, ok := args["refresh"]; ok {
		value := boolArg(args, "refresh")
		policy.Refresh = &value
	}
	return policy
}

func missingRefsOf(proj vaultsession.MutationResult) commandpayload.MissingReferences {
	return commandpayload.MissingReferences{
		MissingRefs:     len(proj.MissingRefs),
		MissingRefItems: proj.MissingRefs,
	}
}

func mutateResult[T any](sess *vaultsession.Session, fn func(*vaultruntime.Runtime) (T, mutation.ChangeSet, error)) (T, vaultsession.MutationResult, commandexec.Result) {
	return mutateMapped(sess, fn, mapContentMutationError)
}

func mutateMapped[T any](sess *vaultsession.Session, fn func(*vaultruntime.Runtime) (T, mutation.ChangeSet, error), mapErr func(error) commandexec.Result) (T, vaultsession.MutationResult, commandexec.Result) {
	value, proj, err := vaultsession.Apply(sess, fn)
	if err != nil {
		var zero T
		return zero, proj, withMutationWarnings(mapErr(err), proj)
	}
	return value, proj, commandexec.Result{}
}

func withMutationWarnings(result commandexec.Result, proj vaultsession.MutationResult) commandexec.Result {
	if len(proj.Warnings) == 0 {
		return result
	}
	result.Warnings = append(result.Warnings, proj.Warnings...)
	return result
}

func appendCommandWarnings(groups ...[]commandexec.Warning) []commandexec.Warning {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	if total == 0 {
		return nil
	}
	combined := make([]commandexec.Warning, 0, total)
	for _, group := range groups {
		combined = append(combined, group...)
	}
	return combined
}

func withIndexRefreshWarnings(warnings []commandexec.Warning, result commandexec.Result) commandexec.Result {
	if len(warnings) == 0 || !result.OK {
		return result
	}
	result.Warnings = append(warnings, result.Warnings...)
	return result
}
