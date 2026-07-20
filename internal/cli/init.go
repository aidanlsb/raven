package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var initCmd = newCanonicalLeafCommand("init", canonicalLeafOptions{
	Args:         cobra.ExactArgs(1),
	Prepare:      prepareInitArgs,
	HandleResult: handleInitResult,
})

var (
	initPromptIn     io.Reader = os.Stdin
	initPromptOut    io.Writer = os.Stdout
	initShouldPrompt           = shouldPromptForConfirm
)

type initPostInitInfo struct {
	Path                string
	SuggestedName       string
	RegisteredName      string
	AlreadyRegistered   bool
	Registered          bool
	IsFirstVault        bool
	HasExistingDefault  bool
	IsActive            bool
	IsDefault           bool
	Activated           bool
	ActiveVaultName     string
	ActiveVaultPath     string
	PreviousActiveName  string
	PreviousActivePath  string
	PreviousVaultName   string
	PreviousVaultPath   string
	PreviousVaultSource string
	SwitchBack          string
	NeedsDefaultChoice  bool
	Guidance            string
	ConfigPath          string
	StatePath           string
}

type initPrompter struct {
	reader *bufio.Reader
}

func prepareInitArgs(_ *cobra.Command, args []string) ([]string, bool, error) {
	if !isJSONOutput() {
		fmt.Printf("%s %s\n", ui.SectionHeader("Initializing vault"), ui.FilePath(args[0]))
	}
	return args, false, nil
}

func handleInitResult(_ *cobra.Command, result commandexec.Result) error {
	if isJSONOutput() {
		return outputJSON(result)
	}

	data := canonicalDataMap(result)
	createdConfig, _ := data["created_config"].(bool)
	createdSchema, _ := data["created_schema"].(bool)
	gitignoreState, _ := data["gitignore_state"].(string)
	status, _ := data["status"].(string)
	docs, _ := data["docs"].(map[string]interface{})
	info := initPostInitInfoFromAny(stringValue(data["path"]), data["post_init"])

	if createdConfig {
		fmt.Println(ui.Check("Created raven.yaml (vault configuration)"))
	} else {
		fmt.Println(ui.Bullet("raven.yaml already exists (kept)"))
	}

	if createdSchema {
		fmt.Println(ui.Check("Created schema.yaml (types and traits)"))
	} else {
		fmt.Println(ui.Bullet("schema.yaml already exists (kept)"))
	}

	fmt.Println(ui.Check("Ensured .raven/ directory exists"))

	switch gitignoreState {
	case "created":
		fmt.Println(ui.Check("Created .gitignore"))
	case "updated":
		fmt.Println(ui.Check("Updated .gitignore (added Raven entries)"))
	default:
		fmt.Println(ui.Bullet(".gitignore already has Raven entries"))
	}

	if len(result.Warnings) > 0 {
		for _, warning := range result.Warnings {
			fmt.Println(ui.Warning(warning.Message))
		}
	} else if fetched, _ := docs["fetched"].(bool); fetched {
		fmt.Println(ui.Checkf("Fetched docs into %s (%d files)", ui.FilePath(stringFromMap(docs, "store_path")), intFromMap(docs, "file_count")))
	}

	if status == "initialized" {
		fmt.Printf("\n%s\n", ui.Star("Vault initialized. Start adding markdown files."))
	} else {
		fmt.Printf("\n%s\n", ui.Star("Existing vault detected. Configuration preserved."))
	}

	renderInitRegistration(info)

	if initShouldPrompt() {
		runInitFollowUp(&info)
	}
	renderInitNextSteps(info)

	return nil
}

