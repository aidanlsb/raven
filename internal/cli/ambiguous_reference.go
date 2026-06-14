package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

type ambiguousReferenceRetryOptions struct {
	CommandID string
	ArgKey    string
	Prompt    string
	Fallback  func(commandexec.Result) error
	BuildArgs func(cmd *cobra.Command, selected string) (map[string]interface{}, error)
	Render    func(cmd *cobra.Command, result commandexec.Result) error
}

func handleAmbiguousReferenceRetry(cmd *cobra.Command, result commandexec.Result, opts ambiguousReferenceRetryOptions) error {
	fallback := opts.Fallback
	if fallback == nil {
		fallback = handleCanonicalFailure
	}
	if result.Error == nil {
		return nil
	}
	if result.Error.Code != ErrRefAmbiguous || !canUseRavenInteractive() {
		return fallback(result)
	}

	reference, matches, matchSources := ambiguousReferenceDetails(result.Error.Details)
	if len(matches) == 0 {
		return fallback(result)
	}

	selected, ok, err := pickAmbiguousReference(reference, matches, matchSources, opts.Prompt)
	if err != nil {
		return fallback(result)
	}
	if !ok {
		return nil
	}

	args := map[string]interface{}{opts.ArgKey: selected}
	if opts.BuildArgs != nil {
		args, err = opts.BuildArgs(cmd, selected)
		if err != nil {
			return err
		}
	}

	retryResult := executeCanonicalCommand(opts.CommandID, getVaultPath(), args)
	if !retryResult.OK {
		return fallback(retryResult)
	}
	if opts.Render == nil {
		return nil
	}
	return opts.Render(cmd, retryResult)
}
