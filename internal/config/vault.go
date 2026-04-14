package config

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/paths"
)

// VaultConfig represents vault-level configuration from raven.yaml.
type VaultConfig struct {
	// DailyDirectory is the resolved daily notes directory (default: "daily").
	// It is derived from directories.daily after loading and is not serialized.
	DailyDirectory string `yaml:"-"`

	// DailyTemplate is a path to a template file (e.g., "templates/daily.md")
	// for new daily notes.
	DailyTemplate string `yaml:"daily_template,omitempty"`

	// Directories configures directory organization for the vault.
	// When set, typed items are nested under the configured type root, untyped pages under the page root.
	// Object IDs strip the directory prefix, keeping references short.
	Directories *DirectoriesConfig `yaml:"directories,omitempty"`

	// AutoReindex triggers an incremental reindex after CLI operations that modify files (default: true)
	AutoReindex *bool `yaml:"auto_reindex,omitempty"`

	// Queries defines saved queries that can be run with `rvn query <name>`
	Queries map[string]*SavedQuery `yaml:"queries,omitempty"`

	// ProtectedPrefixes are additional vault-relative path prefixes that Raven should
	// treat as protected/system-managed. Raven automation features
	// should refuse to read/write/move/edit/delete within these prefixes.
	//
	// Critical protected prefixes are hardcoded in code (e.g., .raven/, .trash/, .git/).
	// This config is additive.
	ProtectedPrefixes []string `yaml:"protected_prefixes,omitempty"`

	// Capture configures quick capture behavior
	Capture *CaptureConfig `yaml:"capture,omitempty"`

	// Deletion configures file deletion behavior
	Deletion *DeletionConfig `yaml:"deletion,omitempty"`
}

func (vc *VaultConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain VaultConfig
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	*vc = VaultConfig(p)

	if value.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i]

		if key.Value == "daily_directory" {
			return fmt.Errorf("daily_directory is no longer supported; use directories.daily instead")
		}
	}
	vc.DailyDirectory = vc.GetDailyDirectory()
	return nil
}

// DirectoriesConfig configures directory organization for the vault.
// This allows nesting type folders under a common root while keeping reference paths short.
//
// Uses user-facing keys (type, page) to encourage singular directory names,
// which leads to more natural reference syntax like [[person/freya]] instead of [[people/freya]].
type DirectoriesConfig struct {
	// Daily is the root directory for daily notes (default: "daily/").
	// Daily note files are created as <daily>/YYYY-MM-DD.md.
	Daily string `yaml:"daily,omitempty"`

	// Object is the root directory for typed items (e.g., "type/").
	// Type default_path values are relative to this.
	// Object IDs strip this prefix, so "type/person/freya" becomes "person/freya".
	Object string `yaml:"type,omitempty"`

	// Page is the root directory for untyped pages (e.g., "page/").
	// Pages get bare IDs without the directory prefix.
	// If empty, defaults to the same as Object.
	Page string `yaml:"page,omitempty"`

	// Template is the root directory for template files referenced by:
	// - schema types.<type>.template
	// - daily_template in raven.yaml
	// If empty, defaults to "templates/".
	Template string `yaml:"template,omitempty"`

	// Deprecated: use Page instead. Kept for backwards compatibility.
	Pages string `yaml:"pages,omitempty"`

	// Deprecated: use Template instead. Kept for backwards compatibility.
	Templates string `yaml:"templates,omitempty"`
}

func (dc *DirectoriesConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain DirectoriesConfig
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	*dc = DirectoriesConfig(p)

	if value.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i]
		switch key.Value {
		case "object", "objects":
			return fmt.Errorf("directories.%s is no longer supported; use directories.type instead", key.Value)
		}
	}
	return nil
}

// GetDirectoriesConfig returns the directories config with defaults applied.
// Returns nil if directories are not configured (flat vault structure).
// Handles backwards compatibility for the remaining plural page/template aliases.
func (vc *VaultConfig) GetDirectoriesConfig() *DirectoriesConfig {
	if vc.Directories == nil {
		return nil
	}
	cfg := *vc.Directories

	// Backwards compatibility: prefer new singular keys, fall back to old plural keys.
	if cfg.Page == "" && cfg.Pages != "" {
		cfg.Page = cfg.Pages
	}
	if cfg.Template == "" && cfg.Templates != "" {
		cfg.Template = cfg.Templates
	}

	// Normalize paths: ensure trailing slash and no leading slash.
	cfg.Daily = paths.NormalizeDirRoot(cfg.Daily)
	cfg.Object = paths.NormalizeDirRoot(cfg.Object)
	cfg.Page = paths.NormalizeDirRoot(cfg.Page)
	cfg.Template = paths.NormalizeDirRoot(cfg.Template)

	// If page root is omitted, default it to the type root.
	// This keeps "all notes under one root" configs simple:
	// directories:
	//   type: type/
	if cfg.Page == "" && cfg.Object != "" {
		cfg.Page = cfg.Object
	}

	// Clear deprecated fields after normalization to avoid confusion
	cfg.Pages = ""
	cfg.Templates = ""

	return &cfg
}

// HasDirectoriesConfig returns true if directory organization is configured.
func (vc *VaultConfig) HasDirectoriesConfig() bool {
	if vc.Directories == nil {
		return false
	}
	return vc.Directories.Object != "" || vc.Directories.Page != "" ||
		vc.Directories.Pages != ""
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
}

// GetCaptureConfig returns the capture config with defaults applied.
func (vc *VaultConfig) GetCaptureConfig() *CaptureConfig {
	if vc.Capture == nil {
		return &CaptureConfig{
			Destination: "daily",
		}
	}

	cfg := *vc.Capture
	if cfg.Destination == "" {
		cfg.Destination = "daily"
	}
	return &cfg
}

