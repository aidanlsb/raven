package cli

import (
	"encoding/json"
	"fmt"
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
	VaultPath       func() string
	Args            cobra.PositionalArgs
	Prepare         func(cmd *cobra.Command, args []string) (preparedArgs []string, handled bool, err error)
	BuildArgs       func(cmd *cobra.Command, args []string) (map[string]interface{}, error)
	Invoke          func(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result
	HandleError     func(result commandexec.Result) error
	HandleResult    func(cmd *cobra.Command, result commandexec.Result) error
	RenderHuman     func(cmd *cobra.Command, result commandexec.Result) error
	SkipFlagBinding bool
}

func newCanonicalLeafCommand(commandID string, opts canonicalLeafOptions) *cobra.Command {
	meta, ok := commands.EffectiveMeta(commandID)
	if !ok {
		panic(fmt.Sprintf("registry metadata missing for %q", commandID))
	}

	cmd := &cobra.Command{
		Use:   localUsageForMeta(meta),
		Short: meta.Description,
		Long:  buildLongDesc(meta),
		Args:  cobraArgsForMeta(meta),
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

			var (
				argsMap map[string]interface{}
				err     error
			)
			if opts.BuildArgs != nil {
				argsMap, err = opts.BuildArgs(cmd, args)
				if err != nil {
					return err
				}
				if argsMap == nil {
					return nil
				}
			} else {
				argsMap, err = buildCanonicalArgsForMeta(meta, cmd, args)
				if err != nil {
					return err
				}
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
			if !result.OK {
				if isJSONOutput() {
					outputCanonicalResultJSON(result)
					return nil
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
				outputCanonicalResultJSON(result)
				return nil
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

	if !opts.SkipFlagBinding {
		bindMetaFlags(cmd, meta.Flags)
	}
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
		if arg.Required {
			base += fmt.Sprintf(" <%s>", arg.Name)
		} else {
			base += fmt.Sprintf(" [%s]", arg.Name)
		}
	}
	return base
}

func cobraArgsForMeta(meta commands.Meta) cobra.PositionalArgs {
	minArgs := 0
	maxArgs := len(meta.Args)
	for _, arg := range meta.Args {
		if arg.Required {
			minArgs++
		}
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
			cmd.Flags().Bool(flag.Name, flag.Default == "true", flag.Description)
		case commands.FlagTypeInt:
			cmd.Flags().Int(flag.Name, 0, flag.Description)
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
	}
}

func buildCanonicalArgsForMeta(meta commands.Meta, cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	argsMap := make(map[string]interface{}, len(meta.Args)+len(meta.Flags))
	for i, arg := range meta.Args {
		if i < len(args) {
			argsMap[arg.Name] = args[i]
		}
	}

	for _, flag := range meta.Flags {
		if flag.Type == commands.FlagTypePosKeyValue {
			return nil, fmt.Errorf("command %q requires custom CLI arg wiring for positional key=value arguments", meta.Name)
		}
		if !cmd.Flags().Changed(flag.Name) {
			continue
		}

		switch flag.Type {
		case commands.FlagTypeBool:
			value, _ := cmd.Flags().GetBool(flag.Name)
			argsMap[flag.Name] = value
		case commands.FlagTypeInt:
			value, _ := cmd.Flags().GetInt(flag.Name)
			argsMap[flag.Name] = value
		case commands.FlagTypeStringSlice:
			value, _ := cmd.Flags().GetStringArray(flag.Name)
			argsMap[flag.Name] = value
		case commands.FlagTypeKeyValue:
			value, _ := cmd.Flags().GetStringArray(flag.Name)
			parsed, err := parseKeyValueArgs(flag.Name, value)
			if err != nil {
				return nil, err
			}
			argsMap[flag.Name] = parsed
		case commands.FlagTypeJSON:
			raw, _ := cmd.Flags().GetString(flag.Name)
			var decoded interface{}
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				return nil, fmt.Errorf("invalid --%s JSON: %w", flag.Name, err)
			}
			argsMap[flag.Name] = decoded
		default:
			value, _ := cmd.Flags().GetString(flag.Name)
			argsMap[flag.Name] = value
		}
	}

	return argsMap, nil
}

func parseKeyValueArgs(flagName string, values []string) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --%s value %q: expected key=value", flagName, value)
		}
		out[key] = item
	}
	return out, nil
}
