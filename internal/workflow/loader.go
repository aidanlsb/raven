package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
)

// externalWorkflowDef is used for parsing external workflow files.
type externalWorkflowDef struct {
	Description string                           `yaml:"description,omitempty"`
	Inputs      map[string]*config.WorkflowInput `yaml:"inputs,omitempty"`
	Steps       []*config.WorkflowStep           `yaml:"steps,omitempty"`
}

// LoadAll loads all workflows from the vault configuration.
func LoadAll(vaultPath string, vaultCfg *config.VaultConfig) ([]*Workflow, error) {
	if vaultCfg.Workflows == nil {
		return nil, nil
	}

	var workflows []*Workflow
	for name, ref := range vaultCfg.Workflows {
		wf, err := LoadWithConfig(vaultPath, name, ref, vaultCfg)
		if err != nil {
			return nil, fmt.Errorf("workflow '%s': %w", name, err)
		}
		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// LoadWithConfig loads a single workflow by name using vault-level workflow policy.
func LoadWithConfig(vaultPath, name string, ref *config.WorkflowRef, vaultCfg *config.VaultConfig) (*Workflow, error) {
	if ref == nil {
		return nil, fmt.Errorf("workflow reference is nil")
	}
	if strings.TrimSpace(ref.File) == "" {
		return nil, fmt.Errorf(
			"inline workflow definitions are not supported: set workflows.%s.file and move the definition to a workflow file",
			name,
		)
	}
	if ref.Description != "" || len(ref.Inputs) > 0 || len(ref.Steps) > 0 {
		return nil, fmt.Errorf(
			"workflow declarations must contain only 'file': move description/inputs/steps into %q",
			ref.File,
		)
	}

	workflowDir := config.DefaultVaultConfig().GetWorkflowDirectory()
	if vaultCfg != nil {
		workflowDir = vaultCfg.GetWorkflowDirectory()
	}
	fileRef, err := ResolveWorkflowFileRef(ref.File, workflowDir)
	if err != nil {
		return nil, err
	}

	wf, err := loadFromFile(vaultPath, name, fileRef)
	if err != nil {
		return nil, err
	}
	return wf, nil
}

// Load loads a single workflow by name with default config policy.
func Load(vaultPath, name string, ref *config.WorkflowRef) (*Workflow, error) {
	return LoadWithConfig(vaultPath, name, ref, config.DefaultVaultConfig())
}

func normalizeWorkflowFileRef(filePath string) (string, error) {
	trimmed := strings.TrimSpace(filePath)
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	normalized := filepath.ToSlash(filepath.Clean(trimmed))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "" || normalized == "." {
		return "", fmt.Errorf("workflow declaration must include a non-empty file path")
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("workflow file path cannot escape the vault")
	}
	return normalized, nil
}

// ResolveWorkflowFileRef normalizes a workflow file reference and enforces
// directories.workflow policy. Bare filenames are resolved under workflowDir.
func ResolveWorkflowFileRef(filePath, workflowDir string) (string, error) {
	normalized, err := normalizeWorkflowFileRef(filePath)
	if err != nil {
		return "", err
	}
	if workflowDir != "" && !strings.HasPrefix(normalized, workflowDir) && !strings.Contains(normalized, "/") {
		normalized = workflowDir + normalized
	}
	if workflowDir != "" && !strings.HasPrefix(normalized, workflowDir) {
		return "", fmt.Errorf(
			"workflow file must be under directories.workflow %q: got %q",
			workflowDir,
			normalized,
		)
	}
	return normalized, nil
}

// loadFromFile loads a workflow from an external YAML file.
func loadFromFile(vaultPath, name, filePath string) (*Workflow, error) {
	// Security: ensure path is within vault
	fullPath := filepath.Join(vaultPath, filePath)
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideVault) {
			return nil, fmt.Errorf("workflow file must be within vault")
		}
		return nil, fmt.Errorf("failed to validate workflow file: %w", err)
	}

	// Read file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse workflow file: %w", err)
	}
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("failed to parse workflow file: empty document")
	}
	if err := rejectLegacyTopLevelKeys(root.Content[0]); err != nil {
		return nil, err
	}

	var def externalWorkflowDef
	if err := root.Content[0].Decode(&def); err != nil {
		return nil, fmt.Errorf("failed to decode workflow file: %w", err)
	}

	wf, err := buildWorkflow(name, def.Description, def.Inputs, def.Steps)
	if err != nil {
		return nil, err
	}
	if err := validateWorkflow(wf); err != nil {
		return nil, err
	}
	return wf, nil
}

