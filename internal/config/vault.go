package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/paths"
)

// VaultConfig represents vault-level configuration from raven.yaml.
type VaultConfig struct {
	// DailyDirectory is where daily notes are stored (default: "daily/")
	DailyDirectory string `yaml:"daily_directory"`

	// DailyTemplate is either a path to a template file (e.g., "templates/daily.md")
	// or inline template content for daily notes.
	DailyTemplate string `yaml:"daily_template,omitempty"`

	// Directories configures directory organization for the vault.
	// When set, typed objects are nested under objects/, untyped pages under pages/.
	// Object IDs strip the directory prefix, keeping references short.
	Directories *DirectoriesConfig `yaml:"directories,omitempty"`

	// AutoReindex triggers an incremental reindex after CLI operations that modify files (default: true)
	AutoReindex *bool `yaml:"auto_reindex,omitempty"`

	// Queries defines saved queries that can be run with `rvn query <name>`
	Queries map[string]*SavedQuery `yaml:"queries,omitempty"`

	// Workflows defines reusable prompt templates for agents.
	// Can be inline definitions or references to external files.
	Workflows map[string]*WorkflowRef `yaml:"workflows,omitempty"`

	// Capture configures quick capture behavior
	Capture *CaptureConfig `yaml:"capture,omitempty"`

	// Deletion configures file deletion behavior
	Deletion *DeletionConfig `yaml:"deletion,omitempty"`
}

// DirectoriesConfig configures directory organization for the vault.
// This allows nesting type folders under a common root while keeping reference paths short.
//
// Uses singular keys (object, page) to encourage singular directory names,
// which leads to more natural reference syntax like [[person/freya]] instead of [[people/freya]].
type DirectoriesConfig struct {
	// Object is the root directory for typed objects (e.g., "object/").
	// Type default_path values are relative to this.
	// Object IDs strip this prefix, so "object/person/freya" becomes "person/freya".
	Object string `yaml:"object,omitempty"`

	// Page is the root directory for untyped pages (e.g., "page/").
	// Pages get bare IDs without the directory prefix.
	// If empty, defaults to the same as Object.
	Page string `yaml:"page,omitempty"`

	// Deprecated: use Object instead. Kept for backwards compatibility.
	Objects string `yaml:"objects,omitempty"`

	// Deprecated: use Page instead. Kept for backwards compatibility.
	Pages string `yaml:"pages,omitempty"`
}

// GetDirectoriesConfig returns the directories config with defaults applied.
// Returns nil if directories are not configured (flat vault structure).
// Handles backwards compatibility: if old plural keys (objects, pages) are set
// but new singular keys (object, page) are not, uses the old values.
func (vc *VaultConfig) GetDirectoriesConfig() *DirectoriesConfig {
	if vc.Directories == nil {
		return nil
	}
	cfg := *vc.Directories

	// Backwards compatibility: prefer new singular keys, fall back to old plural keys
	if cfg.Object == "" && cfg.Objects != "" {
		cfg.Object = cfg.Objects
	}
	if cfg.Page == "" && cfg.Pages != "" {
		cfg.Page = cfg.Pages
	}

	// Normalize paths: ensure trailing slash and no leading slash.
	cfg.Object = paths.NormalizeDirRoot(cfg.Object)
	cfg.Page = paths.NormalizeDirRoot(cfg.Page)

	// Clear deprecated fields after normalization to avoid confusion
	cfg.Objects = ""
	cfg.Pages = ""

	return &cfg
}

// HasDirectoriesConfig returns true if directory organization is configured.
func (vc *VaultConfig) HasDirectoriesConfig() bool {
	if vc.Directories == nil {
		return false
	}
	// Check both new singular keys and old plural keys for backwards compatibility
	return vc.Directories.Object != "" || vc.Directories.Page != "" ||
		vc.Directories.Objects != "" || vc.Directories.Pages != ""
}

