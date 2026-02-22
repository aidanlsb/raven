package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/paths"
)

const maxReferenceCompletionResults = 200

type referenceCompletionOptions struct {
	IncludeDynamicDates bool
	DisableWhenStdin    bool
	NonTargetDirective  cobra.ShellCompDirective
}

func completeReferenceArgAt(argIndex int, opts referenceCompletionOptions) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if opts.DisableWhenStdin {
			stdinMode, err := cmd.Flags().GetBool("stdin")
			if err == nil && stdinMode {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}

		if len(args) != argIndex {
			return nil, opts.NonTargetDirective
		}

		return completeReferenceValues(cmd, toComplete, opts.IncludeDynamicDates)
	}
}

func completeReferenceFlag(includeDynamicDates bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeReferenceValues(cmd, toComplete, includeDynamicDates)
	}
}

func completeReferenceValues(cmd *cobra.Command, toComplete string, includeDynamicDates bool) ([]string, cobra.ShellCompDirective) {
	vaultPath := completionVaultPath(cmd)
	if vaultPath == "" {
		dateMatches := filterDynamicDateKeywords(toComplete, includeDynamicDates)
		if len(dateMatches) > 0 {
			return dateMatches, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveDefault
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		dateMatches := filterDynamicDateKeywords(toComplete, includeDynamicDates)
		if len(dateMatches) > 0 {
			return dateMatches, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveDefault
	}
	defer db.Close()

	ids, err := db.AllObjectIDs()
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}

	matches := referenceCompletionCandidates(ids, toComplete, includeDynamicDates)
	if len(matches) == 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}

	return matches, cobra.ShellCompDirectiveNoFileComp
}

func referenceCompletionCandidates(objectIDs []string, toComplete string, includeDynamicDates bool) []string {
	pathLikeInput := strings.Contains(toComplete, "/")
	sectionInput := strings.Contains(toComplete, "#")

	seen := make(map[string]struct{})
	matches := make([]string, 0, minInt(maxReferenceCompletionResults, len(objectIDs)))

	addMatch := func(candidate string) {
		if candidate == "" {
			return
		}
		if !matchesCompletion(candidate, toComplete) {
			return
		}
		if _, exists := seen[candidate]; exists {
			return
		}
		seen[candidate] = struct{}{}
		matches = append(matches, candidate)
	}

	for _, id := range objectIDs {
		if !sectionInput && strings.Contains(id, "#") {
			continue
		}

		addMatch(id)

		if pathLikeInput || sectionInput {
			continue
		}

		addMatch(paths.ShortNameFromID(id))
	}

	for _, keyword := range filterDynamicDateKeywords(toComplete, includeDynamicDates) {
		addMatch(keyword)
	}

	sort.Strings(matches)
	if len(matches) > maxReferenceCompletionResults {
		matches = matches[:maxReferenceCompletionResults]
	}
	return matches
}

func filterDynamicDateKeywords(toComplete string, includeDynamicDates bool) []string {
	if !includeDynamicDates {
		return nil
	}

	candidates := []string{"today", "tomorrow", "yesterday"}
	var matches []string
	for _, c := range candidates {
		if matchesCompletion(c, toComplete) {
			matches = append(matches, c)
		}
	}
	return matches
}

func matchesCompletion(candidate, input string) bool {
	if input == "" {
		return true
	}

	candidate = strings.ToLower(candidate)
	input = strings.ToLower(input)

	if strings.HasPrefix(candidate, input) {
		return true
	}

	// Allow segment-wise shorthand for hyphenated names:
	// "proj-o" matches "project-one".
	if strings.Contains(input, "-") && strings.Contains(candidate, "-") {
		inputParts := strings.Split(input, "-")
		candidateParts := strings.Split(candidate, "-")
		if len(candidateParts) >= len(inputParts) {
			for i, part := range inputParts {
				if part == "" {
					continue
				}
				if !strings.HasPrefix(candidateParts[i], part) {
					return false
				}
			}
			return true
		}
	}

	return false
}

func completionVaultPath(cmd *cobra.Command) string {
	if explicit := strings.TrimSpace(getFlagString(cmd, "vault-path")); explicit != "" {
		return explicit
	}

	cfgPath := strings.TrimSpace(getFlagString(cmd, "config"))
	statePath := strings.TrimSpace(getFlagString(cmd, "state"))
	namedVault := strings.TrimSpace(getFlagString(cmd, "vault"))

	resolvedConfigPath := config.ResolveConfigPath(cfgPath)

	var (
		cfg *config.Config
		err error
	)
	if cfgPath != "" {
		cfg, err = config.LoadFrom(cfgPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil || cfg == nil {
		return ""
	}

	if namedVault != "" {
		path, err := cfg.GetVaultPath(namedVault)
		if err == nil {
			return path
		}
		return ""
	}

	resolvedStatePath := config.ResolveStatePath(statePath, resolvedConfigPath, cfg)
	state, err := config.LoadState(resolvedStatePath)
	if err == nil {
		activeVaultName := strings.TrimSpace(state.ActiveVault)
		if activeVaultName != "" {
			path, err := cfg.GetVaultPath(activeVaultName)
			if err == nil {
				return path
			}
		}
	}

	defaultPath, err := cfg.GetDefaultVaultPath()
	if err == nil {
		return defaultPath
	}

	return ""
}

func getFlagString(cmd *cobra.Command, name string) string {
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