func buildWorkflow(
	name string,
	description string,
	inputs map[string]*config.WorkflowInput,
	steps []*config.WorkflowStep,
) (*Workflow, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("workflow must define 'steps'; top-level context/prompt/outputs were removed in workflow v3")
	}

	return &Workflow{
		Name:        name,
		Description: description,
		Inputs:      inputs,
		Steps:       steps,
	}, nil
}

func validateWorkflow(wf *Workflow) error {
	if wf == nil {
		return fmt.Errorf("workflow is nil")
	}
	if len(wf.Steps) == 0 {
		return fmt.Errorf("workflow has no steps")
	}

	seen := make(map[string]struct{}, len(wf.Steps))
	for i, s := range wf.Steps {
		if s == nil {
			return fmt.Errorf("step %d is nil", i)
		}
		if s.ID == "" {
			return fmt.Errorf("step %d is missing id", i)
		}
		if _, ok := seen[s.ID]; ok {
			return fmt.Errorf("duplicate step id: %s", s.ID)
		}
		seen[s.ID] = struct{}{}

		if s.Type == "" {
			return fmt.Errorf("step '%s' is missing type", s.ID)
		}
		switch s.Type {
		case "agent":
			if s.Prompt == "" {
				return fmt.Errorf("step '%s' (agent) missing prompt", s.ID)
			}
			if len(s.Outputs) > 0 {
				for name, out := range s.Outputs {
					if name == "" {
						return fmt.Errorf("step '%s' (agent) has empty output name", s.ID)
					}
					if out == nil {
						return fmt.Errorf("step '%s' (agent) output '%s' is nil", s.ID, name)
					}
					if !isValidOutputType(out.Type) {
						return fmt.Errorf("step '%s' (agent) output '%s' has unknown type '%s'", s.ID, name, out.Type)
					}
				}
			}
		case "tool":
			if s.Tool == "" {
				return fmt.Errorf("step '%s' (tool) missing tool", s.ID)
			}
		default:
			return fmt.Errorf("step '%s' has unknown type '%s'", s.ID, s.Type)
		}
	}
	return nil
}

func rejectLegacyTopLevelKeys(root *yaml.Node) error {
	if root == nil {
		return fmt.Errorf("failed to parse workflow file: empty document")
	}
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("failed to parse workflow file: expected mapping at document root")
	}
	for i := 0; i < len(root.Content)-1; i += 2 {
		switch root.Content[i].Value {
		case "context", "prompt", "outputs":
			return fmt.Errorf(
				"workflow file uses legacy top-level key '%s': workflows are steps-only in v3; migrate to explicit 'steps' with 'agent' and 'tool' steps",
				root.Content[i].Value,
			)
		}
	}
	return nil
}

func isValidOutputType(outputType string) bool {
	switch outputType {
	case "markdown", "string", "number", "bool", "object", "array":
		return true
	default:
		return false
	}
}

// Get retrieves a workflow by name from the vault configuration.
func Get(vaultPath, name string, vaultCfg *config.VaultConfig) (*Workflow, error) {
	if vaultCfg.Workflows == nil {
		return nil, fmt.Errorf("no workflows defined in raven.yaml")
	}

	ref, ok := vaultCfg.Workflows[name]
	if !ok {
		return nil, fmt.Errorf("workflow '%s' not found", name)
	}

	return LoadWithConfig(vaultPath, name, ref, vaultCfg)
}

// List returns all workflow names and descriptions.
func List(vaultPath string, vaultCfg *config.VaultConfig) ([]*ListItem, error) {
	if vaultCfg.Workflows == nil {
		return nil, nil
	}

	var items []*ListItem
	for name, ref := range vaultCfg.Workflows {
		item := &ListItem{
			Name: name,
		}

		// Load to get full definition (handles file references)
		wf, err := LoadWithConfig(vaultPath, name, ref, vaultCfg)
		if err != nil {
			// Include error in description rather than failing
			item.Description = fmt.Sprintf("(error: %v)", err)
		} else {
			item.Description = wf.Description
			item.Inputs = wf.Inputs
		}

		items = append(items, item)
	}

	return items, nil
}