// WorkflowRef is a reference to a workflow definition.
// It can contain an inline definition or a file reference.
type WorkflowRef struct {
	// File is a path to an external workflow file (relative to vault root).
	File string `yaml:"file,omitempty"`

	// Inline definition fields
	Description string                    `yaml:"description,omitempty"`
	Inputs      map[string]*WorkflowInput `yaml:"inputs,omitempty"`
	Context     map[string]*ContextQuery  `yaml:"context,omitempty"`
	Prompt      string                    `yaml:"prompt,omitempty"`
}

// WorkflowInput defines a workflow input parameter.
type WorkflowInput struct {
	Type        string `yaml:"type" json:"type"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Target      string `yaml:"target,omitempty" json:"target,omitempty"`
}

// ContextQuery defines a query to run for gathering context.
type ContextQuery struct {
	Read      string `yaml:"read,omitempty"`
	Query     string `yaml:"query,omitempty"`
	Backlinks string `yaml:"backlinks,omitempty"`
	Search    string `yaml:"search,omitempty"`
	Limit     int    `yaml:"limit,omitempty"`
}

// DeletionConfig configures how file deletion is handled.
type DeletionConfig struct {
	// Behavior controls what happens when a file is deleted.
	// "trash" (default) - Move to trash directory within vault
	// "permanent" - Delete the file permanently
	Behavior string `yaml:"behavior,omitempty"`

	// TrashDir is the directory within the vault where trashed files go (default: ".trash")
	TrashDir string `yaml:"trash_dir,omitempty"`
}

// GetDeletionConfig returns the deletion config with defaults applied.
func (vc *VaultConfig) GetDeletionConfig() *DeletionConfig {
	if vc.Deletion == nil {
		return &DeletionConfig{
			Behavior: "trash",
			TrashDir: ".trash",
		}
	}

	cfg := *vc.Deletion
	if cfg.Behavior == "" {
		cfg.Behavior = "trash"
	}
	if cfg.TrashDir == "" {
		cfg.TrashDir = ".trash"
	}
	return &cfg
}

// IsAutoReindexEnabled returns true if auto-reindexing is enabled (default: true).
func (vc *VaultConfig) IsAutoReindexEnabled() bool {
	if vc.AutoReindex == nil {
		return true // Enabled by default
	}
	return *vc.AutoReindex
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

	// Timestamp prefixes each capture with the time (default: false)
	Timestamp *bool `yaml:"timestamp,omitempty"`
}

// GetCaptureConfig returns the capture config with defaults applied.
func (vc *VaultConfig) GetCaptureConfig() *CaptureConfig {
	if vc.Capture == nil {
		return &CaptureConfig{
			Destination: "daily",
			Timestamp:   boolPtr(false),
		}
	}

	cfg := *vc.Capture
	if cfg.Destination == "" {
		cfg.Destination = "daily"
	}
	if cfg.Timestamp == nil {
		cfg.Timestamp = boolPtr(false)
	}
	return &cfg
}

func boolPtr(b bool) *bool {
	return &b
}

// SavedQuery defines a saved query using the Raven query language.
type SavedQuery struct {
	// Query is the query string using Raven query language
	// e.g., "object:project .status==active" or "trait:due .value==past"
	Query string `yaml:"query"`

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
// Returns true if a new file was created, false if one already existed.
func CreateDefaultVaultConfig(vaultPath string) (bool, error) {
	configPath := filepath.Join(vaultPath, "raven.yaml")

	// Skip if file already exists
	if _, err := os.Stat(configPath); err == nil {
		return false, nil
	}

	defaultConfig := `# Raven Vault Configuration
# These settings control vault-level behavior.

# Where daily notes are stored
daily_directory: daily

# Auto-reindex after CLI operations that modify files (default: true)
# When enabled, commands like 'rvn add', 'rvn new', 'rvn set', 'rvn edit'
# will automatically update the index. Disable if you prefer manual reindexing.
auto_reindex: true

# Quick capture settings for 'rvn add'
# capture:
#   destination: daily      # "daily" (default) or a file path like "inbox.md"
#   heading: "## Captured"  # Optional heading to append under
#   timestamp: true         # Prefix captures with time (default: false)

# Deletion settings for 'rvn delete'
# deletion:
#   behavior: trash         # "trash" (default) or "permanent"
#   trash_dir: .trash       # Directory for trashed files (default: .trash)

# Saved queries - run with 'rvn query <name>'
# Uses the Raven query language (same as 'rvn query "..."')
queries:
  # All items with @due trait
  tasks:
    query: "trait:due"
    description: "All tasks with due dates"

  # Overdue items
  overdue:
    query: "trait:due .value==past"
    description: "Items past their due date"

  # Items due this week
  this-week:
    query: "trait:due .value==this-week"
    description: "Items due this week"

  # Active projects
  active-projects:
    query: "object:project .status==active"
    description: "Projects with status active"
`

	if err := atomicfile.WriteFile(configPath, []byte(defaultConfig), 0o644); err != nil {
		return false, fmt.Errorf("failed to write vault config: %w", err)
	}

	return true, nil
}

// SaveVaultConfig writes the vault config back to raven.yaml.
func SaveVaultConfig(vaultPath string, cfg *VaultConfig) error {
	configPath := filepath.Join(vaultPath, "raven.yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := atomicfile.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write raven.yaml: %w", err)
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

// FilePathToObjectID converts a file path (relative to vault) to an object ID.
// If directories are configured, the appropriate root prefix is stripped.
// For example, with object: "object/", the path "object/person/freya.md" becomes "person/freya".
func (vc *VaultConfig) FilePathToObjectID(filePath string) string {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil {
		return paths.FilePathToObjectID(filePath, "", "")
	}
	return paths.FilePathToObjectID(filePath, dirs.Object, dirs.Page)
}

// ObjectIDToFilePath converts an object ID to a file path (relative to vault).
// If directories are configured, the appropriate root prefix is added.
// The typeName helps determine which root to use (object vs page).
// If typeName is empty or "page", uses the page root; otherwise uses object root.
func (vc *VaultConfig) ObjectIDToFilePath(objectID, typeName string) string {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil {
		return paths.ObjectIDToFilePath(objectID, typeName, "", "")
	}
	return paths.ObjectIDToFilePath(objectID, typeName, dirs.Object, dirs.Page)
}

// ResolveReferenceToFilePath resolves a reference (object ID) to a file path.
// This handles the logic of checking whether the reference looks like a typed object
// (has a directory prefix like "person/freya") or an untyped page ("my-note").
// Returns the relative file path within the vault.
func (vc *VaultConfig) ResolveReferenceToFilePath(ref string) string {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil {
		return paths.ObjectIDToFilePath(ref, "", "", "")
	}

	// If the reference contains a slash, it's likely a typed object path
	// e.g., "person/freya" -> "object/person/freya.md"
	if strings.Contains(ref, "/") {
		if dirs.Object != "" {
			return dirs.Object + ref + ".md"
		}
		return ref + ".md"
	}

	// No slash - it's a bare name, treat as an untyped page
	// e.g., "my-note" -> "page/my-note.md"
	if dirs.Page != "" {
		return dirs.Page + ref + ".md"
	}
	if dirs.Object != "" {
		return dirs.Object + ref + ".md"
	}
	return ref + ".md"
}

// IsInObjectsRoot checks if a file path is under the object root directory.
func (vc *VaultConfig) IsInObjectsRoot(filePath string) bool {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil || dirs.Object == "" {
		return false
	}
	return strings.HasPrefix(filepath.ToSlash(filePath), dirs.Object)
}

// IsInPagesRoot checks if a file path is under the page root directory.
func (vc *VaultConfig) IsInPagesRoot(filePath string) bool {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil || dirs.Page == "" {
		return false
	}
	return strings.HasPrefix(filepath.ToSlash(filePath), dirs.Page)
}

// GetObjectsRoot returns the object root directory, or empty string if not configured.
func (vc *VaultConfig) GetObjectsRoot() string {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil {
		return ""
	}
	return dirs.Object
}

// GetPagesRoot returns the page root directory, or empty string if not configured.
func (vc *VaultConfig) GetPagesRoot() string {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil {
		return ""
	}
	return dirs.Page
}
