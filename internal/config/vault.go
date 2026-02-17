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

	// DailyTemplate is a path to a template file (e.g., "templates/daily.md")
	// for new daily notes.
	DailyTemplate string `yaml:"daily_template,omitempty"`

	// Directories configures directory organization for the vault.
	// When set, typed objects are nested under objects/, untyped pages under pages/.
	// Object IDs strip the directory prefix, keeping references short.
	Directories *DirectoriesConfig `yaml:"directories,omitempty"`

	// AutoReindex triggers an incremental reindex after CLI operations that modify files (default: true)
	AutoReindex *bool `yaml:"auto_reindex,omitempty"`

	// Queries defines saved queries that can be run with `rvn query <name>`
	Queries map[string]*SavedQuery `yaml:"queries,omitempty"`

	// Workflows defines reusable multi-step workflows.
	// Declarations are file references keyed by workflow name.
	Workflows map[string]*WorkflowRef `yaml:"workflows,omitempty"`

	// WorkflowRuns configures persisted workflow run checkpoints and retention.
	WorkflowRuns *WorkflowRunsConfig `yaml:"workflow_runs,omitempty"`

	// ProtectedPrefixes are additional vault-relative path prefixes that Raven should
	// treat as protected/system-managed. Workflows and other automation features
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
		val := value.Content[i+1]

		// Backwards compatibility: migrate top-level workflow_directory into directories.workflow.
		if key.Value == "workflow_directory" {
			legacyDir := strings.TrimSpace(val.Value)
			if legacyDir != "" {
				if vc.Directories == nil {
					vc.Directories = &DirectoriesConfig{}
				}
				if vc.Directories.Workflow == "" && vc.Directories.Workflows == "" {
					vc.Directories.Workflow = legacyDir
				}
			}
			continue
		}

		if key.Value != "workflows" || val.Kind != yaml.MappingNode {
			continue
		}

		for j := 0; j < len(val.Content)-1; j += 2 {
			wfKey := val.Content[j]
			wfVal := val.Content[j+1]
			if wfKey.Value != "runs" || wfVal.Kind != yaml.MappingNode {
				continue
			}
			if !isWorkflowRunsConfigNode(wfVal) {
				continue
			}

			var runs WorkflowRunsConfig
			if err := wfVal.Decode(&runs); err != nil {
				return fmt.Errorf("invalid workflows.runs config: %w", err)
			}
			vc.WorkflowRuns = &runs
			if vc.Workflows != nil {
				delete(vc.Workflows, "runs")
			}
		}
	}
	return nil
}

func isWorkflowRunsConfigNode(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		switch node.Content[i].Value {
		case "storage_path", "auto_prune", "keep_completed_for_days",
			"keep_failed_for_days", "keep_awaiting_for_days",
			"max_runs", "preserve_latest_per_workflow":
			return true
		}
	}
	return false
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

	// Workflow is the root directory for workflow definition files
	// referenced by workflows.<name>.file declarations.
	// If empty, defaults to "workflows/".
	Workflow string `yaml:"workflow,omitempty"`

	// Template is the root directory for template files referenced by:
	// - schema types.<type>.template
	// - daily_template in raven.yaml
	// If empty, defaults to "templates/".
	Template string `yaml:"template,omitempty"`

	// Deprecated: use Object instead. Kept for backwards compatibility.
	Objects string `yaml:"objects,omitempty"`

	// Deprecated: use Page instead. Kept for backwards compatibility.
	Pages string `yaml:"pages,omitempty"`

	// Deprecated: use Workflow instead. Kept for backwards compatibility.
	Workflows string `yaml:"workflows,omitempty"`

	// Deprecated: use Template instead. Kept for backwards compatibility.
	Templates string `yaml:"templates,omitempty"`
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
	if cfg.Workflow == "" && cfg.Workflows != "" {
		cfg.Workflow = cfg.Workflows
	}
	if cfg.Template == "" && cfg.Templates != "" {
		cfg.Template = cfg.Templates
	}

	// Normalize paths: ensure trailing slash and no leading slash.
	cfg.Object = paths.NormalizeDirRoot(cfg.Object)
	cfg.Page = paths.NormalizeDirRoot(cfg.Page)
	cfg.Workflow = paths.NormalizeDirRoot(cfg.Workflow)
	cfg.Template = paths.NormalizeDirRoot(cfg.Template)

	// If page root is omitted, default it to object root.
	// This keeps "all notes under one root" configs simple:
	// directories:
	//   object: objects/
	if cfg.Page == "" && cfg.Object != "" {
		cfg.Page = cfg.Object
	}

	// Clear deprecated fields after normalization to avoid confusion
	cfg.Objects = ""
	cfg.Pages = ""
	cfg.Workflows = ""
	cfg.Templates = ""

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

	Steps []*WorkflowStep `yaml:"steps,omitempty"`
}

