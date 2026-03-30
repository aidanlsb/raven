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
	Path              string
	SuggestedName     string
	RegisteredName    string
	AlreadyRegistered bool
	IsActive          bool
	IsDefault         bool
	ConfigPath        string
	StatePath         string
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
		outputJSON(result)
		return nil
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

	if initShouldPrompt() {
		runInitFollowUp(&info)
	}
	renderInitNextSteps(info)

	return nil
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
	info.IsActive = boolValue(data["is_active"])
	info.IsDefault = boolValue(data["is_default"])
	info.ConfigPath = stringValue(data["config_path"])
	info.StatePath = stringValue(data["state_path"])
	return info
}

func runInitFollowUp(info *initPostInitInfo) {
	if info == nil {
		return
	}
	if info.Path == "" {
		return
	}
	prompter := newInitPrompter()

	if !info.AlreadyRegistered {
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
		return
	}

	fmt.Println()
	fmt.Println(ui.Infof("Vault is already registered as '%s'.", info.RegisteredName))

	if !info.IsDefault && prompter.confirm("Set this vault as the default?") {
		pinResult := executeCanonicalCommand("vault_pin", "", map[string]interface{}{
			"name": info.RegisteredName,
		})
		if !pinResult.OK {
			printInitFollowUpFailure("set default vault", pinResult)
			return
		}
		_ = renderVaultPin(nil, pinResult)
		info.IsDefault = true
	}

	if !info.IsActive && prompter.confirm("Set this vault as the active vault?") {
		useResult := executeCanonicalCommand("vault_use", "", map[string]interface{}{
			"name": info.RegisteredName,
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
	if info.AlreadyRegistered && info.IsDefault && info.IsActive {
		return
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader("Next steps"))
	if !info.AlreadyRegistered {
		fmt.Println(ui.Bullet(ui.Hint(fmt.Sprintf("rvn vault add %s %s --pin", info.SuggestedName, formatInitSuggestedPath(info.Path)))))
		fmt.Println(ui.Bullet(ui.Hint(fmt.Sprintf("rvn vault use %s", info.SuggestedName))))
		return
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
