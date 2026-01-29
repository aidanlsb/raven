package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
)

// externalWorkflowDef is used for parsing external workflow files.
type externalWorkflowDef struct {
	Description string                                  `yaml:"description,omitempty"`
	Inputs      map[string]*config.WorkflowInput        `yaml:"inputs,omitempty"`
	Context     map[string]*config.WorkflowContextItem  `yaml:"context,omitempty"`
	Prompt      string                                  `yaml:"prompt,omitempty"`
	Outputs     map[string]*config.WorkflowPromptOutput `yaml:"outputs,omitempty"`
	Steps       []*config.WorkflowStep                  `yaml:"steps,omitempty"`
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
	// Allow a top-level description alongside file-backed workflows for convenience,
	// but disallow mixing file-backed workflows with any inline definition fields.
	if ref.File != "" && (len(ref.Inputs) > 0 || len(ref.Steps) > 0 || len(ref.Context) > 0 || ref.Prompt != "" || len(ref.Outputs) > 0) {
		return nil, fmt.Errorf("workflow has both 'file' and inline fields; use one or the other")
	}

	// Load from external file if specified
	if ref.File != "" {
		wf, err := loadFromFile(vaultPath, name, ref.File)
		if err != nil {
			return nil, err
		}
		if ref.Description != "" {
			wf.Description = ref.Description
		}
		return wf, nil
	}

	// Use inline definition
	wf, err := buildWorkflow(name, ref.Description, ref.Inputs, ref.Context, ref.Prompt, ref.Outputs, ref.Steps)
	if err != nil {
		return nil, err
	}
	if err := validateWorkflow(wf); err != nil {
		return nil, err
	}
	return wf, nil
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

	wf, err := buildWorkflow(name, def.Description, def.Inputs, def.Context, def.Prompt, def.Outputs, def.Steps)
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
	ctx map[string]*config.WorkflowContextItem,
	prompt string,
	outputs map[string]*config.WorkflowPromptOutput,
	steps []*config.WorkflowStep,
) (*Workflow, error) {
	// Prefer explicit steps if present (legacy/advanced).
	if len(steps) > 0 {
		return &Workflow{
			Name:        name,
			Description: description,
			Inputs:      inputs,
			Steps:       steps,
		}, nil
	}

	// Otherwise, simplified prompt workflow.
	if prompt == "" {
		return nil, fmt.Errorf("workflow must have either 'file' or 'prompt' or 'steps'")
	}

	compiled, err := compilePromptWorkflow(ctx, prompt, outputs)
	if err != nil {
		return nil, err
	}

	return &Workflow{
		Name:        name,
		Description: description,
		Inputs:      inputs,
		Context:     ctx,
		Prompt:      prompt,
		Outputs:     outputs,
		Steps:       compiled,
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
		case "query":
			if s.RQL == "" {
				return fmt.Errorf("step '%s' (query) missing rql", s.ID)
			}
		case "read":
			if s.Ref == "" {
				return fmt.Errorf("step '%s' (read) missing ref", s.ID)
			}
		case "search":
			if s.Term == "" {
				return fmt.Errorf("step '%s' (search) missing term", s.ID)
			}
		case "backlinks":
			if s.Target == "" {
				return fmt.Errorf("step '%s' (backlinks) missing target", s.ID)
			}
		case "prompt":
			if s.Template == "" {
				return fmt.Errorf("step '%s' (prompt) missing template", s.ID)
			}
			if len(s.Outputs) > 0 {
				for name, out := range s.Outputs {
					if name == "" {
						return fmt.Errorf("step '%s' (prompt) has empty output name", s.ID)
					}
					if out == nil {
						return fmt.Errorf("step '%s' (prompt) output '%s' is nil", s.ID, name)
					}
					switch out.Type {
					case "markdown":
					default:
						return fmt.Errorf("step '%s' (prompt) output '%s' has unknown type '%s'", s.ID, name, out.Type)
					}
				}
			}
		default:
			return fmt.Errorf("step '%s' has unknown type '%s'", s.ID, s.Type)
		}
	}
	return nil
}

func compilePromptWorkflow(
	ctx map[string]*config.WorkflowContextItem,
	prompt string,
	outputs map[string]*config.WorkflowPromptOutput,
) ([]*config.WorkflowStep, error) {
	var steps []*config.WorkflowStep

	if len(ctx) > 0 {
		keys := make([]string, 0, len(ctx))
		for k := range ctx {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, id := range keys {
			if id == "" {
				return nil, fmt.Errorf("context key cannot be empty")
			}
			if id == "prompt" {
				return nil, fmt.Errorf("context key 'prompt' is reserved")
			}
			item := ctx[id]
			if item == nil {
				return nil, fmt.Errorf("context item '%s' is nil", id)
			}

			set := 0
			if item.Query != "" {
				set++
			}
			if item.Read != "" {
				set++
			}
			if item.Backlinks != "" {
				set++
			}
			if item.Search != "" {
				set++
			}
			if set != 1 {
				return nil, fmt.Errorf("context item '%s' must set exactly one of query/read/backlinks/search", id)
			}

			switch {
			case item.Query != "":
				steps = append(steps, &config.WorkflowStep{ID: id, Type: "query", RQL: item.Query})
			case item.Read != "":
				steps = append(steps, &config.WorkflowStep{ID: id, Type: "read", Ref: item.Read})
			case item.Backlinks != "":
				steps = append(steps, &config.WorkflowStep{ID: id, Type: "backlinks", Target: item.Backlinks})
			case item.Search != "":
				st := &config.WorkflowStep{ID: id, Type: "search", Term: item.Search}
				if item.Limit > 0 {
					st.Limit = item.Limit
				}
				steps = append(steps, st)
			}
		}
	}

	steps = append(steps, &config.WorkflowStep{
		ID:       "prompt",
		Type:     "prompt",
		Template: prompt,
		Outputs:  outputs,
	})

	return steps, nil
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
