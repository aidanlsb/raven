package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
)

const (
	canonicalLeafAnnotationKey = "raven.dev/canonical-leaf"
	exceptionLeafAnnotationKey = "raven.dev/exception-leaf"
	localLeafAnnotationKey     = "raven.dev/local-leaf"
)

type humanRenderer = func(*cobra.Command, commandexec.Result) error
type argsBuilder = func(cmd *cobra.Command, positional []string, argsMap map[string]interface{}) error

// canonicalLeafOptions is the default leaf adapter. The path is: bind flags
// from command metadata, build the args map, invoke the shared executor, then
// print JSON or RenderHuman.
type canonicalLeafOptions struct {
	// BuildArgs optionally adjusts the args map after metadata binding.
	BuildArgs argsBuilder
	// RenderHuman prints human output after a successful invoke. JSON mode
	// never calls this. Commands with nothing useful to print may leave it nil.
	RenderHuman humanRenderer
}

// exceptionLeafOptions is for interactive or multi-step CLI flows that cannot
// use the default path: pickers, confirm-and-retry, browse, and check prompts.
// New commands should not start here.
type exceptionLeafOptions struct {
	BuildArgs    argsBuilder
	RenderHuman  humanRenderer
	Args         cobra.PositionalArgs
	Prepare      func(cmd *cobra.Command, args []string) (preparedArgs []string, handled bool, err error)
	Invoke       func(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result
	HandleError  func(cmd *cobra.Command, result commandexec.Result) error
	HandleResult func(cmd *cobra.Command, result commandexec.Result) error
}

func newCanonicalLeafCommand(commandID string, opts canonicalLeafOptions) *cobra.Command {
	return newLeafCommand(commandID, leafRuntime{
		BuildArgs:   opts.BuildArgs,
		RenderHuman: opts.RenderHuman,
	})
}

func newExceptionLeafCommand(commandID string, opts exceptionLeafOptions) *cobra.Command {
	cmd := newLeafCommand(commandID, leafRuntime{
		Args:         opts.Args,
		Prepare:      opts.Prepare,
		BuildArgs:    opts.BuildArgs,
		Invoke:       opts.Invoke,
		HandleError:  opts.HandleError,
		HandleResult: opts.HandleResult,
		RenderHuman:  opts.RenderHuman,
	})
	cmd.Annotations[exceptionLeafAnnotationKey] = "true"
	return cmd
}

type leafRuntime struct {
	Args         cobra.PositionalArgs
	Prepare      func(cmd *cobra.Command, args []string) (preparedArgs []string, handled bool, err error)
	BuildArgs    argsBuilder
	Invoke       func(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result
	HandleError  func(cmd *cobra.Command, result commandexec.Result) error
	HandleResult func(cmd *cobra.Command, result commandexec.Result) error
	RenderHuman  humanRenderer
}

func newLeafCommand(commandID string, rt leafRuntime) *cobra.Command {
	meta, ok := commands.EffectiveMeta(commandID)
	if !ok {
		panic(fmt.Sprintf("registry metadata missing for %q", commandID))
	}
	policy := commands.PolicyForCommandID(commandID)

	cmd := &cobra.Command{
		Use:    localUsageForMeta(meta),
		Short:  meta.Description,
		Long:   buildLongDesc(meta),
		Args:   cobraArgsForMeta(meta),
		Hidden: policy.Invokable && !policy.Discoverable,
		Annotations: map[string]string{
			canonicalLeafAnnotationKey: "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLeaf(cmd, commandID, meta, args, rt)
		},
	}

	if rt.Args != nil {
		cmd.Args = rt.Args
	}

	bindMetaFlags(cmd, meta.Flags)
	return cmd
}

func runLeaf(cmd *cobra.Command, commandID string, meta commands.Meta, args []string, rt leafRuntime) error {
	if rt.Prepare != nil {
		preparedArgs, handled, err := rt.Prepare(cmd, args)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
		args = preparedArgs
	}

	argsMap, err := buildCanonicalArgsForMeta(meta, cmd, args)
	if err != nil {
		return err
	}
	if rt.BuildArgs != nil {
		if err := rt.BuildArgs(cmd, args, argsMap); err != nil {
			return err
		}
	}

	vaultPath := canonicalVaultPath(commandID, argsMap)
	var result commandexec.Result
	if rt.Invoke != nil {
		result = rt.Invoke(cmd, commandID, vaultPath, argsMap)
	} else {
		result = executeCanonicalCommand(commandID, vaultPath, argsMap)
	}

	if rt.HandleResult != nil {
		return rt.HandleResult(cmd, result)
	}
	return finishCanonicalLeaf(cmd, result, rt.RenderHuman, rt.HandleError)
}

// finishCanonicalLeaf is the shared success/failure renderer: JSON when
// requested, otherwise the default error printer or RenderHuman.
func finishCanonicalLeaf(cmd *cobra.Command, result commandexec.Result, render humanRenderer, handleError func(*cobra.Command, commandexec.Result) error) error {
	if isJSONOutput() {
		return outputCanonicalResultJSON(result)
	}
	if !result.OK {
		if handleError != nil {
			return handleError(cmd, result)
		}
		return handleCanonicalFailure(result)
	}
	if render != nil {
		return render(cmd, result)
	}
	return nil
}

func canonicalVaultPath(commandID string, args map[string]interface{}) string {
	if commands.RequiresVaultForInvocation(commandID, args) {
		return getVaultPath()
	}
	return ""
}

func markLocalLeaf(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[localLeafAnnotationKey] = "true"
}

func localUsageForMeta(meta commands.Meta) string {
	nameParts := strings.Fields(meta.Name)
	base := meta.Name
	if len(nameParts) > 0 {
		base = nameParts[len(nameParts)-1]
	}

	use := strings.TrimSpace(meta.Use)
	if use != "" {
		if use == base ||
			strings.HasPrefix(use, base+" ") ||
			strings.HasPrefix(use, base+"<") ||
			strings.HasPrefix(use, base+"[") {
			return use
		}
	}

	for _, arg := range meta.Args {
		name := arg.Name
		if arg.Variadic {
			name += "..."
		}
		if arg.Required && !arg.CLIOptional {
			base += fmt.Sprintf(" <%s>", name)
		} else {
			base += fmt.Sprintf(" [%s]", name)
		}
	}
	return base
}

func cobraArgsForMeta(meta commands.Meta) cobra.PositionalArgs {
	minArgs := 0
	maxArgs := len(meta.Args)
	variadic := false
	for _, arg := range meta.Args {
		if arg.Required && !arg.CLIOptional {
			minArgs++
		}
		if arg.Variadic {
			variadic = true
		}
	}

	if variadic {
		return cobra.MinimumNArgs(minArgs)
	}

	if minArgs == maxArgs {
		if minArgs == 0 {
			return cobra.NoArgs
		}
		return cobra.ExactArgs(minArgs)
	}
	return cobra.RangeArgs(minArgs, maxArgs)
}

func bindMetaFlags(cmd *cobra.Command, flags []commands.FlagMeta) {
	for _, flag := range flags {
		switch flag.Type {
		case commands.FlagTypeBool:
			defaultValue := flag.Default == "true"
			cmd.Flags().Bool(flag.Name, defaultValue, flag.Description)
		case commands.FlagTypeInt:
			defaultValue := 0
			if strings.TrimSpace(flag.Default) != "" {
				parsed, err := strconv.Atoi(strings.TrimSpace(flag.Default))
				if err != nil {
					panic(fmt.Sprintf("invalid default for int flag %q: %q", flag.Name, flag.Default))
				}
				defaultValue = parsed
			}
			cmd.Flags().Int(flag.Name, defaultValue, flag.Description)
		case commands.FlagTypeKeyValue, commands.FlagTypeStringSlice:
			cmd.Flags().StringArray(flag.Name, nil, flag.Description)
		case commands.FlagTypeJSON:
			cmd.Flags().String(flag.Name, flag.Default, flag.Description)
		default:
			cmd.Flags().String(flag.Name, flag.Default, flag.Description)
		}
		if flag.Short != "" {
			cmd.Flags().Lookup(flag.Name).Shorthand = flag.Short
		}
		if flag.Required {
			_ = cmd.MarkFlagRequired(flag.Name)
		}
	}
}

func buildCanonicalArgsForMeta(meta commands.Meta, cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	argsMap := make(map[string]interface{}, len(meta.Args)+len(meta.Flags))

	// Check mutex constraints first
	if err := validateMutexConstraints(meta, cmd); err != nil {
		return nil, err
	}

	// Handle --stdin bulk operations
	stdinMode := false
	if cmd.Flags().Changed("stdin") {
		stdinValue, _ := cmd.Flags().GetBool("stdin")
		if stdinValue {
			stdinMode = true
			argsMap["stdin"] = true

			// Read IDs from stdin
			if meta.BulkStdinArgName != "" {
				fileIDs, sectionIDs, err := ReadIDsFromStdin()
				if err != nil {
					return nil, handleError("INTERNAL", err, "")
				}
				ids := append(fileIDs, sectionIDs...)
				if len(ids) == 0 {
					return nil, handleErrorMsg("MISSING_ARGUMENT",
						fmt.Sprintf("no %s provided via stdin", meta.BulkStdinArgName),
						fmt.Sprintf("Pipe %s to stdin, one per line", meta.BulkStdinArgName))
				}
				argsMap[meta.BulkStdinArgName] = stringsToAny(ids)
			}
		}
	}

	// Check if we're in bulk mode:
	// - via stdin flag, OR
	// - via explicit bulk flags (e.g., --trait-id populating trait_ids)
	bulkMode := stdinMode
	if !bulkMode && meta.BulkStdinArgName != "" {
		// Check if any flag uses ArgsKey matching BulkStdinArgName
		for _, flag := range meta.Flags {
			if flag.ArgsKey == meta.BulkStdinArgName && cmd.Flags().Changed(flag.Name) {
				bulkMode = true
				break
			}
		}
	}

	// Process positional arguments
	argIndex := 0
	hasConsumedNonIndependentRef := false
	for _, arg := range meta.Args {
		// Skip stdin-dependent args when in bulk mode unless marked independent
		if bulkMode && !arg.StdinIndependent && meta.BulkStdinArgName != "" {
			continue
		}

		if arg.Variadic {
			if argIndex < len(args) {
				remaining := args[argIndex:]
				if meta.VariadicJoin {
					argsMap[arg.Name] = strings.Join(remaining, " ")
				} else {
					argsMap[arg.Name] = append([]string{}, remaining...)
				}
			}
			break
		}
		if argIndex < len(args) {
			argsMap[arg.Name] = args[argIndex]
			argIndex++
			// Track if we consumed a non-independent reference arg
			if (arg.Name == "reference" || arg.Name == "object_id") && !arg.StdinIndependent {
				hasConsumedNonIndependentRef = true
			}
		} else if arg.Required && !arg.CLIOptional {
			return nil, handleErrorMsg("MISSING_ARGUMENT",
				fmt.Sprintf("missing required argument: %s", arg.Name),
				fmt.Sprintf("Usage: %s", meta.Use))
		}
	}

	// Check for conflicting stdin + reference in same command
	if bulkMode && meta.BulkStdinArgName != "" && hasConsumedNonIndependentRef {
		return nil, handleErrorMsg("INVALID_INPUT",
			"cannot specify positional reference with --stdin",
			"Use either --stdin or a positional reference, not both")
	}

	// Process flags
	for _, flag := range meta.Flags {
		if !cmd.Flags().Changed(flag.Name) {
			continue
		}

		// Determine the args map key (use ArgsKey if set, otherwise flag.Name)
		argsKey := flag.Name
		if flag.ArgsKey != "" {
			argsKey = flag.ArgsKey
		}

		parser, ok := flagParsers[flag.Type]
		if !ok {
			parser = flagParsers[commands.FlagTypeString]
		}
		if err := parser(cmd, flag.Name, argsKey, argsMap); err != nil {
			return nil, err
		}
	}

	// Include confirm and dry-run flags if present
	if cmd.Flags().Changed("confirm") {
		value, _ := cmd.Flags().GetBool("confirm")
		argsMap["confirm"] = value
	}
	if cmd.Flags().Changed("dry-run") {
		value, _ := cmd.Flags().GetBool("dry-run")
		argsMap["dry-run"] = value
	}

	return argsMap, nil
}

type flagParser func(cmd *cobra.Command, flagName, argsKey string, argsMap map[string]interface{}) error

var flagParsers = map[commands.FlagType]flagParser{
	commands.FlagTypeBool:        parseBoolFlag,
	commands.FlagTypeInt:         parseIntFlag,
	commands.FlagTypeStringSlice: parseStringSliceFlag,
	commands.FlagTypeKeyValue:    parseKeyValueFlag,
	commands.FlagTypeJSON:        parseJSONFlag,
	commands.FlagTypeString:      parseStringFlag,
}

func parseBoolFlag(cmd *cobra.Command, flagName, argsKey string, argsMap map[string]interface{}) error {
	value, _ := cmd.Flags().GetBool(flagName)
	argsMap[argsKey] = value
	return nil
}

func parseIntFlag(cmd *cobra.Command, flagName, argsKey string, argsMap map[string]interface{}) error {
	value, _ := cmd.Flags().GetInt(flagName)
	argsMap[argsKey] = value
	return nil
}

func parseStringSliceFlag(cmd *cobra.Command, flagName, argsKey string, argsMap map[string]interface{}) error {
	value, _ := cmd.Flags().GetStringArray(flagName)
	if existing, ok := argsMap[argsKey].([]string); ok {
		value = append(existing, value...)
	}
	argsMap[argsKey] = value
	return nil
}

func parseKeyValueFlag(cmd *cobra.Command, flagName, argsKey string, argsMap map[string]interface{}) error {
	value, _ := cmd.Flags().GetStringArray(flagName)
	parsed, err := parseKeyValueArgs(flagName, value)
	if err != nil {
		return err
	}
	argsMap[argsKey] = parsed
	return nil
}

func parseJSONFlag(cmd *cobra.Command, flagName, argsKey string, argsMap map[string]interface{}) error {
	raw, _ := cmd.Flags().GetString(flagName)
	if strings.TrimSpace(raw) != "" {
		var decoded interface{}
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return handleErrorMsg("INVALID_INPUT",
				fmt.Sprintf("invalid --%s JSON: %s", flagName, err.Error()),
				"Ensure the JSON is well-formed")
		}
		argsMap[argsKey] = decoded
	}
	return nil
}

func parseStringFlag(cmd *cobra.Command, flagName, argsKey string, argsMap map[string]interface{}) error {
	value, _ := cmd.Flags().GetString(flagName)
	if value != "" || cmd.Flags().Changed(flagName) {
		argsMap[argsKey] = value
	}
	return nil
}

func validateMutexConstraints(meta commands.Meta, cmd *cobra.Command) error {
	for _, group := range meta.MutexGroups {
		setFlags := []string{}
		for _, flagName := range group {
			if cmd.Flags().Changed(flagName) {
				setFlags = append(setFlags, flagName)
			}
		}
		if len(setFlags) > 1 {
			return handleErrorMsg("INVALID_INPUT",
				fmt.Sprintf("--%s and --%s are mutually exclusive", setFlags[0], setFlags[1]),
				fmt.Sprintf("Use only one of: %s", strings.Join(formatFlagList(group), ", ")))
		}
	}
	return nil
}

func formatFlagList(flags []string) []string {
	out := make([]string, len(flags))
	for i, flag := range flags {
		out[i] = "--" + flag
	}
	return out
}

func parseKeyValueArgs(flagName string, values []string) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, handleErrorMsg("INVALID_INPUT",
				fmt.Sprintf("invalid --%s value %q: expected key=value", flagName, value),
				fmt.Sprintf("Use --%s key=value format", flagName))
		}
		out[key] = item
	}
	return out, nil
}
