package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
)

// externalWorkflowDef is used for parsing external workflow files.
type externalWorkflowDef struct {
	Description string                           `yaml:"description,omitempty"`
	Inputs      map[string]*config.WorkflowInput `yaml:"inputs,omitempty"`
	Steps       []*config.WorkflowStep           `yaml:"steps,omitempty"`
}

// LoadAll loads all workflows from the keep configuration.
func LoadAll(keepPath string, keepCfg *config.KeepConfig) ([]*Workflow, error) {
	if keepCfg.Workflows == nil {
		return nil, nil
	}

	var workflows []*Workflow
	for name, ref := range keepCfg.Workflows {
		wf, err := LoadWithConfig(keepPath, name, ref, keepCfg)
		if err != nil {
			return nil, fmt.Errorf("workflow '%s': %w", name, err)
		}
		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// LoadWithConfig loads a single workflow by name using keep-level workflow policy.
func LoadWithConfig(keepPath, name string, ref *config.WorkflowRef, keepCfg *config.KeepConfig) (*Workflow, error) {
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

	workflowDir := config.DefaultKeepConfig().GetWorkflowDirectory()
	if keepCfg != nil {
		workflowDir = keepCfg.GetWorkflowDirectory()
	}
	fileRef, err := ResolveWorkflowFileRef(ref.File, workflowDir)
	if err != nil {
		return nil, err
	}

	wf, err := loadFromFile(keepPath, name, fileRef)
	if err != nil {
		return nil, err
	}
	return wf, nil
}

// Load loads a single workflow by name with default config policy.
func Load(keepPath, name string, ref *config.WorkflowRef) (*Workflow, error) {
	return LoadWithConfig(keepPath, name, ref, config.DefaultKeepConfig())
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
		return "", fmt.Errorf("workflow file path cannot escape the keep")
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
func loadFromFile(keepPath, name, filePath string) (*Workflow, error) {
	// Security: ensure path is within keep
	fullPath := filepath.Join(keepPath, filePath)
	if err := paths.ValidateWithinKeep(keepPath, fullPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideKeep) {
			return nil, fmt.Errorf("workflow file must be within keep")
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
			if err := validateStepOutputContract(s.ID, "agent", s.Outputs); err != nil {
				return err
			}
		case "tool":
			if s.Tool == "" {
				return fmt.Errorf("step '%s' (tool) missing tool", s.ID)
			}
			if err := validateToolName(s.Tool); err != nil {
				return fmt.Errorf("step '%s' (tool) %w", s.ID, err)
			}
		case "foreach":
			if err := validateForEachStep(s); err != nil {
				return err
			}
		case "switch":
			if err := validateSwitchStep(s); err != nil {
				return err
			}
		default:
			return fmt.Errorf("step '%s' has unknown type '%s'", s.ID, s.Type)
		}
	}
	return nil
}

var workflowVarNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateForEachStep(step *config.WorkflowStep) error {
	if step == nil {
		return fmt.Errorf("foreach step is nil")
	}
	if step.ForEach == nil {
		return fmt.Errorf("step '%s' (foreach) missing foreach config", step.ID)
	}

	def := step.ForEach
	if strings.TrimSpace(def.Items) == "" {
		return fmt.Errorf("step '%s' (foreach) missing foreach.items", step.ID)
	}
	if len(def.Steps) == 0 {
		return fmt.Errorf("step '%s' (foreach) must define foreach.steps", step.ID)
	}

	itemVar := strings.TrimSpace(def.As)
	if itemVar == "" {
		itemVar = "item"
	}
	indexVar := strings.TrimSpace(def.IndexAs)
	if indexVar == "" {
		indexVar = "index"
	}
	if !workflowVarNamePattern.MatchString(itemVar) {
		return fmt.Errorf("step '%s' (foreach) has invalid foreach.as '%s'", step.ID, def.As)
	}
	if !workflowVarNamePattern.MatchString(indexVar) {
		return fmt.Errorf("step '%s' (foreach) has invalid foreach.index_as '%s'", step.ID, def.IndexAs)
	}
	if itemVar == indexVar {
		return fmt.Errorf("step '%s' (foreach) variable names conflict: '%s'", step.ID, itemVar)
	}

	onError := strings.TrimSpace(def.OnError)
	if onError != "" && onError != "fail_fast" && onError != "continue" {
		return fmt.Errorf("step '%s' (foreach) has invalid foreach.on_error '%s'", step.ID, def.OnError)
	}

	seen := make(map[string]struct{}, len(def.Steps))
	for i, nested := range def.Steps {
		if nested == nil {
			return fmt.Errorf("step '%s' (foreach) nested step %d is nil", step.ID, i)
		}
		nestedID := strings.TrimSpace(nested.ID)
		if nestedID == "" {
			return fmt.Errorf("step '%s' (foreach) nested step %d missing id", step.ID, i)
		}
		if _, ok := seen[nestedID]; ok {
			return fmt.Errorf("step '%s' (foreach) has duplicate nested step id '%s'", step.ID, nestedID)
		}
		seen[nestedID] = struct{}{}

		if nested.Type != "tool" {
			return fmt.Errorf("step '%s' (foreach) nested step '%s' must be type 'tool'", step.ID, nestedID)
		}
		if strings.TrimSpace(nested.Tool) == "" {
			return fmt.Errorf("step '%s' (foreach) nested step '%s' missing tool", step.ID, nestedID)
		}
		if err := validateToolName(nested.Tool); err != nil {
			return fmt.Errorf("step '%s' (foreach) nested step '%s' %w", step.ID, nestedID, err)
		}
	}

	return nil
}

func validateSwitchStep(step *config.WorkflowStep) error {
	if step == nil {
		return fmt.Errorf("switch step is nil")
	}
	if step.Switch == nil {
		return fmt.Errorf("step '%s' (switch) missing switch config", step.ID)
	}

	def := step.Switch
	if strings.TrimSpace(def.Value) == "" {
		return fmt.Errorf("step '%s' (switch) missing switch.value", step.ID)
	}
	if len(def.Cases) == 0 {
		return fmt.Errorf("step '%s' (switch) must define switch.cases", step.ID)
	}
	if def.Default == nil {
		return fmt.Errorf("step '%s' (switch) must define switch.default", step.ID)
	}
	if err := validateStepOutputContract(step.ID, "switch", def.Outputs); err != nil {
		return err
	}

	requireEmit := len(def.Outputs) > 0
	for label, branch := range def.Cases {
		if strings.TrimSpace(label) == "" {
			return fmt.Errorf("step '%s' (switch) has empty case label", step.ID)
		}
		if err := validateSwitchBranch(step.ID, fmt.Sprintf("case '%s'", label), branch, requireEmit, def.Outputs); err != nil {
			return err
		}
	}
	if err := validateSwitchBranch(step.ID, "default", def.Default, requireEmit, def.Outputs); err != nil {
		return err
	}
	return nil
}

func validateSwitchBranch(
	stepID string,
	branchName string,
	branch *config.WorkflowSwitchCase,
	requireEmit bool,
	contract map[string]*config.WorkflowPromptOutput,
) error {
	if branch == nil {
		return fmt.Errorf("step '%s' (switch) %s branch is nil", stepID, branchName)
	}
	if len(branch.Steps) == 0 && len(branch.Emit) == 0 {
		return fmt.Errorf("step '%s' (switch) %s must define steps or emit", stepID, branchName)
	}

	seen := make(map[string]struct{}, len(branch.Steps))
	for i, nested := range branch.Steps {
		if nested == nil {
			return fmt.Errorf("step '%s' (switch) %s nested step %d is nil", stepID, branchName, i)
		}
		nestedID := strings.TrimSpace(nested.ID)
		if nestedID == "" {
			return fmt.Errorf("step '%s' (switch) %s nested step %d missing id", stepID, branchName, i)
		}
		if _, ok := seen[nestedID]; ok {
			return fmt.Errorf("step '%s' (switch) %s has duplicate nested step id '%s'", stepID, branchName, nestedID)
		}
		seen[nestedID] = struct{}{}

		switch nested.Type {
		case "tool":
			if strings.TrimSpace(nested.Tool) == "" {
				return fmt.Errorf("step '%s' (switch) %s nested step '%s' missing tool", stepID, branchName, nestedID)
			}
			if err := validateToolName(nested.Tool); err != nil {
				return fmt.Errorf("step '%s' (switch) %s nested step '%s' %w", stepID, branchName, nestedID, err)
			}
		case "foreach":
			if err := validateForEachStep(nested); err != nil {
				return err
			}
		default:
			return fmt.Errorf(
				"step '%s' (switch) %s nested step '%s' must be type 'tool' or 'foreach'",
				stepID,
				branchName,
				nestedID,
			)
		}
	}

	if requireEmit {
		if len(branch.Emit) == 0 {
			return fmt.Errorf("step '%s' (switch) %s missing emit", stepID, branchName)
		}
		for key := range branch.Emit {
			if _, ok := contract[key]; !ok {
				return fmt.Errorf(
					"step '%s' (switch) %s emit has undeclared field '%s'",
					stepID,
					branchName,
					key,
				)
			}
		}
		for name, out := range contract {
			if out != nil && out.Required {
				if _, ok := branch.Emit[name]; !ok {
					return fmt.Errorf("step '%s' (switch) %s emit missing required field '%s'", stepID, branchName, name)
				}
			}
		}
	}

	return nil
}

func validateStepOutputContract(
	stepID string,
	stepType string,
	outputs map[string]*config.WorkflowPromptOutput,
) error {
	if len(outputs) == 0 {
		return nil
	}
	for name, out := range outputs {
		if name == "" {
			return fmt.Errorf("step '%s' (%s) has empty output name", stepID, stepType)
		}
		if out == nil {
			return fmt.Errorf("step '%s' (%s) output '%s' is nil", stepID, stepType, name)
		}
		if !isValidOutputType(out.Type) {
			return fmt.Errorf("step '%s' (%s) output '%s' has unknown type '%s'", stepID, stepType, name, out.Type)
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
	_, err := parseOutputType(outputType)
	return err == nil
}

func validateToolName(tool string) error {
	if _, ok := commands.ResolveToolCommandID(strings.TrimSpace(tool)); ok {
		return nil
	}
	return fmt.Errorf("references unknown tool '%s'", tool)
}

// Get retrieves a workflow by name from the keep configuration.
func Get(keepPath, name string, keepCfg *config.KeepConfig) (*Workflow, error) {
	if keepCfg.Workflows == nil {
		return nil, fmt.Errorf("no workflows defined in raven.yaml")
	}

	ref, ok := keepCfg.Workflows[name]
	if !ok {
		return nil, fmt.Errorf("workflow '%s' not found", name)
	}

	return LoadWithConfig(keepPath, name, ref, keepCfg)
}

// List returns all workflow names and descriptions.
func List(keepPath string, keepCfg *config.KeepConfig) ([]*ListItem, error) {
	if keepCfg.Workflows == nil {
		return nil, nil
	}

	var items []*ListItem
	for name, ref := range keepCfg.Workflows {
		item := &ListItem{
			Name: name,
		}

		// Load to get full definition (handles file references)
		wf, err := LoadWithConfig(keepPath, name, ref, keepCfg)
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
