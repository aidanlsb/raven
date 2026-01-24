package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
)

// externalWorkflowDef is used for parsing external workflow files.
type externalWorkflowDef struct {
	Description string                           `yaml:"description,omitempty"`
	Inputs      map[string]*config.WorkflowInput `yaml:"inputs,omitempty"`
	Context     map[string]*config.ContextQuery  `yaml:"context,omitempty"`
	Prompt      string                           `yaml:"prompt"`
}

// LoadAll loads all workflows from the vault configuration.
func LoadAll(vaultPath string, vaultCfg *config.VaultConfig) ([]*Workflow, error) {
	if vaultCfg.Workflows == nil {
		return nil, nil
	}

	var workflows []*Workflow
	for name, ref := range vaultCfg.Workflows {
		wf, err := Load(vaultPath, name, ref)
		if err != nil {
			return nil, fmt.Errorf("workflow '%s': %w", name, err)
		}
		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// Load loads a single workflow by name.
func Load(vaultPath, name string, ref *config.WorkflowRef) (*Workflow, error) {
	if ref == nil {
		return nil, fmt.Errorf("workflow reference is nil")
	}

	// Check for conflicting definition
	if ref.File != "" && ref.Prompt != "" {
		return nil, fmt.Errorf("workflow has both 'file' and inline definition; use one or the other")
	}

	// Load from external file if specified
	if ref.File != "" {
		return loadFromFile(vaultPath, name, ref.File)
	}

	// Use inline definition
	if ref.Prompt == "" {
		return nil, fmt.Errorf("workflow must have either 'file' or 'prompt'")
	}

	return &Workflow{
		Name:        name,
		Description: ref.Description,
		Inputs:      ref.Inputs,
		Context:     ref.Context,
		Prompt:      ref.Prompt,
	}, nil
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

	// Read and parse file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var def externalWorkflowDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse workflow file: %w", err)
	}

	if def.Prompt == "" {
		return nil, fmt.Errorf("workflow file must have 'prompt' field")
	}

	return &Workflow{
		Name:        name,
		Description: def.Description,
		Inputs:      def.Inputs,
		Context:     def.Context,
		Prompt:      def.Prompt,
	}, nil
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

	return Load(vaultPath, name, ref)
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
		wf, err := Load(vaultPath, name, ref)
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
