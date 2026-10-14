package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/mcp"
)

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run Raven as an MCP server",
	Long: `Run Raven as an MCP (Model Context Protocol) server.

This enables LLM agents to interact with your vault through a standardized protocol.

The server communicates over stdin/stdout using JSON-RPC 2.0.

Examples:
  rvn mcp serve                    # Require vault/vault_path on vault-scoped calls
  rvn mcp serve --vault personal   # Pin a named vault for this server process
  rvn mcp serve --vault-path PATH  # Pin an explicit vault path

For use with Claude Desktop, add to your config:
  {
    "mcpServers": {
      "raven": {
        "command": "rvn",
        "args": ["mcp", "serve", "--vault-path", "/path/to/vault"]
      }
    }
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Don't output anything to stdout except MCP protocol
		// (but we can log to stderr if needed)

		server := mcp.NewServer(mcp.ServerOptions{
			ConfigPath:      configPath,
			PinnedVaultName: vaultName,
			PinnedVaultPath: vaultPathFlag,
		})
		if err := server.Run(); err != nil {
			return fmt.Errorf("MCP server error: %w", err)
		}

		return nil
	},
}

func init() {
	markLocalLeaf(mcpServeCmd)
	mcpCmd.AddCommand(mcpServeCmd)
}