// renderInitRegistration reports what the first-run vault policy did during init.
func renderInitRegistration(info initPostInitInfo) {
	if info.Path == "" || !info.AlreadyRegistered {
		return
	}
	fmt.Println()
	switch {
	case info.IsFirstVault:
		activePath := info.ActiveVaultPath
		if activePath == "" {
			activePath = info.Path
		}
		fmt.Println(ui.Checkf("Registered '%s' at %s as your default and active vault.", info.RegisteredName, ui.FilePath(activePath)))
	case info.Activated:
		if info.Registered {
			fmt.Println(ui.Checkf("Registered vault '%s' in global config.", info.RegisteredName))
		} else {
			fmt.Println(ui.Infof("Vault is already registered as '%s'.", info.RegisteredName))
		}
		fmt.Println(ui.Warningf("Active vault switched to '%s' at %s.", info.ActiveVaultName, ui.FilePath(info.ActiveVaultPath)))
		if info.PreviousActiveName != "" {
			fmt.Println(ui.Infof("Previously active: '%s' at %s.", info.PreviousActiveName, ui.FilePath(info.PreviousActivePath)))
		} else if info.PreviousVaultName != "" {
			fmt.Println(ui.Infof("Previously resolved via %s: '%s' at %s.", info.PreviousVaultSource, info.PreviousVaultName, ui.FilePath(info.PreviousVaultPath)))
		}
		if info.SwitchBack != "" {
			fmt.Println(ui.Warningf("Switch back: %s", info.SwitchBack))
		}
	case info.Registered:
		fmt.Println(ui.Checkf("Registered vault '%s' in global config.", info.RegisteredName))
	default:
		fmt.Println(ui.Infof("Vault is already registered as '%s'.", info.RegisteredName))
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func initPostInitInfoFromAny(path string, raw interface{}) initPostInitInfo {
	info := initPostInitInfo{
		Path: path,
	}
	data, _ := raw.(map[string]interface{})
	info.SuggestedName = stringValue(data["suggested_name"])
	info.RegisteredName = stringValue(data["registered_name"])
	info.AlreadyRegistered = boolValue(data["already_registered"])
	info.Registered = boolValue(data["registered"])
	info.IsFirstVault = boolValue(data["is_first_vault"])
	info.HasExistingDefault = boolValue(data["has_existing_default"])
	info.IsActive = boolValue(data["is_active"])
	info.IsDefault = boolValue(data["is_default"])
	info.Activated = boolValue(data["activated"])
	if active, ok := data["active_vault"].(map[string]interface{}); ok {
		info.ActiveVaultName = stringValue(active["name"])
		info.ActiveVaultPath = stringValue(active["path"])
	}
	if previous, ok := data["previous_active_vault"].(map[string]interface{}); ok {
		info.PreviousActiveName = stringValue(previous["name"])
		info.PreviousActivePath = stringValue(previous["path"])
	}
	if previous, ok := data["previous_vault"].(map[string]interface{}); ok {
		info.PreviousVaultName = stringValue(previous["name"])
		info.PreviousVaultPath = stringValue(previous["path"])
		info.PreviousVaultSource = stringValue(previous["source"])
	}
	info.SwitchBack = stringValue(data["switch_back"])
	info.NeedsDefaultChoice = boolValue(data["needs_user_choice_for_default"])
	info.Guidance = stringValue(data["guidance"])
	info.ConfigPath = stringValue(data["config_path"])
	info.StatePath = stringValue(data["state_path"])
	return info
}

// runInitFollowUp handles interactive setup after init.
//
// Registration already happens in the init handler (first-run vault policy), so
// this only needs to:
//   - offer a manual retry if auto-registration failed, and
//   - ask whether an additional active vault should also become the default.
func runInitFollowUp(info *initPostInitInfo) {
	if info == nil || info.Path == "" {
		return
	}

	if !info.AlreadyRegistered {
		runInitManualRegister(info)
		return
	}

	// First vault is fully configured (registered + default + active); nothing to ask.
	if info.IsFirstVault {
		return
	}

	prompter := newInitPrompter()
	fmt.Println()
	fmt.Println(ui.Info("The new vault is active. Change the default only if you intend to."))

	if !info.IsDefault && prompter.confirm("Set this vault as the default?") {
		pinResult := executeCanonicalCommand("vault_pin", "", map[string]interface{}{
			"name": info.RegisteredName,
		})
		if !pinResult.OK {
			printInitFollowUpFailure("set default vault", pinResult)
		} else {
			_ = renderVaultPin(nil, pinResult)
			info.IsDefault = true
			info.NeedsDefaultChoice = false
		}
	}

}

// runInitManualRegister is the interactive fallback used when auto-registration
// failed. It mirrors the register/pin/activate prompts.
func runInitManualRegister(info *initPostInitInfo) {
	prompter := newInitPrompter()
	fmt.Println()
	if !prompter.confirm("Register this vault in global config?") {
		return
	}

	name := strings.TrimSpace(prompter.input("Vault name?", info.SuggestedName))
	if name == "" {
		name = info.SuggestedName
	}
	pin := prompter.confirm("Set this as the default vault?")
	activate := prompter.confirm("Set this as the active vault?")

	addResult := executeCanonicalCommand("vault_add", "", map[string]interface{}{
		"name": name,
		"path": info.Path,
		"pin":  pin,
	})
	if !addResult.OK {
		printInitFollowUpFailure("register vault", addResult)
		return
	}
	_ = renderVaultAdd(nil, addResult)
	info.AlreadyRegistered = true
	info.RegisteredName = name
	info.IsDefault = pin

	if activate {
		useResult := executeCanonicalCommand("vault_use", "", map[string]interface{}{
			"name": name,
		})
		if !useResult.OK {
			printInitFollowUpFailure("activate vault", useResult)
			return
		}
		_ = renderVaultUse(nil, useResult)
		info.IsActive = true
	}
}

func newInitPrompter() *initPrompter {
	return &initPrompter{reader: bufio.NewReader(initPromptIn)}
}

func (p *initPrompter) confirm(message string) bool {
	if !initShouldPrompt() {
		return false
	}
	fmt.Fprintf(initPromptOut, "%s %s ", message, ui.Hint("[y/N]"))
	response, _ := p.reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func (p *initPrompter) input(message, defaultValue string) string {
	if !initShouldPrompt() {
		return defaultValue
	}
	label := message
	if defaultValue != "" {
		label += " " + ui.Hint("["+defaultValue+"]")
	}
	fmt.Fprintf(initPromptOut, "%s ", label)
	response, _ := p.reader.ReadString('\n')
	response = strings.TrimSpace(response)
	if response == "" {
		return defaultValue
	}
	return response
}

func printInitFollowUpFailure(action string, result commandexec.Result) {
	if result.Error == nil {
		return
	}
	fmt.Println(ui.Warningf("Could not %s: %s", action, result.Error.Message))
	if strings.TrimSpace(result.Error.Suggestion) != "" {
		fmt.Printf("  %s\n", ui.Hint(result.Error.Suggestion))
	}
}

func renderInitNextSteps(info initPostInitInfo) {
	if info.Path == "" {
		return
	}
	// Fully configured (first vault, or the user completed the prompts): nothing to do.
	if info.AlreadyRegistered && info.IsDefault && info.IsActive {
		return
	}

	if !info.AlreadyRegistered {
		fmt.Println()
		fmt.Println(ui.SectionHeader("Next steps"))
		fmt.Println(ui.Bullet(ui.Hint(fmt.Sprintf("rvn vault add %s %s --pin", info.SuggestedName, formatInitSuggestedPath(info.Path)))))
		fmt.Println(ui.Bullet(ui.Hint(fmt.Sprintf("rvn vault use %s", info.SuggestedName))))
		return
	}

	// Additional vaults become active automatically; retain the restore command
	// and optional default change as explicit follow-up information.
	fmt.Println()
	fmt.Println(ui.SectionHeader("Next steps"))
	if info.SwitchBack != "" {
		fmt.Println(ui.Bullet(ui.Hint("Switch back: " + info.SwitchBack)))
	}
	if !info.IsDefault {
		fmt.Println(ui.Bullet(ui.Hint(fmt.Sprintf("rvn vault pin %s", info.RegisteredName))))
	}
	if !info.IsActive {
		fmt.Println(ui.Bullet(ui.Hint(fmt.Sprintf("rvn vault use %s", info.RegisteredName))))
	}
}

func formatInitSuggestedPath(path string) string {
	displayPath := strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(path)), "\\", "/")
	return `"` + displayPath + `"`
}