func (w *WorkflowRef) UnmarshalYAML(value *yaml.Node) error {
	if w == nil {
		return fmt.Errorf("workflow reference is nil")
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("invalid workflow definition (expected mapping)")
	}

	for i := 0; i < len(value.Content)-1; i += 2 {
		switch value.Content[i].Value {
		case "context", "prompt", "outputs":
			return fmt.Errorf(
				"workflow uses legacy top-level key '%s': workflows are steps-only in v3; migrate to explicit 'steps' with 'agent' and 'tool' steps",
				value.Content[i].Value,
			)
		}
	}

	type plain WorkflowRef
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	*w = WorkflowRef(p)
	return nil
}

// WorkflowInput defines a workflow input parameter.
type WorkflowInput struct {
	Type        string      `yaml:"type" json:"type"`
	Required    bool        `yaml:"required,omitempty" json:"required,omitempty"`
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Target      string      `yaml:"target,omitempty" json:"target,omitempty"`
}

// WorkflowStep defines a single step in a workflow.
//
// This is a configuration-level type (parsed from YAML). Runtime behavior is
// implemented in internal/workflow.
type WorkflowStep struct {
	ID          string `yaml:"id" json:"id"`
	Type        string `yaml:"type" json:"type"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// query
	RQL string `yaml:"rql,omitempty" json:"rql,omitempty"`

	// read
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty"`

	// search
	Term  string `yaml:"term,omitempty" json:"term,omitempty"`
	Limit int    `yaml:"limit,omitempty" json:"limit,omitempty"`

	// backlinks
	Target string `yaml:"target,omitempty" json:"target,omitempty"`

	// agent
	Prompt  string                           `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Outputs map[string]*WorkflowPromptOutput `yaml:"outputs,omitempty" json:"outputs,omitempty"`

	// tool
	Tool      string                 `yaml:"tool,omitempty" json:"tool,omitempty"`
	Arguments map[string]interface{} `yaml:"arguments,omitempty" json:"arguments,omitempty"`
}

type WorkflowPromptOutput struct {
	Type     string `yaml:"type" json:"type"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

// WorkflowRunsConfig configures persisted workflow run checkpoints and pruning.
type WorkflowRunsConfig struct {
	// StoragePath is the vault-relative directory for workflow run checkpoints.
	StoragePath string `yaml:"storage_path,omitempty"`

	// AutoPrune runs retention cleanup on workflow run/continue.
	AutoPrune *bool `yaml:"auto_prune,omitempty"`

	// KeepCompletedForDays keeps completed runs for this many days.
	KeepCompletedForDays *int `yaml:"keep_completed_for_days,omitempty"`

	// KeepFailedForDays keeps failed runs for this many days.
	KeepFailedForDays *int `yaml:"keep_failed_for_days,omitempty"`

	// KeepAwaitingForDays keeps awaiting-agent runs for this many days.
	KeepAwaitingForDays *int `yaml:"keep_awaiting_for_days,omitempty"`

	// MaxRuns is the hard cap for stored run records.
	MaxRuns *int `yaml:"max_runs,omitempty"`

	// PreserveLatestPerWorkflow preserves the newest N runs per workflow.
	PreserveLatestPerWorkflow *int `yaml:"preserve_latest_per_workflow,omitempty"`
}

// ResolvedWorkflowRunsConfig is WorkflowRunsConfig with defaults applied.
type ResolvedWorkflowRunsConfig struct {
	StoragePath               string
	AutoPrune                 bool
	KeepCompletedForDays      int
	KeepFailedForDays         int
	KeepAwaitingForDays       int
	MaxRuns                   int
	PreserveLatestPerWorkflow int
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

func intPtr(i int) *int {
	return &i
}

const defaultWorkflowDirectory = "workflows/"
const defaultTemplateDirectory = "templates/"

// GetWorkflowRunsConfig returns workflow run checkpoint settings with defaults applied.
func (vc *VaultConfig) GetWorkflowRunsConfig() ResolvedWorkflowRunsConfig {
	defaults := ResolvedWorkflowRunsConfig{
		StoragePath:               ".raven/workflow-runs",
		AutoPrune:                 true,
		KeepCompletedForDays:      7,
		KeepFailedForDays:         14,
		KeepAwaitingForDays:       30,
		MaxRuns:                   1000,
		PreserveLatestPerWorkflow: 5,
	}

	if vc.WorkflowRuns == nil {
		return defaults
	}

	cfg := vc.WorkflowRuns
	if cfg.StoragePath != "" {
		normalized := filepath.ToSlash(filepath.Clean(cfg.StoragePath))
		normalized = strings.TrimPrefix(normalized, "./")
		normalized = strings.TrimPrefix(normalized, "/")
		if normalized != "." && normalized != "" && !strings.HasPrefix(normalized, "..") {
			defaults.StoragePath = normalized
		}
	}
	if cfg.AutoPrune != nil {
		defaults.AutoPrune = *cfg.AutoPrune
	}
	if cfg.KeepCompletedForDays != nil {
		defaults.KeepCompletedForDays = *cfg.KeepCompletedForDays
	}
	if cfg.KeepFailedForDays != nil {
		defaults.KeepFailedForDays = *cfg.KeepFailedForDays
	}
	if cfg.KeepAwaitingForDays != nil {
		defaults.KeepAwaitingForDays = *cfg.KeepAwaitingForDays
	}
	if cfg.MaxRuns != nil {
		defaults.MaxRuns = *cfg.MaxRuns
	}
	if cfg.PreserveLatestPerWorkflow != nil {
		defaults.PreserveLatestPerWorkflow = *cfg.PreserveLatestPerWorkflow
	}
	return defaults
}

// GetWorkflowDirectory returns the configured workflow directory with defaults applied.
// The result is always normalized as a vault-relative directory with trailing slash.
func (vc *VaultConfig) GetWorkflowDirectory() string {
	dir := defaultWorkflowDirectory
	if vc != nil && vc.Directories != nil {
		raw := vc.Directories.Workflow
		if raw == "" {
			raw = vc.Directories.Workflows
		}
		if raw != "" {
			dir = raw
		}
	}

	normalized := paths.NormalizeDirRoot(dir)
	if normalized == "" {
		return defaultWorkflowDirectory
	}

	cleaned := filepath.ToSlash(filepath.Clean(normalized))
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return defaultWorkflowDirectory
	}
	return paths.NormalizeDirRoot(cleaned)
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

	cleaned := filepath.ToSlash(filepath.Clean(normalized))
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return defaultTemplateDirectory
	}
	return paths.NormalizeDirRoot(cleaned)
}

