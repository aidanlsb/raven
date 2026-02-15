// Package commands provides command metadata and Cobra command generation.
package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Handler is a function that executes a command.
// It receives the vault path, parsed args, and flag values.
type Handler func(vaultPath string, args []string, flags map[string]interface{}) error

// HandlerRegistry maps command names to their handlers.
var HandlerRegistry = make(map[string]Handler)

// RegisterHandler registers a handler for a command.
func RegisterHandler(name string, handler Handler) {
	HandlerRegistry[name] = handler
}

// GenerateCobraCommand creates a Cobra command from registry metadata.
// This reduces boilerplate by generating Use, Short, Long, Args, and flags
// from the registry, while keeping the handler logic separate.
func GenerateCobraCommand(name string, handler Handler) *cobra.Command {
	meta, ok := Registry[name]
	if !ok {
		return nil
	}

	// Build Use string
	use := name
	for _, arg := range meta.Args {
		if arg.Required {
			use += fmt.Sprintf(" <%s>", arg.Name)
		} else {
			use += fmt.Sprintf(" [%s]", arg.Name)
		}
	}

	// Build Long description
	longDesc := meta.Description
	if meta.LongDesc != "" {
		longDesc = meta.LongDesc
	}
	if len(meta.Examples) > 0 {
		longDesc += "\n\nExamples:\n"
		for _, ex := range meta.Examples {
			longDesc += "  " + ex + "\n"
		}
	}

	// Calculate min/max args
	minArgs := 0
	maxArgs := len(meta.Args)
	for _, arg := range meta.Args {
		if arg.Required {
			minArgs++
		}
	}

	cmd := &cobra.Command{
		Use:   use,
		Short: meta.Description,
		Long:  longDesc,
	}

	// Set args validation
	if minArgs == maxArgs {
		if minArgs == 0 {
			cmd.Args = cobra.NoArgs
		} else {
			cmd.Args = cobra.ExactArgs(minArgs)
		}
	} else {
		cmd.Args = cobra.RangeArgs(minArgs, maxArgs)
	}

	// Add flags
	for _, flag := range meta.Flags {
		switch flag.Type {
		case FlagTypeBool:
			defaultBool := flag.Default == "true"
			cmd.Flags().Bool(flag.Name, defaultBool, flag.Description)
		case FlagTypeInt:
			var defaultInt int
			fmt.Sscanf(flag.Default, "%d", &defaultInt)
			cmd.Flags().Int(flag.Name, defaultInt, flag.Description)
		case FlagTypeKeyValue:
			// StringArray for repeatable flags
			cmd.Flags().StringArray(flag.Name, nil, flag.Description)
		case FlagTypeStringSlice:
			// StringArray for repeatable string flags
			cmd.Flags().StringArray(flag.Name, nil, flag.Description)
		case FlagTypeJSON:
			// JSON payloads are passed as string flags.
			cmd.Flags().String(flag.Name, flag.Default, flag.Description)
		case FlagTypePosKeyValue:
			// Positional key=value args are not Cobra flags.
			// (This exists in the registry primarily for MCP schema generation.)
			continue
		default:
			cmd.Flags().String(flag.Name, flag.Default, flag.Description)
		}

		// Add short flag if specified
		if flag.Short != "" {
			cmd.Flags().Lookup(flag.Name).Shorthand = flag.Short
		}
	}

	// Add shell completion for dynamic args
	if len(meta.Args) > 0 {
		cmd.ValidArgsFunction = generateCompletionFunc(meta.Args)
	}

	// Set RunE if handler provided
	if handler != nil {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			// Get vault path from parent command context
			vaultPath, _ := cmd.Flags().GetString("vault-path")
			if vaultPath == "" {
				if parent := cmd.Parent(); parent != nil {
					vaultPath, _ = parent.PersistentFlags().GetString("vault-path")
				}
			}

			// Collect flag values
			flags := make(map[string]interface{})
			for _, flag := range meta.Flags {
				switch flag.Type {
				case FlagTypeBool:
					val, _ := cmd.Flags().GetBool(flag.Name)
					flags[flag.Name] = val
				case FlagTypeInt:
					val, _ := cmd.Flags().GetInt(flag.Name)
					flags[flag.Name] = val
				case FlagTypeKeyValue:
					val, _ := cmd.Flags().GetStringArray(flag.Name)
					flags[flag.Name] = val
				case FlagTypeStringSlice:
					val, _ := cmd.Flags().GetStringArray(flag.Name)
					flags[flag.Name] = val
				case FlagTypeJSON:
					val, _ := cmd.Flags().GetString(flag.Name)
					flags[flag.Name] = val
				case FlagTypePosKeyValue:
					// Positional key=value args are in args, not flags.
					continue
				default:
					val, _ := cmd.Flags().GetString(flag.Name)
					flags[flag.Name] = val
				}
			}

			return handler(vaultPath, args, flags)
		}
	}

	return cmd
}

// generateCompletionFunc creates a shell completion function based on arg metadata.
func generateCompletionFunc(args []ArgMeta) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, completedArgs []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		argIndex := len(completedArgs)
		if argIndex >= len(args) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		arg := args[argIndex]

		// Static completions
		if len(arg.Completions) > 0 {
			var matches []string
			for _, c := range arg.Completions {
				if strings.HasPrefix(c, toComplete) {
					matches = append(matches, c)
				}
			}
			return matches, cobra.ShellCompDirectiveNoFileComp
		}

		// Dynamic completions would be handled by the caller
		// (requires schema access, which we don't have in this package)
		switch arg.DynamicComp {
		case "types", "traits", "queries":
			// Signal that we need dynamic completion from schema
			return nil, cobra.ShellCompDirectiveNoFileComp
		case "files":
			return nil, cobra.ShellCompDirectiveDefault // Let shell do file completion
		}

		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// GetCommandMeta returns the metadata for a command.
func GetCommandMeta(name string) (Meta, bool) {
	meta, ok := Registry[name]
	return meta, ok
}

// AllCommandNames returns all registered command names.
func AllCommandNames() []string {
	var names []string
	for name := range Registry {
		names = append(names, name)
	}
	return names
}