const defaultDailyDirectory = "daily"
const defaultTemplateDirectory = "templates/"

//go:embed defaults/raven.yaml
var defaultVaultFiles embed.FS

// GetDailyDirectory returns the configured daily directory with defaults applied.
// The result is always normalized as a vault-relative directory without trailing slash.
func (vc *VaultConfig) GetDailyDirectory() string {
	dir := defaultDailyDirectory
	if vc != nil && vc.Directories != nil && vc.Directories.Daily != "" {
		dir = vc.Directories.Daily
	} else if vc != nil && vc.DailyDirectory != "" {
		// Fallback for programmatic configs that set DailyDirectory directly.
		dir = vc.DailyDirectory
	}

	normalized := paths.NormalizeDirRoot(dir)
	if normalized == "" {
		return defaultDailyDirectory
	}

	cleaned := paths.NormalizeVaultRelPath(normalized)
	if !paths.IsValidVaultRelPath(cleaned) {
		return defaultDailyDirectory
	}
	return strings.TrimSuffix(cleaned, "/")
}

// GetTemplateDirectory returns the configured template directory with defaults applied.
// The result is always normalized as a vault-relative directory with trailing slash.
func (vc *VaultConfig) GetTemplateDirectory() string {
	dir := defaultTemplateDirectory
	if vc != nil && vc.Directories != nil {
		raw := vc.Directories.Template
		if raw == "" {
			raw = vc.Directories.Templates
		}
		if raw != "" {
			dir = raw
		}
	}

	normalized := paths.NormalizeDirRoot(dir)
	if normalized == "" {
		return defaultTemplateDirectory
	}

	cleaned := paths.NormalizeVaultRelPath(normalized)
	if !paths.IsValidVaultRelPath(cleaned) {
		return defaultTemplateDirectory
	}
	return paths.NormalizeDirRoot(cleaned)
}

// SavedQuery defines a saved query using the Raven query language.
type SavedQuery struct {
	// Query is the query string using Raven query language
	// e.g., "type:project .status==active" or "trait:due .value<today"
	Query string `yaml:"query"`

	// Args declares accepted saved-query input names and their positional order.
	// Example: args: [project, status]
	Args []string `yaml:"args,omitempty"`

	// Description for help text
	Description string `yaml:"description,omitempty"`
}

// DefaultVaultConfig returns the default vault configuration.
func DefaultVaultConfig() *VaultConfig {
	return &VaultConfig{
		DailyDirectory: defaultDailyDirectory,
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
	config.DailyDirectory = config.GetDailyDirectory()

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

	defaultConfig, err := defaultVaultFiles.ReadFile("defaults/raven.yaml")
	if err != nil {
		return false, fmt.Errorf("failed to load embedded default vault config: %w", err)
	}
	if err := atomicfile.WriteFile(configPath, defaultConfig, 0o644); err != nil {
		return false, fmt.Errorf("failed to write vault config: %w", err)
	}

	defaultDirectories := []string{"daily", "type", "page", "templates"}
	for _, dir := range defaultDirectories {
		if err := os.MkdirAll(filepath.Join(vaultPath, dir), 0o755); err != nil {
			return false, fmt.Errorf("failed to create default directory %q: %w", dir, err)
		}
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
	return filepath.Join(vaultPath, vc.GetDailyDirectory(), date+".md")
}

// DailyNoteID returns the object ID for a daily note given a date string.
func (vc *VaultConfig) DailyNoteID(date string) string {
	return path.Join(vc.GetDailyDirectory(), date)
}

// FilePathToObjectID converts a file path (relative to vault) to an object ID.
// If directories are configured, the appropriate root prefix is stripped.
// For example, with type: "type/", the path "type/person/freya.md" becomes "person/freya".
func (vc *VaultConfig) FilePathToObjectID(filePath string) string {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil {
		return paths.FilePathToObjectID(filePath, "", "")
	}
	return paths.FilePathToObjectID(filePath, dirs.Object, dirs.Page)
}

// ObjectIDToFilePath converts an object ID to a file path (relative to vault).
// If directories are configured, the appropriate root prefix is added.
// The typeName helps determine which root to use (type vs page).
// If typeName is empty or "page", uses the page root; otherwise uses the type root.
func (vc *VaultConfig) ObjectIDToFilePath(objectID, typeName string) string {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil {
		return paths.ObjectIDToFilePath(objectID, typeName, "", "")
	}
	return paths.ObjectIDToFilePath(objectID, typeName, dirs.Object, dirs.Page)
}

// ResolveReferenceToFilePath resolves a reference (object ID) to a file path.
// This handles the logic of checking whether the reference looks like a typed item
// (has a directory prefix like "person/freya") or an untyped page ("my-note").
// Returns the relative file path within the vault.
func (vc *VaultConfig) ResolveReferenceToFilePath(ref string) string {
	dirs := vc.GetDirectoriesConfig()
	if dirs == nil {
		return paths.ObjectIDToFilePath(ref, "", "", "")
	}

	ref = filepath.ToSlash(strings.TrimSpace(ref))
	ref = strings.TrimPrefix(ref, "./")
	ref = strings.TrimPrefix(ref, "/")
	ref = strings.TrimSuffix(ref, ".md")

	// If the reference is already rooted, keep it rooted exactly once.
	if dirs.Object != "" && strings.HasPrefix(ref, dirs.Object) {
		return ref + ".md"
	}
	if dirs.Page != "" && strings.HasPrefix(ref, dirs.Page) {
		return ref + ".md"
	}

	// If the reference contains a slash, it's likely a typed item path
	// e.g., "person/freya" -> "type/person/freya.md"
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

// GetObjectsRoot returns the configured type root directory, or empty string if not configured.
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
