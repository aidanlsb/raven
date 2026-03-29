package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/vault"
)

var dailyCmd = newCanonicalLeafCommand("daily", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	HandleResult: handleDailyResult,
})

func handleDailyResult(cmd *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	relativePath, _ := data["file"].(string)
	created, _ := data["created"].(bool)
	filePath := relativePath
	if relativePath != "" {
		filePath = filepath.Join(getVaultPath(), filepath.FromSlash(relativePath))
	}

	if !isJSONOutput() && created {
		fmt.Printf("Created %s\n", relativePath)
	}

	edit, _ := cmd.Flags().GetBool("edit")
	if isJSONOutput() {
		editor := ""
		if cfg := getConfig(); cfg != nil {
			editor = cfg.GetEditor()
		}

		opened := false
		if edit {
			opened = vault.OpenInEditor(getConfig(), filePath)
		}

		payload := map[string]interface{}{}
		for key, value := range data {
			payload[key] = value
		}
		payload["opened"] = opened
		payload["editor"] = editor
		outputSuccess(payload, result.Meta)
		return nil
	}

	openFileInEditor(filePath, relativePath, created)
	return nil
}

func init() {
	rootCmd.AddCommand(dailyCmd)
}
