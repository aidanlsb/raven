package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
)

type keepContext struct {
	cfg        *config.Config
	state      *config.State
	configPath string
	statePath  string
}

type keepRow struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDefault bool   `json:"is_default"`
	IsActive  bool   `json:"is_active"`
}

type currentKeepInfo struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Source        string `json:"source"`
	ActiveMissing bool   `json:"active_missing"`
}

var (
	keepAddReplace         bool
	keepAddPin             bool
	keepRemoveClearDefault bool
	keepRemoveClearActive  bool
)

func loadKeepContext() (*keepContext, error) {
	loadedCfg, resolvedConfigPath, err := loadGlobalConfigWithPath()
	if err != nil {
		return nil, err
	}
	if loadedCfg == nil {
		loadedCfg = &config.Config{}
	}

	resolvedStatePath := config.ResolveStatePath(statePathFlag, resolvedConfigPath, loadedCfg)
	state, err := config.LoadState(resolvedStatePath)
	if err != nil {
		return nil, err
	}

	return &keepContext{
		cfg:        loadedCfg,
		state:      state,
		configPath: resolvedConfigPath,
		statePath:  resolvedStatePath,
	}, nil
}

func defaultKeepName(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if strings.TrimSpace(cfg.DefaultKeep) != "" {
		return strings.TrimSpace(cfg.DefaultKeep)
	}
	if strings.TrimSpace(cfg.Keep) != "" && len(cfg.Keeps) == 0 {
		return "default"
	}
	return ""
}

func keepRows(cfg *config.Config, state *config.State) ([]keepRow, string, string, bool) {
	keeps := cfg.ListKeeps()
	defaultName := defaultKeepName(cfg)
	activeName := ""
	if state != nil {
		activeName = strings.TrimSpace(state.ActiveKeep)
	}

	rows := make([]keepRow, 0, len(keeps))
	names := make([]string, 0, len(keeps))
	for name := range keeps {
		names = append(names, name)
	}
	sort.Strings(names)

	activeMissing := activeName != ""
	for _, name := range names {
		rows = append(rows, keepRow{
			Name:      name,
			Path:      keeps[name],
			IsDefault: name == defaultName,
			IsActive:  name == activeName,
		})
		if name == activeName {
			activeMissing = false
		}
	}

	return rows, defaultName, activeName, activeMissing
}

func resolveCurrentKeep(cfg *config.Config, state *config.State) (*currentKeepInfo, error) {
	activeName := ""
	if state != nil {
		activeName = strings.TrimSpace(state.ActiveKeep)
	}

	if activeName != "" {
		path, err := cfg.GetKeepPath(activeName)
		if err == nil {
			return &currentKeepInfo{
				Name:   activeName,
				Path:   path,
				Source: "active_keep",
			}, nil
		}
	}

	defaultPath, err := cfg.GetDefaultKeepPath()
	if err != nil {
		if activeName != "" {
			return nil, fmt.Errorf("active keep '%s' not found in config and no default keep configured", activeName)
		}
		return nil, err
	}

	source := "default_keep"
	activeMissing := false
	if activeName != "" {
		source = "default_keep_fallback"
		activeMissing = true
	}

	return &currentKeepInfo{
		Name:          defaultKeepName(cfg),
		Path:          defaultPath,
		Source:        source,
		ActiveMissing: activeMissing,
	}, nil
}

func runKeepList(cmd *cobra.Command, args []string) error {
	ctx, err := loadKeepContext()
	if err != nil {
		return handleError(ErrConfigInvalid, err, "")
	}

	rows, defaultName, activeName, activeMissing := keepRows(ctx.cfg, ctx.state)
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"config_path":    ctx.configPath,
			"state_path":     ctx.statePath,
			"default_keep":   defaultName,
			"active_keep":    activeName,
			"active_missing": activeMissing,
			"keeps":          rows,
		}, &Meta{Count: len(rows)})
		return nil
	}

	if len(rows) == 0 {
		fmt.Println("No keeps configured.")
		fmt.Printf("Config: %s\n", ctx.configPath)
		fmt.Println()
		fmt.Println("Add keeps to config.toml:")
		fmt.Println()
		fmt.Println("  default_keep = \"personal\"")
		fmt.Println()
		fmt.Println("  [keeps]")
		fmt.Println("  personal = \"/path/to/your/notes\"")
		return nil
	}

	for _, row := range rows {
		prefix := "  "
		if row.IsActive && row.IsDefault {
			prefix = ">*"
		} else if row.IsActive {
			prefix = "> "
		} else if row.IsDefault {
			prefix = " *"
		}
		fmt.Printf("%s %-12s -> %s\n", prefix, row.Name, row.Path)
	}

	fmt.Println()
	fmt.Println("> = active keep (state)")
	fmt.Println("* = default keep (config)")
	fmt.Printf("config: %s\n", ctx.configPath)
	fmt.Printf("state:  %s\n", ctx.statePath)
	if activeMissing {
		fmt.Printf("warning: active keep '%s' in state is not configured\n", activeName)
	}

	return nil
}

