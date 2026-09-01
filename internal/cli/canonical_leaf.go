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
	localLeafAnnotationKey     = "raven.dev/local-leaf"
)

type canonicalLeafOptions struct {
	VaultPath      func() string
	Args           cobra.PositionalArgs
	Prepare        func(cmd *cobra.Command, args []string) (preparedArgs []string, handled bool, err error)
	Invoke         func(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result
	HandleError    func(result commandexec.Result) error
	HandleErrorCmd func(cmd *cobra.Command, result commandexec.Result) error
	HandleResult   func(cmd *cobra.Command, result commandexec.Result) error
	RenderHuman    func(cmd *cobra.Command, result commandexec.Result) error
}

func newCanonicalLeafCommand(commandID string, opts canonicalLeafOptions) *cobra.Command {
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
			if opts.Prepare != nil {
				preparedArgs, handled, err := opts.Prepare(cmd, args)
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

			vaultPath := ""
			if opts.VaultPath != nil {
				vaultPath = opts.VaultPath()
			}

			invoke := executeCanonicalCommand
			if opts.Invoke != nil {
				invoke = func(commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
					return opts.Invoke(cmd, commandID, vaultPath, args)
				}
			}
			result := invoke(commandID, vaultPath, argsMap)
			handleFailure := handleCanonicalFailure
			if opts.HandleError != nil {
				handleFailure = opts.HandleError
			}
			if opts.HandleErrorCmd != nil {
				handleFailure = func(result commandexec.Result) error {
					return opts.HandleErrorCmd(cmd, result)
				}
			}
			if !result.OK {
				if isJSONOutput() {
					return outputCanonicalResultJSON(result)
				}
				if err := handleFailure(result); err != nil {
					return err
				}
				return nil
			}
			if opts.HandleResult != nil {
				return opts.HandleResult(cmd, result)
			}
			if isJSONOutput() {
				return outputCanonicalResultJSON(result)
			}
			if opts.RenderHuman != nil {
				return opts.RenderHuman(cmd, result)
			}
			return nil
		},
	}

	if opts.Args != nil {
		cmd.Args = opts.Args
	}

	bindMetaFlags(cmd, meta.Flags)
	return cmd
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
		case commands.FlagTypePosKeyValue:
			continue
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
		if flag.Type == commands.FlagTypePosKeyValue {
			return nil, handleErrorMsg("INTERNAL",
				fmt.Sprintf("command %q uses deprecated FlagTypePosKeyValue", meta.Name),
				"Contact support")
		}
		if !cmd.Flags().Changed(flag.Name) {
			continue
		}

		// Determine the args map key (use ArgsKey if set, otherwise flag.Name)
		argsKey := flag.Name
		if flag.ArgsKey != "" {
			argsKey = flag.ArgsKey
		}

		switch flag.Type {
		case commands.FlagTypeBool:
			value, _ := cmd.Flags().GetBool(flag.Name)
			argsMap[argsKey] = value
		case commands.FlagTypeInt:
			value, _ := cmd.Flags().GetInt(flag.Name)
			argsMap[argsKey] = value
		case commands.FlagTypeStringSlice:
			value, _ := cmd.Flags().GetStringArray(flag.Name)
			// If there's already a variadic positional arg with same name, merge them
			if existing, ok := argsMap[argsKey].([]string); ok {
				value = append(existing, value...)
			}
			argsMap[argsKey] = value
		case commands.FlagTypeKeyValue:
			value, _ := cmd.Flags().GetStringArray(flag.Name)
			parsed, err := parseKeyValueArgs(flag.Name, value)
			if err != nil {
				return nil, err
			}
			argsMap[argsKey] = parsed
		case commands.FlagTypeJSON:
			raw, _ := cmd.Flags().GetString(flag.Name)
			if strings.TrimSpace(raw) != "" {
				var decoded interface{}
				if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
					return nil, handleErrorMsg("INVALID_INPUT",
						fmt.Sprintf("invalid --%s JSON: %s", flag.Name, err.Error()),
						"Ensure the JSON is well-formed")
				}
				argsMap[argsKey] = decoded
			}
		default:
			value, _ := cmd.Flags().GetString(flag.Name)
			if value != "" || cmd.Flags().Changed(flag.Name) {
				argsMap[argsKey] = value
			}
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
