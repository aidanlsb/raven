package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/lsp"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Run Raven as an LSP server",
	Long: `Run Raven as a Language Server Protocol server.

This enables editors (Neovim, VS Code, Helix, ...) to provide diagnostics,
completion, go-to-definition, find-references, and hover for vault files.

The server communicates over stdin/stdout using Content-Length framed
JSON-RPC 2.0 (LSP 3.17).

Vault selection: --vault-path or --vault take priority; otherwise the editor
workspace root is used when it is a Raven vault; otherwise the active/default
vault from Raven config.

Example Neovim setup:
  vim.lsp.config('raven', {
    cmd = { 'rvn', 'lsp' },
    filetypes = { 'markdown' },
    root_markers = { 'raven.yaml', '.raven' },
  })
  vim.lsp.enable('raven')`,
	RunE: func(cmd *cobra.Command, args []string) error {
		explicitPath := strings.TrimSpace(vaultPathFlag)
		if explicitPath == "" && strings.TrimSpace(vaultName) != "" {
			p, err := cfg.GetVaultPath(vaultName)
			if err != nil {
				return fmt.Errorf("vault '%s' not found", vaultName)
			}
			explicitPath = p
		}

		// Best-effort active/default vault; the workspace root may still
		// provide the vault when this fails.
		fallbackPath := ""
		if current, err := configsvc.CurrentVault(configsvc.ContextOptions{
			ConfigPathOverride: configPath,
			StatePathOverride:  statePathFlag,
		}); err == nil && current != nil {
			fallbackPath = current.Current.Path
		}

		// Don't output anything to stdout except LSP protocol.
		server := lsp.NewServer(lsp.Options{
			ExplicitVaultPath: explicitPath,
			FallbackVaultPath: fallbackPath,
		})
		if err := server.Run(); err != nil {
			return fmt.Errorf("LSP server error: %w", err)
		}
		return nil
	},
}

func init() {
	markLocalLeaf(lspCmd)
	rootCmd.AddCommand(lspCmd)
}
