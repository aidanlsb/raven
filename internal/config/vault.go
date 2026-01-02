package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// VaultConfig represents vault-level configuration from raven.yaml.
type VaultConfig struct {
	// DailyDirectory is where daily notes are stored (default: "daily/")
	DailyDirectory string `yaml:"daily_directory"`

	// Queries defines saved queries that can be run with `rvn query <name>`
	Queries map[string]*SavedQuery `yaml:"queries,omitempty"`

	// Capture configures quick capture behavior
	Capture *CaptureConfig `yaml:"capture,omitempty"`

	// AuditLog enables logging of all operations to .raven/audit.log (default: true)
	AuditLog *bool `yaml:"audit_log,omitempty"`
}

// IsAuditLogEnabled returns true if audit logging is enabled (default: true).
func (vc *VaultConfig) IsAuditLogEnabled() bool {
	if vc.AuditLog == nil {
		return true // Enabled by default
	}
	return *vc.AuditLog
}

// CaptureConfig defines settings for quick capture via `rvn add`.
type CaptureConfig struct {
	// Destination where captures are appended.
	// "daily" (default) - append to today's daily note
	// A file path like "inbox.md" - append to that specific file
	Destination string `yaml:"destination,omitempty"`

	// Heading to append captures under (e.g., "## Captured").
	// If empty, appends to end of file.
	// The heading is created if it doesn't exist.
	Heading string `yaml:"heading,omitempty"`

	// Timestamp prefixes each capture with the time (default: true)
	Timestamp *bool `yaml:"timestamp,omitempty"`

	// Reindex triggers an incremental reindex after capture (default: true)
	Reindex *bool `yaml:"reindex,omitempty"`
}

// GetCaptureConfig returns the capture config with defaults applied.
func (vc *VaultConfig) GetCaptureConfig() *CaptureConfig {
	if vc.Capture == nil {
		return &CaptureConfig{
			Destination: "daily",
			Timestamp:   boolPtr(true),
			Reindex:     boolPtr(true),
		}
	}

	cfg := *vc.Capture
	if cfg.Destination == "" {
		cfg.Destination = "daily"
	}
	if cfg.Timestamp == nil {
		cfg.Timestamp = boolPtr(true)
	}
	if cfg.Reindex == nil {
		cfg.Reindex = boolPtr(true)
	}
	return &cfg
}

func boolPtr(b bool) *bool {
	return &b
}

// SavedQuery defines a saved query.
type SavedQuery struct {
	// Types to query (e.g., ["person", "project"])
	Types []string `yaml:"types,omitempty"`

	// Traits to query (e.g., ["due", "status"])
	Traits []string `yaml:"traits,omitempty"`

	// Filters for each trait (e.g., {"status": "todo,in_progress", "due": "past"})
	Filters map[string]string `yaml:"filters,omitempty"`

	// Tags to query (e.g., ["project", "important"])
	Tags []string `yaml:"tags,omitempty"`

	// Description for help text
	Description string `yaml:"description,omitempty"`
}

// DefaultVaultConfig returns the default vault configuration.
func DefaultVaultConfig() *VaultConfig {
	return &VaultConfig{
		DailyDirectory: "daily",
	}
}

// LoadVaultConfig loads vault configuration from raven.yaml.
// Returns default config if file doesn't exist.
func LoadVaultConfig(vaultPath string) (*VaultConfig, error) {
	configPath := filepath.Join(vaultPath, "raven.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultVaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault config %s: %w", configPath, err)
	}

	var config VaultConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse vault config %s: %w", configPath, err)
	}

	// Apply defaults for missing values
	if config.DailyDirectory == "" {
		config.DailyDirectory = "daily"
	}

	return &config, nil
}

// CreateDefaultVaultConfig creates a default raven.yaml file in the vault.
func CreateDefaultVaultConfig(vaultPath string) error {
	configPath := filepath.Join(vaultPath, "raven.yaml")

	defaultConfig := `# Raven Vault Configuration
# These settings control vault-level behavior.

# Where daily notes are stored
daily_directory: daily

# Quick capture settings for 'rvn add'
# capture:
#   destination: daily      # "daily" (default) or a file path like "inbox.md"
#   heading: "## Captured"  # Optional heading to append under
#   timestamp: true         # Prefix captures with time (default: true)
#   reindex: true           # Reindex file after capture (default: true)

# Saved queries - run with 'rvn query <name>'
queries:
  # All items with @due or @status traits (i.e., "tasks")
  tasks:
    traits: [due, status]
    filters:
      status: "todo,in_progress,"   # Include items without explicit status
    description: "Open tasks"

  # Overdue items
  overdue:
    traits: [due]
    filters:
      due: past
    description: "Items past their due date"

  # Items due this week
  this-week:
    traits: [due]
    filters:
      due: this-week
    description: "Items due this week"

  # Example tag-based query
  # important:
  #   tags: [important]
  #   description: "Items tagged #important"
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write vault config: %w", err)
	}

	return nil
}

// DailyNotePath returns the full path for a daily note given a date string (YYYY-MM-DD).
func (vc *VaultConfig) DailyNotePath(vaultPath, date string) string {
	return filepath.Join(vaultPath, vc.DailyDirectory, date+".md")
}

// DailyNoteID returns the object ID for a daily note given a date string.
func (vc *VaultConfig) DailyNoteID(date string) string {
	return filepath.Join(vc.DailyDirectory, date)
}
