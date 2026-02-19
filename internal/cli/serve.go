package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/mcp"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run Raven as an MCP server",
	Long: `Run Raven as an MCP (Model Context Protocol) server.

This enables LLM agents to interact with your vault through a standardized protocol.

The server communicates over stdin/stdout using JSON-RPC 2.0.

Examples:
  rvn serve                    # Run MCP server using normal CLI vault resolution
  rvn serve --vault personal   # Force named vault for this server process

For use with Claude Desktop, add to your config:
  {
    "mcpServers": {
      "raven": {
        "command": "rvn",
        "args": ["serve", "--vault-path", "/path/to/vault"]
      }
    }
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		baseArgs := make([]string, 0, 8)
		if strings.TrimSpace(configPath) != "" {
			baseArgs = append(baseArgs, "--config", configPath)
		}
		if strings.TrimSpace(statePathFlag) != "" {
			baseArgs = append(baseArgs, "--state", statePathFlag)
		}
		if strings.TrimSpace(vaultPathFlag) != "" {
			baseArgs = append(baseArgs, "--vault-path", vaultPathFlag)
		} else if strings.TrimSpace(vaultName) != "" {
			baseArgs = append(baseArgs, "--vault", vaultName)
		}

		// Don't output anything to stdout except MCP protocol
		// (but we can log to stderr if needed)

		server := mcp.NewServerWithBaseArgs(baseArgs)
		if err := server.Run(); err != nil {
			return fmt.Errorf("MCP server error: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
