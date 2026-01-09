package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/watcher"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch vault for changes and auto-reindex",
	Long: `Watch the vault directory for file changes and automatically update the index.

This runs in the foreground and reindexes files as they are saved.
Use this if you're not using the LSP server but want automatic reindexing.

The watcher:
- Monitors all .md files in the vault
- Debounces rapid changes (waits 100ms after last change)
- Ignores .raven/, .git/, .trash/ directories
- Updates the index incrementally (single file at a time)

Examples:
  # Watch the default vault
  rvn watch

  # Watch with debug output
  rvn watch --debug

  # Watch a specific vault
  rvn watch --vault-path /path/to/vault

  # Run in background (shell-dependent)
  rvn watch &`,
	RunE: runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
	watchCmd.Flags().Bool("debug", false, "Enable debug logging")
}

func runWatch(cmd *cobra.Command, args []string) error {
	debug, _ := cmd.Flags().GetBool("debug")
	vaultPath := getVaultPath()

	if vaultPath == "" {
		return fmt.Errorf("no vault specified")
	}

	// Load schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	// Load vault config (optional) to support directory roots.
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil || vaultCfg == nil {
		vaultCfg = &config.VaultConfig{}
	}
	var parseOpts *parser.ParseOptions
	if vaultCfg.HasDirectoriesConfig() {
		parseOpts = &parser.ParseOptions{
			ObjectsRoot: vaultCfg.GetObjectsRoot(),
			PagesRoot:   vaultCfg.GetPagesRoot(),
		}
	}

	// Open database
	db, err := index.Open(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to open index: %w", err)
	}
	defer db.Close()

	// Create watcher
	w, err := watcher.New(watcher.Config{
		VaultPath: vaultPath,
		Database:  db,
		Schema:    sch,
		ParseOptions: parseOpts,
		Debug:     debug,
		OnReindex: func(path string, err error) {
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reindexing %s: %v\n", path, err)
			} else if debug {
				fmt.Printf("Reindexed: %s\n", path)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down watcher...")
		cancel()
	}()

	fmt.Printf("Watching vault: %s\n", vaultPath)
	fmt.Println("Press Ctrl+C to stop")

	// Start watching
	return w.Start(ctx)
}