// SavedQuery defines a saved query using the Raven query language.
type SavedQuery struct {
	// Query is the query string using Raven query language
	// e.g., "object:project .status==active" or "trait:due .value==past"
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

# Additional protected/system prefixes (additive).
# Workflows and other automation features refuse to operate on protected paths.
# Critical protected paths are enforced automatically (.raven/, .trash/, .git/, raven.yaml, schema.yaml).
# protected_prefixes:
#   - templates/
#   - private/

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
    query: "object:project has(trait:status .value==in_progress)"
    description: "Projects marked in progress"

# Optional directory settings
# directories:
#   object: object/
#   page: page/
#   workflow: workflows/
#   template: templates/

# Workflows registry - declarations are file references only
# Workflow definitions live in directories.workflow (default: workflows/)
workflows:
  onboard:
    file: workflows/onboard.yaml

# Workflow run checkpoint retention
# Add this under the top-level workflows block
#   runs:
#     storage_path: .raven/workflow-runs
#     auto_prune: true
#     keep_completed_for_days: 7
#     keep_failed_for_days: 14
#     keep_awaiting_for_days: 30
#     max_runs: 1000
#     preserve_latest_per_workflow: 5
`

	if err := atomicfile.WriteFile(configPath, []byte(defaultConfig), 0o644); err != nil {
		return false, fmt.Errorf("failed to write vault config: %w", err)
	}

	workflowDir := filepath.Join(vaultPath, "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return false, fmt.Errorf("failed to create default workflow directory: %w", err)
	}

	defaultOnboardWorkflow := `description: "Interactive vault setup and onboarding"
steps:
  - id: onboard-agent
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      You are helping the user set up their Raven vault. This is a fresh vault with default configuration.

      Your goal is to understand what they want to track and customize the schema accordingly.

      ## Interview Questions

      Ask these questions conversationally (not all at once):

      1. **What do you want to use this vault for?**
         Examples: work projects, personal tasks, reading notes, meeting notes, research, recipes, contacts, etc.

      2. **What kinds of things do you want to track?**
         These become types. Listen for nouns: projects, people, books, articles, meetings, decisions, etc.

      3. **What metadata matters to you?**
         These become fields or traits. Listen for: deadlines, priorities, status, tags, ratings, etc.

      4. **Do you want daily notes?**
         The vault already supports these. Ask if they want to use them for journaling, standups, logs, etc.

      5. **What are 2-3 concrete things you're working on right now?**
         Use these to create seed content that makes the vault immediately useful.

      ## Actions to Take

      Based on their answers:

      1. **Create types** using raven_schema_add_type for each kind of object they want to track
         - Set appropriate name_field and default_path
         - Add relevant fields

      2. **Create traits** using raven_schema_add_trait for cross-cutting annotations
         - @due, @priority, @status are already in the default schema
         - Add custom ones based on their needs (e.g., @rating, @context, @energy)

      3. **Create 2-3 seed objects** using raven_new based on what they're currently working on
         - This demonstrates the system and gives them something to query

      4. **Show a sample query** using raven_query to demonstrate immediate value
         - Query something they just created

      5. **Suggest useful saved queries** and offer to add them with raven_query_add

      ## Important Guidelines

      - Be conversational, not robotic. This is a dialog, not a form.
      - Start simple. Don't overwhelm with options - let complexity emerge from their needs.
      - Explain as you go. Help them understand why you're creating each type/trait.
      - The default schema already has: person, project types and due, priority, status, highlight, pinned, archived traits.
      - Build on defaults rather than replacing them unless they ask.
      - Refer to raven://guide/onboarding for detailed guidance on the onboarding process.
`
	onboardPath := filepath.Join(workflowDir, "onboard.yaml")
	if err := atomicfile.WriteFile(onboardPath, []byte(defaultOnboardWorkflow), 0o644); err != nil {
		return false, fmt.Errorf("failed to write default onboard workflow: %w", err)
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
