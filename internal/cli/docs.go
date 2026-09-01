package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

const (
	docsCommandHint = "For command docs, use: rvn help <command>"
)

var docsCmd = newCanonicalLeafCommand("docs", canonicalLeafOptions{
	Prepare:     prepareDocsCommand,
	Invoke:      invokeDocsCommand,
	HandleError: handleCanonicalDocsLeafFailure,
	RenderHuman: renderDocsCommand,
})

func invokeDocsCommand(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	result := executeCanonicalCommand(commandID, vaultPath, args)
	return rewriteCanonicalDocsFailure(result, docsArgsFromCanonical(args))
}

func docsArgsFromCanonical(args map[string]interface{}) []string {
	out := make([]string, 0, 2)
	if section := stringValue(args["section"]); section != "" {
		out = append(out, section)
	}
	if topic := stringValue(args["topic"]); topic != "" {
		out = append(out, topic)
	}
	return out
}

var docsSearchCmd = newCanonicalLeafCommand("docs_search", canonicalLeafOptions{
	HandleError: handleCanonicalDocsLeafFailure,
	RenderHuman: renderDocsSearch,
})

var docsFetchCmd = newCanonicalLeafCommand("docs_fetch", canonicalLeafOptions{
	HandleError: handleCanonicalDocsLeafFailure,
	RenderHuman: renderDocsFetch,
})

func handleCanonicalDocsLeafFailure(result commandexec.Result) error {
	return handleCanonicalDocsFailure(result, nil)
}

func handleCanonicalDocsFailure(result commandexec.Result, args []string) error {
	result = rewriteCanonicalDocsFailure(result, args)
	if result.Error == nil {
		if isJSONOutput() {
			return outputJSON(result)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if isJSONOutput() {
		return outputJSON(result)
	}
	return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
}

func rewriteCanonicalDocsFailure(result commandexec.Result, args []string) commandexec.Result {
	if result.Error == nil {
		return result
	}

	if len(args) > 0 && result.Error.Code == ErrInvalidInput && strings.HasPrefix(result.Error.Message, "unknown docs section: ") {
		if cmdPath, ok := resolveCLICommandPath(args); ok {
			result.Error.Message = fmt.Sprintf("%q is a CLI command, not a docs section", cmdPath)
			result.Error.Suggestion = fmt.Sprintf("Use 'rvn help %s' for command documentation", cmdPath)
		} else if isCommandSectionAlias(args[0]) {
			result.Error.Message = "command docs are not part of 'rvn docs'"
			result.Error.Suggestion = docsCommandHint
		}
	}
	return result
}

func normalizeDocsSegment(input string) string {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func isCommandSectionAlias(raw string) bool {
	normalized := normalizeDocsSegment(raw)
	return normalized == "command" || normalized == "commands"
}

func resolveCLICommandPath(args []string) (string, bool) {
	for i := len(args); i >= 1; i-- {
		path := strings.Join(args[:i], " ")
		cmd, ok := findCommandByPathRuntime(rootCmd, path)
		if !ok {
			continue
		}
		// Don't redirect docs->docs.
		if cmd.Name() == "docs" {
			continue
		}
		return path, true
	}
	return "", false
}

func findCommandByPathRuntime(root *cobra.Command, path string) (*cobra.Command, bool) {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return nil, false
	}

	cur := root
	for _, part := range parts {
		var next *cobra.Command
		for _, child := range cur.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func init() {
	docsCmd.AddCommand(docsSearchCmd)
	docsCmd.AddCommand(docsFetchCmd)
	rootCmd.AddCommand(docsCmd)
}
