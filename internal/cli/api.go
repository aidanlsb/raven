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

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/localapi"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	apiPort        int
	apiToken       string
	apiCORSOrigins []string
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Run Raven as a local HTTP API for companion UIs",
	Long: `Run a loopback-only HTTP API that invokes canonical Raven commands.

The server binds 127.0.0.1 only. Every request except CORS preflight requires
a bearer token (generated per run, or set with --token / RAVEN_API_TOKEN).
POST /v1/invoke runs through the same command layer as the CLI and MCP.
The MVP allowlist is read-only vault commands plus set, unset, new, and move.

On startup Raven prints the listen URL and token. Pass --json for a single
startup envelope line that a launcher (for example Tauri) can parse.

Examples:
  rvn api --vault-path /path/to/vault
  rvn api --vault personal --port 7438 --json
  rvn api --vault-path /path/to/vault --token "$RAVEN_API_TOKEN"`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, err := resolveAPIPort(cmd)
		if err != nil {
			return err
		}
		token := resolveAPIToken(cmd)
		origins := resolveAPICORSOrigins(cmd)

		server, err := localapi.New(localapi.Options{
			VaultPath:   getVaultPath(),
			ConfigPath:  getConfigPath(),
			Token:       token,
			Port:        port,
			CORSOrigins: origins,
		})
		if err != nil {
			return handleError(codes.ErrInternal, err, "")
		}

		ln, info, err := server.Listen()
		if err != nil {
			return handleError(codes.ErrInternal, fmt.Errorf("listen: %w", err), "Choose a free --port or pass --port 0")
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		errCh := make(chan error, 1)
		go func() {
			errCh <- server.Serve(ctx, ln)
		}()

		if err := printAPIStartup(info); err != nil {
			stop()
			serveErr := <-errCh
			if serveErr != nil {
				return handleError(codes.ErrInternal, serveErr, "")
			}
			return err
		}

		if err := <-errCh; err != nil {
			return handleError(codes.ErrInternal, err, "")
		}
		return nil
	},
}

func init() {
	markLocalLeaf(apiCmd)
	apiCmd.Flags().IntVar(&apiPort, "port", 7438, "Listen port (0 chooses an ephemeral port)")
	apiCmd.Flags().StringVar(&apiToken, "token", "", "Bearer token (default: generated per run; env RAVEN_API_TOKEN)")
	apiCmd.Flags().StringSliceVar(&apiCORSOrigins, "cors-origin", nil, "Allowed CORS origin (repeatable; default localhost dev ports)")
	rootCmd.AddCommand(apiCmd)
}

func resolveAPIPort(cmd *cobra.Command) (int, error) {
	if cmd.Flags().Changed("port") {
		return apiPort, nil
	}
	raw := strings.TrimSpace(os.Getenv("RAVEN_API_PORT"))
	if raw == "" {
		return apiPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 0 || port > 65535 {
		return 0, handleErrorMsg(codes.ErrInvalidInput, "RAVEN_API_PORT must be an integer 0-65535", "Unset the env var or pass --port")
	}
	return port, nil
}

func resolveAPIToken(cmd *cobra.Command) string {
	if cmd.Flags().Changed("token") {
		return apiToken
	}
	if env := strings.TrimSpace(os.Getenv("RAVEN_API_TOKEN")); env != "" {
		return env
	}
	return apiToken
}

func resolveAPICORSOrigins(cmd *cobra.Command) []string {
	if cmd.Flags().Changed("cors-origin") {
		return apiCORSOrigins
	}
	raw := strings.TrimSpace(os.Getenv("RAVEN_API_CORS_ORIGINS"))
	if raw == "" {
		return apiCORSOrigins
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

func printAPIStartup(info localapi.Startup) error {
	if isJSONOutput() {
		payload, err := json.Marshal(commandexec.Success(info, nil))
		if err != nil {
			return handleError(codes.ErrInternal, err, "")
		}
		fmt.Println(string(payload))
		return nil
	}

	fmt.Printf("%s %s\n", ui.Check("Listening"), ui.FilePath(info.URL))
	fmt.Printf("%s Bearer %s\n", ui.Hint("Authorization:"), info.Token)
	fmt.Printf("%s GET /v1/health /v1/schema /v1/commands; POST /v1/invoke\n", ui.Hint("Endpoints:"))
	return nil
}
