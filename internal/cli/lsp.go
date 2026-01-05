package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/lsp"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Start the Language Server Protocol server",
	Long: `Start a Language Server Protocol (LSP) server for Raven.

This enables IDE features like:
- Autocomplete for references ([[) and traits (@)
- Go-to-definition for references
- Hover information
- Real-time diagnostics

The server communicates over stdin/stdout using JSON-RPC.

Configure your editor to use this command as the LSP server for markdown files.

Examples:
  # Start LSP server (for editor integration)
  rvn lsp

  # Start with debug logging to stderr
  rvn lsp --debug

  # Start for a specific vault
  rvn lsp --vault-path /path/to/vault`,
	RunE: runLSP,
}

func init() {
	rootCmd.AddCommand(lspCmd)
	lspCmd.Flags().Bool("debug", false, "Enable debug logging to stderr")
}

func runLSP(cmd *cobra.Command, args []string) error {
	debug, _ := cmd.Flags().GetBool("debug")
	vaultPath := getVaultPath()

	// Create server
	server := lsp.NewServer(vaultPath, debug)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Run server
	return server.Run(ctx)
}
