package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/mcpclient"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	mcpClientFlag    string
	mcpVaultName     string
	mcpVaultPathFlag string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP client integrations",
	Long: `Manage MCP client integrations for raven.

Install, remove, or inspect the raven MCP server entry in supported
client config files (Codex, Claude Code, Claude Desktop, Cursor).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Add raven to an MCP client config",
	Long: `Add raven to an MCP client config file.

Supported clients: codex, claude-code, claude-desktop, cursor

Examples:
  rvn mcp install --client codex
  rvn mcp install --client claude-code
  rvn mcp install --client claude-desktop --vault work`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := mcpclient.Client(mcpClientFlag)
		if !mcpclient.ValidClient(mcpClientFlag) {
			return handleErrorMsg(ErrMCPClientInvalid, fmt.Sprintf("unknown client: %s", mcpClientFlag),
				"Supported clients: codex, claude-code, claude-desktop, cursor")
		}

		cfgPath, err := mcpclient.ConfigPath(client, "")
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		entry := mcpclient.BuildServerEntry(configPath, statePathFlag, mcpVaultName, mcpVaultPathFlag)
		result, err := mcpclient.Install(client, cfgPath, entry)
		if err != nil {
			return handleError(ErrMCPConfigWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"client":      string(client),
				"config_path": cfgPath,
				"result":      result.String(),
				"entry": map[string]interface{}{
					"command": entry.Command,
					"args":    entry.Args,
				},
			}, nil)
			return nil
		}

		switch result {
		case mcpclient.Installed:
			fmt.Println(ui.Checkf("Installed raven in %s config.", client))
		case mcpclient.Updated:
			fmt.Println(ui.Checkf("Updated raven in %s config.", client))
		case mcpclient.AlreadyInstalled:
			fmt.Println(ui.Starf("Raven is already installed in %s config.", client))
		}
		fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(cfgPath))
		return nil
	},
}

var mcpRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove raven from an MCP client config",
	Long: `Remove raven from an MCP client config file.

Supported clients: codex, claude-code, claude-desktop, cursor

Examples:
  rvn mcp remove --client codex
  rvn mcp remove --client claude-code`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := mcpclient.Client(mcpClientFlag)
		if !mcpclient.ValidClient(mcpClientFlag) {
			return handleErrorMsg(ErrMCPClientInvalid, fmt.Sprintf("unknown client: %s", mcpClientFlag),
				"Supported clients: codex, claude-code, claude-desktop, cursor")
		}

		cfgPath, err := mcpclient.ConfigPath(client, "")
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		removed, err := mcpclient.Remove(client, cfgPath)
		if err != nil {
			return handleError(ErrMCPConfigWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"client":      string(client),
				"config_path": cfgPath,
				"removed":     removed,
			}, nil)
			return nil
		}

		if removed {
			fmt.Println(ui.Checkf("Removed raven from %s config.", client))
		} else {
			fmt.Println(ui.Starf("Raven was not found in %s config.", client))
		}
		fmt.Printf("%s %s\n", ui.Hint("config:"), ui.FilePath(cfgPath))
		return nil
	},
}

var mcpStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show raven MCP status across all clients",
	Long: `Show raven MCP status across all supported clients.

Checks each client's config file and reports whether raven is configured.

Examples:
  rvn mcp status
  rvn mcp status --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		clients := mcpclient.AllClients()
		statuses := make([]map[string]interface{}, 0, len(clients))

		for _, client := range clients {
			cfgPath, err := mcpclient.ConfigPath(client, "")
			if err != nil {
				continue
			}

			cs, err := mcpclient.Status(client, cfgPath)
			if err != nil {
				// Report error but continue
				if isJSONOutput() {
					statuses = append(statuses, map[string]interface{}{
						"client":      string(client),
						"config_path": cfgPath,
						"error":       err.Error(),
					})
				} else {
					fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", client, ui.Warning(err.Error()))))
				}
				continue
			}

			entry := map[string]interface{}(nil)
			if cs.Entry != nil {
				entry = map[string]interface{}{
					"command": cs.Entry.Command,
					"args":    cs.Entry.Args,
				}
			}

			statuses = append(statuses, map[string]interface{}{
				"client":      string(cs.Client),
				"config_path": cs.ConfigPath,
				"exists":      cs.Exists,
				"installed":   cs.Installed,
				"entry":       entry,
			})

			if !isJSONOutput() {
				status := "not installed"
				detail := ""
				if cs.Installed && cs.Entry != nil {
					status = "installed"
					detail = fmt.Sprintf("  (%s %s)", cs.Entry.Command, strings.Join(cs.Entry.Args, " "))
				} else if !cs.Exists {
					status = "no config file"
				}
				line := fmt.Sprintf("%s %s", ui.Bold.Render(string(client)), status)
				if detail != "" {
					line = fmt.Sprintf("%s %s", line, ui.Hint(detail))
				}
				fmt.Println(ui.Bullet(line))
			}
		}

		if isJSONOutput() {
			installed := 0
			for _, s := range statuses {
				if b, ok := s["installed"].(bool); ok && b {
					installed++
				}
			}
			outputSuccess(map[string]interface{}{
				"clients": statuses,
			}, &Meta{Count: installed})
			return nil
		}

		return nil
	},
}

var mcpShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the MCP config snippet for manual setup",
	Long: `Print the client config snippet for manual setup.

Outputs the config that would be added to the client config file,
useful for unsupported clients or manual configuration.

Examples:
  rvn mcp show --client codex
  rvn mcp show --client claude-code
  rvn mcp show --client cursor --vault work`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mcpClientFlag != "" && !mcpclient.ValidClient(mcpClientFlag) {
			return handleErrorMsg(ErrMCPClientInvalid, fmt.Sprintf("unknown client: %s", mcpClientFlag),
				"Supported clients: codex, claude-code, claude-desktop, cursor")
		}

		client := mcpclient.Client(mcpClientFlag)
		entry := mcpclient.BuildServerEntry(configPath, statePathFlag, mcpVaultName, mcpVaultPathFlag)

		if isJSONOutput() {
			if mcpclient.IsTOMLClient(client) {
				cfgPath, _ := mcpclient.ConfigPath(client, "")
				outputSuccess(map[string]interface{}{
					"client":      string(client),
					"format":      "toml",
					"config_path": cfgPath,
					"snippet": fmt.Sprintf("[mcp_servers.raven]\ncommand = %q\nargs = %s\n",
						entry.Command, mustMarshalArgs(entry.Args)),
				}, nil)
				return nil
			}

			outputSuccess(map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"raven": map[string]interface{}{
						"command": entry.Command,
						"args":    entry.Args,
					},
				},
			}, nil)
			return nil
		}

		snippet, err := mcpclient.ShowSnippet(client, entry)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		fmt.Print(snippet)

		if mcpClientFlag != "" {
			cfgPath, err := mcpclient.ConfigPath(client, "")
			if err == nil {
				fmt.Printf("\n%s %s\n", ui.Hint("Add this to:"), ui.FilePath(cfgPath))
			}
		}

		return nil
	},
}

func init() {
	markLocalLeaf(mcpInstallCmd)
	markLocalLeaf(mcpRemoveCmd)
	markLocalLeaf(mcpStatusCmd)
	markLocalLeaf(mcpShowCmd)

	mcpInstallCmd.Flags().StringVar(&mcpClientFlag, "client", "", "MCP client (codex, claude-code, claude-desktop, cursor)")
	mcpInstallCmd.Flags().StringVar(&mcpVaultName, "vault", "", "Pin a named vault")
	mcpInstallCmd.Flags().StringVar(&mcpVaultPathFlag, "vault-path", "", "Pin an explicit vault path")
	mcpInstallCmd.Flags().StringVar(&configPath, "config", "", "Path to config file")
	mcpInstallCmd.Flags().StringVar(&statePathFlag, "state", "", "Path to state file (overrides state_file in config)")
	_ = mcpInstallCmd.MarkFlagRequired("client")

	mcpRemoveCmd.Flags().StringVar(&mcpClientFlag, "client", "", "MCP client (codex, claude-code, claude-desktop, cursor)")
	_ = mcpRemoveCmd.MarkFlagRequired("client")

	mcpShowCmd.Flags().StringVar(&mcpClientFlag, "client", "", "MCP client (codex, claude-code, claude-desktop, cursor)")
	mcpShowCmd.Flags().StringVar(&mcpVaultName, "vault", "", "Pin a named vault")
	mcpShowCmd.Flags().StringVar(&mcpVaultPathFlag, "vault-path", "", "Pin an explicit vault path")
	mcpShowCmd.Flags().StringVar(&configPath, "config", "", "Path to config file")
	mcpShowCmd.Flags().StringVar(&statePathFlag, "state", "", "Path to state file (overrides state_file in config)")

	mcpCmd.AddCommand(mcpInstallCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)
	mcpCmd.AddCommand(mcpStatusCmd)
	mcpCmd.AddCommand(mcpShowCmd)

	rootCmd.AddCommand(mcpCmd)
}

func mustMarshalArgs(args []string) string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, fmt.Sprintf("%q", arg))
	}
	return "[" + strings.Join(out, ", ") + "]"
}
