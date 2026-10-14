package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/localapi"
)

var (
	apiPort        int
	apiToken       string
	apiCORSOrigins []string
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Run Raven's local HTTP API for companion UIs",
	Long: `Run a headless local HTTP API bound to 127.0.0.1.

This is the Raven-side surface for Perch and other companion UIs. There is no
embedded UI. Markdown remains the source of truth; every invoke goes through
the same command layer as the CLI and MCP.

The process prints the listen URL and bearer token on startup. Use --json for
a single-line JSON object a launcher (for example Tauri) can parse.

A bearer token is required on every request. One is generated per run unless
--token or RAVEN_API_TOKEN is set. CORS is restricted to localhost companion
dev origins by default; override with --cors-origin or RAVEN_API_CORS_ORIGINS.

Examples:
  rvn api --vault-path /path/to/vault
  rvn api --json --port 0
  rvn api --token "$RAVEN_API_TOKEN" --cors-origin http://localhost:5173`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLocalAPI(cmd)
	},
}

func runLocalAPI(cmd *cobra.Command) error {
	port, token, origins, err := resolveAPIListenOptions(cmd)
	if err != nil {
		return err
	}

	server, err := localapi.New(localapi.Options{
		VaultPath:   getVaultPath(),
		VaultName:   vaultName,
		ConfigPath:  configPath,
		StatePath:   statePathFlag,
		Token:       token,
		CORSOrigins: origins,
	})
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	listener, info, err := server.Bind(port)
	if err != nil {
		return handleError(ErrInternal, err, "Choose a free --port or pass --port 0")
	}

	if err := printAPIStartup(info); err != nil {
		_ = listener.Close()
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Serve(ctx, listener); err != nil {
		return handleError(ErrInternal, err, "")
	}
	return nil
}

func resolveAPIListenOptions(cmd *cobra.Command) (port int, token string, origins []string, err error) {
	port = apiPort
	if !cmd.Flags().Changed("port") {
		if raw := strings.TrimSpace(os.Getenv("RAVEN_API_PORT")); raw != "" {
			parsed, parseErr := strconv.Atoi(raw)
			if parseErr != nil || parsed < 0 || parsed > 65535 {
				return 0, "", nil, handleErrorMsg(ErrInvalidInput, "invalid RAVEN_API_PORT", "Set RAVEN_API_PORT to an integer 0-65535")
			}
			port = parsed
		}
	}

	token = strings.TrimSpace(apiToken)
	if !cmd.Flags().Changed("token") {
		if envToken := strings.TrimSpace(os.Getenv("RAVEN_API_TOKEN")); envToken != "" {
			token = envToken
		}
	}

	origins = append([]string{}, apiCORSOrigins...)
	if !cmd.Flags().Changed("cors-origin") {
		if raw := strings.TrimSpace(os.Getenv("RAVEN_API_CORS_ORIGINS")); raw != "" {
			origins = splitCommaList(raw)
		}
	}
	return port, token, origins, nil
}

func printAPIStartup(info localapi.Info) error {
	if isJSONOutput() {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(map[string]interface{}{
			"ok":   true,
			"data": info,
		})
	}
	fmt.Printf("Raven local API listening on %s\n", info.URL)
	fmt.Printf("Authorization: Bearer %s\n", info.Token)
	return nil
}

func splitCommaList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func init() {
	markLocalLeaf(apiCmd)
	apiCmd.Flags().IntVar(&apiPort, "port", localapi.DefaultPort, "Listen port (0 = OS-assigned; also RAVEN_API_PORT)")
	apiCmd.Flags().StringVar(&apiToken, "token", "", "Bearer token (generated per run if omitted; also RAVEN_API_TOKEN)")
	apiCmd.Flags().StringArrayVar(&apiCORSOrigins, "cors-origin", nil, "Allowed CORS origin (repeatable; also RAVEN_API_CORS_ORIGINS)")
	rootCmd.AddCommand(apiCmd)
}