var keepCmd = &cobra.Command{
	Use:   "keep",
	Short: "Manage configured keeps and active selection",
	Long: `Manage configured keeps and active selection.

The active keep is stored in state.toml.
The default keep is stored in config.toml and used as fallback.`,
	Args: cobra.NoArgs,
	RunE: runKeepList,
}

var keepListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured keeps",
	Args:  cobra.NoArgs,
	RunE:  runKeepList,
}

var keepCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the current resolved keep",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadKeepContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		current, err := resolveCurrentKeep(ctx.cfg, ctx.state)
		if err != nil {
			return handleError(ErrKeepNotSpecified, err, "Use 'rvn keep use <name>' or set default_keep in config.toml")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":           current.Name,
				"path":           current.Path,
				"source":         current.Source,
				"active_missing": current.ActiveMissing,
				"config_path":    ctx.configPath,
				"state_path":     ctx.statePath,
			}, nil)
			return nil
		}

		fmt.Printf("current: %s\n", current.Name)
		fmt.Printf("path:    %s\n", current.Path)
		fmt.Printf("source:  %s\n", current.Source)
		if current.ActiveMissing {
			fmt.Printf("warning: active keep '%s' is missing; using default\n", strings.TrimSpace(ctx.state.ActiveKeep))
		}
		return nil
	},
}

var keepUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the active keep in state.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		ctx, err := loadKeepContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		path, err := ctx.cfg.GetKeepPath(name)
		if err != nil {
			return handleError(ErrKeepNotFound, err, "Run 'rvn keep list' to see configured keeps")
		}

		ctx.state.ActiveKeep = name
		if err := config.SaveState(ctx.statePath, ctx.state); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"active_keep": name,
				"path":        path,
				"state_path":  ctx.statePath,
			}, nil)
			return nil
		}

		fmt.Printf("Active keep set to '%s' -> %s\n", name, path)
		fmt.Printf("state: %s\n", ctx.statePath)
		return nil
	},
}

var keepClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear active keep from state.toml",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := loadKeepContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		prev := strings.TrimSpace(ctx.state.ActiveKeep)
		ctx.state.ActiveKeep = ""
		if err := config.SaveState(ctx.statePath, ctx.state); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"cleared":    true,
				"previous":   prev,
				"state_path": ctx.statePath,
			}, nil)
			return nil
		}

		if prev == "" {
			fmt.Println("Active keep already clear.")
		} else {
			fmt.Printf("Cleared active keep '%s'.\n", prev)
		}
		fmt.Printf("state: %s\n", ctx.statePath)
		return nil
	},
}

var keepPinCmd = &cobra.Command{
	Use:   "pin <name>",
	Short: "Set default_keep in config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		ctx, err := loadKeepContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		path, err := ctx.cfg.GetKeepPath(name)
		if err != nil {
			return handleError(ErrKeepNotFound, err, "Run 'rvn keep list' to see configured keeps")
		}

		ctx.cfg.DefaultKeep = name
		if err := config.SaveTo(ctx.configPath, ctx.cfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"default_keep": name,
				"path":         path,
				"config_path":  ctx.configPath,
			}, nil)
			return nil
		}

		fmt.Printf("Default keep set to '%s' -> %s\n", name, path)
		fmt.Printf("config: %s\n", ctx.configPath)
		return nil
	},
}

var keepAddCmd = &cobra.Command{
	Use:   "add <name> <path>",
	Short: "Add a keep to config.toml",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		rawPath := strings.TrimSpace(args[1])
		if name == "" {
			return handleErrorMsg(ErrMissingArgument, "keep name is required", "")
		}
		if rawPath == "" {
			return handleErrorMsg(ErrMissingArgument, "keep path is required", "")
		}

		ctx, err := loadKeepContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		absPath, err := filepath.Abs(rawPath)
		if err != nil {
			return handleError(ErrInvalidInput, fmt.Errorf("failed to resolve keep path: %w", err), "")
		}

		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return handleErrorMsg(ErrFileNotFound, fmt.Sprintf("keep path does not exist: %s", absPath), "Run 'rvn init "+absPath+"' to create it first")
			}
			return handleError(ErrFileReadError, err, "")
		}
		if !info.IsDir() {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("keep path must be a directory: %s", absPath), "")
		}

		if ctx.cfg.Keeps == nil {
			ctx.cfg.Keeps = make(map[string]string)
		}

		prevPath, existed := ctx.cfg.Keeps[name]
		if existed && !keepAddReplace {
			return handleErrorMsg(ErrDuplicateName, fmt.Sprintf("keep '%s' already exists", name), "Use --replace to update the path")
		}

		ctx.cfg.Keeps[name] = absPath
		if keepAddPin {
			ctx.cfg.DefaultKeep = name
		}

		if err := config.SaveTo(ctx.configPath, ctx.cfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":          name,
				"path":          absPath,
				"config_path":   ctx.configPath,
				"replaced":      existed,
				"previous_path": prevPath,
				"pinned":        keepAddPin,
				"default_keep":  ctx.cfg.DefaultKeep,
			}, nil)
			return nil
		}

		if existed {
			fmt.Printf("Updated keep '%s' -> %s\n", name, absPath)
		} else {
			fmt.Printf("Added keep '%s' -> %s\n", name, absPath)
		}
		if keepAddPin {
			fmt.Printf("Default keep set to '%s'.\n", name)
		}
		fmt.Printf("config: %s\n", ctx.configPath)
		return nil
	},
}

var keepRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a keep from config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		if name == "" {
			return handleErrorMsg(ErrMissingArgument, "keep name is required", "")
		}

		ctx, err := loadKeepContext()
		if err != nil {
			return handleError(ErrConfigInvalid, err, "")
		}

		activeName := strings.TrimSpace(ctx.state.ActiveKeep)
		defaultName := defaultKeepName(ctx.cfg)
		removingActive := activeName != "" && name == activeName
		removingDefault := defaultName != "" && name == defaultName

		if removingDefault && !keepRemoveClearDefault {
			return handleErrorMsg(ErrConfirmationRequired, fmt.Sprintf("keep '%s' is the current default keep", name), "Use --clear-default to clear default_keep as part of removal, or pin another keep first")
		}
		if removingActive && !keepRemoveClearActive {
			return handleErrorMsg(ErrConfirmationRequired, fmt.Sprintf("keep '%s' is the current active keep", name), "Use --clear-active to clear active_keep as part of removal, or switch active keep first")
		}

		removedPath := ""
		removedLegacy := false
		if ctx.cfg.Keeps != nil {
			if p, ok := ctx.cfg.Keeps[name]; ok {
				removedPath = p
				delete(ctx.cfg.Keeps, name)
			}
		}
		if removedPath == "" && name == "default" && strings.TrimSpace(ctx.cfg.Keep) != "" && len(ctx.cfg.Keeps) == 0 {
			removedPath = strings.TrimSpace(ctx.cfg.Keep)
			ctx.cfg.Keep = ""
			removedLegacy = true
		}
		if removedPath == "" {
			return handleErrorMsg(ErrKeepNotFound, fmt.Sprintf("keep '%s' not found in config", name), "Run 'rvn keep list' to see configured keeps")
		}

		defaultCleared := false
		if removingDefault && keepRemoveClearDefault {
			if strings.TrimSpace(ctx.cfg.DefaultKeep) == name {
				ctx.cfg.DefaultKeep = ""
			}
			defaultCleared = true
		}

		activeCleared := false
		if removingActive && keepRemoveClearActive {
			ctx.state.ActiveKeep = ""
			activeCleared = true
		}

		if err := config.SaveTo(ctx.configPath, ctx.cfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}
		if activeCleared {
			if err := config.SaveState(ctx.statePath, ctx.state); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":            name,
				"removed_path":    removedPath,
				"removed_legacy":  removedLegacy,
				"default_cleared": defaultCleared,
				"active_cleared":  activeCleared,
				"config_path":     ctx.configPath,
				"state_path":      ctx.statePath,
			}, nil)
			return nil
		}

		fmt.Printf("Removed keep '%s' (%s)\n", name, removedPath)
		if defaultCleared {
			fmt.Println("Cleared default keep.")
		}
		if activeCleared {
			fmt.Println("Cleared active keep.")
		}
		fmt.Printf("config: %s\n", ctx.configPath)
		if activeCleared {
			fmt.Printf("state:  %s\n", ctx.statePath)
		}
		return nil
	},
}

func init() {
	keepCmd.AddCommand(keepListCmd)
	keepCmd.AddCommand(keepCurrentCmd)
	keepCmd.AddCommand(keepPathCmd)
	keepCmd.AddCommand(keepUseCmd)
	keepCmd.AddCommand(keepPinCmd)
	keepCmd.AddCommand(keepClearCmd)
	keepCmd.AddCommand(keepAddCmd)
	keepCmd.AddCommand(keepRemoveCmd)

	keepAddCmd.Flags().BoolVar(&keepAddReplace, "replace", false, "Replace existing keep path if name already exists")
	keepAddCmd.Flags().BoolVar(&keepAddPin, "pin", false, "Also set this keep as default_keep")
	keepRemoveCmd.Flags().BoolVar(&keepRemoveClearDefault, "clear-default", false, "Clear default_keep when removing the default")
	keepRemoveCmd.Flags().BoolVar(&keepRemoveClearActive, "clear-active", false, "Clear active_keep when removing the active keep")

	rootCmd.AddCommand(keepCmd)
}
