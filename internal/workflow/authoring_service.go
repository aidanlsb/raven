package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
)

type StepMutationAction string

const (
	StepMutationAdd    StepMutationAction = "add"
	StepMutationUpdate StepMutationAction = "update"
	StepMutationRemove StepMutationAction = "remove"
)

type StepMutationRequest struct {
	WorkflowName string
	Action       StepMutationAction
	TargetStepID string
	Step         *config.WorkflowStep
	Position     PositionHint
}

type StepMutationResult struct {
	WorkflowName string
	FileRef      string
	Action       StepMutationAction
	StepID       string
	PreviousID   string
	Index        int
	Step         *config.WorkflowStep
}

type AddWorkflowRequest struct {
	Name string
	File string
}

type AddWorkflowResult struct {
	Workflow *Workflow
	FileRef  string
	Source   string
}

type ScaffoldWorkflowRequest struct {
	Name        string
	File        string
	Description string
	Force       bool
}

type ScaffoldWorkflowResult struct {
	Workflow   *Workflow
	FileRef    string
	Source     string
	Scaffolded bool
}

type RemoveWorkflowRequest struct {
	Name string
}

type RemoveWorkflowResult struct {
	Name    string
	Removed bool
}

type ValidationItem struct {
	Name  string `json:"name"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

type ValidateWorkflowsRequest struct {
	Name string
}

type ValidateWorkflowsResult struct {
	Valid   bool             `json:"valid"`
	Checked int              `json:"checked"`
	Invalid int              `json:"invalid"`
	Results []ValidationItem `json:"results"`
}

type AuthoringService struct {
	vaultPath string
	vaultCfg  *config.VaultConfig
}

func NewAuthoringService(vaultPath string, vaultCfg *config.VaultConfig) *AuthoringService {
	return &AuthoringService{
		vaultPath: vaultPath,
		vaultCfg:  vaultCfg,
	}
}

func (s *AuthoringService) AddWorkflow(req AddWorkflowRequest) (*AddWorkflowResult, error) {
	if s == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "authoring service is nil", nil)
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, newDomainError(CodeInvalidInput, "workflow name cannot be empty", nil)
	}
	if err := validateWorkflowName(name); err != nil {
		return nil, err
	}
	if err := s.ensureVaultConfig(); err != nil {
		return nil, err
	}

	workflowDir := s.vaultCfg.GetWorkflowDirectory()
	rawFileRef := strings.TrimSpace(req.File)
	if rawFileRef == "" {
		return nil, newDomainError(CodeInvalidInput, "--file is required", nil)
	}

	fileRef, err := ResolveWorkflowFileRef(rawFileRef, workflowDir)
	if err != nil {
		return nil, newDomainError(CodeInvalidInput, err.Error(), err)
	}

	loaded, err := s.registerWorkflow(name, fileRef)
	if err != nil {
		return nil, err
	}

	return &AddWorkflowResult{
		Workflow: loaded,
		FileRef:  fileRef,
		Source:   "file",
	}, nil
}

func (s *AuthoringService) ScaffoldWorkflow(req ScaffoldWorkflowRequest) (*ScaffoldWorkflowResult, error) {
	if s == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "authoring service is nil", nil)
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, newDomainError(CodeInvalidInput, "workflow name cannot be empty", nil)
	}
	if err := validateWorkflowName(name); err != nil {
		return nil, err
	}
	if err := s.ensureVaultConfig(); err != nil {
		return nil, err
	}

	workflowDir := s.vaultCfg.GetWorkflowDirectory()
	fileRef := strings.TrimSpace(req.File)
	if fileRef == "" {
		fileRef = fmt.Sprintf("%s%s.yaml", workflowDir, name)
	}

	fileRef, err := ResolveWorkflowFileRef(fileRef, workflowDir)
	if err != nil {
		return nil, newDomainError(CodeInvalidInput, err.Error(), err)
	}

	fullPath := filepath.Join(s.vaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(s.vaultPath, fullPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideVault) {
			return nil, newDomainError(CodeFileOutsideVault, "workflow files must be within the vault", err)
		}
		return nil, newDomainError(CodeWorkflowInvalid, "failed to validate workflow file path", err)
	}

	if _, err := os.Stat(fullPath); err == nil && !req.Force {
		return nil, newDomainError(CodeFileExists, fmt.Sprintf("workflow file already exists: %s", fileRef), nil)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, newDomainError(CodeFileReadError, fmt.Sprintf("stat workflow file %s", fileRef), err)
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, newDomainError(CodeFileWriteError, fmt.Sprintf("create workflow directory for %s", fileRef), err)
	}

	content := buildWorkflowScaffoldYAML(name, req.Description)
	if err := atomicfile.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return nil, newDomainError(CodeFileWriteError, fmt.Sprintf("write scaffold workflow file %s", fileRef), err)
	}

	loaded, err := s.registerWorkflow(name, fileRef)
	if err != nil {
		return nil, err
	}

	return &ScaffoldWorkflowResult{
		Workflow:   loaded,
		FileRef:    fileRef,
		Source:     "file",
		Scaffolded: true,
	}, nil
}

func (s *AuthoringService) RemoveWorkflow(req RemoveWorkflowRequest) (*RemoveWorkflowResult, error) {
	if s == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "authoring service is nil", nil)
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, newDomainError(CodeInvalidInput, "workflow name cannot be empty", nil)
	}
	if err := s.ensureVaultConfig(); err != nil {
		return nil, err
	}

	if len(s.vaultCfg.Workflows) == 0 {
		return nil, newDomainError(CodeWorkflowNotFound, fmt.Sprintf("workflow '%s' not found", name), nil)
	}
	if _, exists := s.vaultCfg.Workflows[name]; !exists {
		return nil, newDomainError(CodeWorkflowNotFound, fmt.Sprintf("workflow '%s' not found", name), nil)
	}

	delete(s.vaultCfg.Workflows, name)
	if len(s.vaultCfg.Workflows) == 0 {
		s.vaultCfg.Workflows = nil
	}
	if err := config.SaveVaultConfig(s.vaultPath, s.vaultCfg); err != nil {
		return nil, newDomainError(CodeFileWriteError, "save vault config", err)
	}

	return &RemoveWorkflowResult{
		Name:    name,
		Removed: true,
	}, nil
}

func (s *AuthoringService) ValidateWorkflows(req ValidateWorkflowsRequest) (*ValidateWorkflowsResult, error) {
	if s == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "authoring service is nil", nil)
	}
	if err := s.ensureVaultConfig(); err != nil {
		return nil, err
	}

	if len(s.vaultCfg.Workflows) == 0 {
		return &ValidateWorkflowsResult{
			Valid:   true,
			Checked: 0,
			Invalid: 0,
			Results: []ValidationItem{},
		}, nil
	}

	var names []string
	name := strings.TrimSpace(req.Name)
	if name != "" {
		if _, ok := s.vaultCfg.Workflows[name]; !ok {
			return nil, newDomainError(CodeWorkflowNotFound, fmt.Sprintf("workflow '%s' not found", name), nil)
		}
		names = []string{name}
	} else {
		names = make([]string, 0, len(s.vaultCfg.Workflows))
		for workflowName := range s.vaultCfg.Workflows {
			names = append(names, workflowName)
		}
		sort.Strings(names)
	}

	results := make([]ValidationItem, 0, len(names))
	invalidCount := 0
	for _, workflowName := range names {
		_, loadErr := LoadWithConfig(s.vaultPath, workflowName, s.vaultCfg.Workflows[workflowName], s.vaultCfg)
		item := ValidationItem{
			Name:  workflowName,
			Valid: loadErr == nil,
		}
		if loadErr != nil {
			item.Error = loadErr.Error()
			invalidCount++
		}
		results = append(results, item)
	}

	return &ValidateWorkflowsResult{
		Valid:   invalidCount == 0,
		Checked: len(results),
		Invalid: invalidCount,
		Results: results,
	}, nil
}

func (s *AuthoringService) MutateStep(req StepMutationRequest) (*StepMutationResult, error) {
	if s == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "authoring service is nil", nil)
	}
	if strings.TrimSpace(req.WorkflowName) == "" {
		return nil, newDomainError(CodeInvalidInput, "workflow name cannot be empty", nil)
	}
	if s.vaultCfg == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "vault config is nil", nil)
	}

	def, fileRef, fullPath, err := s.loadWorkflowDefinition(req.WorkflowName)
	if err != nil {
		return nil, err
	}

	result, err := applyStepMutation(def, req)
	if err != nil {
		return nil, err
	}

	wf, err := buildWorkflow(req.WorkflowName, def.Description, def.Inputs, def.Steps)
	if err != nil {
		return nil, newDomainError(CodeWorkflowInvalid, err.Error(), err)
	}
	if err := validateWorkflow(wf); err != nil {
		return nil, newDomainError(CodeWorkflowInvalid, err.Error(), err)
	}

	encoded, err := yaml.Marshal(def)
	if err != nil {
		return nil, newDomainError(CodeWorkflowInvalid, "encode workflow definition", err)
	}
	if err := atomicfile.WriteFile(fullPath, encoded, 0o644); err != nil {
		return nil, newDomainError(CodeFileWriteError, fmt.Sprintf("write workflow file %s", fileRef), err)
	}

	result.WorkflowName = req.WorkflowName
	result.FileRef = fileRef
	return result, nil
}

func (s *AuthoringService) ensureVaultConfig() error {
	if s.vaultCfg != nil {
		return nil
	}

	vaultCfg, err := config.LoadVaultConfig(s.vaultPath)
	if err != nil {
		return newDomainError(CodeFileReadError, "load vault config", err)
	}
	s.vaultCfg = vaultCfg
	return nil
}

func validateWorkflowName(name string) error {
	if name == "runs" {
		return newDomainError(CodeInvalidInput, "workflow name 'runs' is reserved for workflows.runs config", nil)
	}
	return nil
}

func (s *AuthoringService) registerWorkflow(name, fileRef string) (*Workflow, error) {
	if s.vaultCfg.Workflows == nil {
		s.vaultCfg.Workflows = make(map[string]*config.WorkflowRef)
	}
	if _, exists := s.vaultCfg.Workflows[name]; exists {
		return nil, newDomainError(CodeDuplicateName, fmt.Sprintf("workflow '%s' already exists", name), nil)
	}

	ref := &config.WorkflowRef{File: fileRef}
	loaded, err := LoadWithConfig(s.vaultPath, name, ref, s.vaultCfg)
	if err != nil {
		return nil, newDomainError(CodeWorkflowInvalid, err.Error(), err)
	}

	s.vaultCfg.Workflows[name] = ref
	if err := config.SaveVaultConfig(s.vaultPath, s.vaultCfg); err != nil {
		return nil, newDomainError(CodeFileWriteError, "save vault config", err)
	}

	return loaded, nil
}

func buildWorkflowScaffoldYAML(name, description string) string {
	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = fmt.Sprintf("Scaffolded workflow: %s", name)
	}

	return fmt.Sprintf(`description: %q
inputs:
  topic:
    type: string
    required: true
    description: "Question or topic to analyze"
steps:
  - id: context
    type: tool
    tool: raven_search
    arguments:
      query: "{{inputs.topic}}"
      limit: 10
  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Return JSON: {"outputs":{"markdown":"..."}}

      Answer this request using my notes:
      {{inputs.topic}}

      ## Relevant context
      {{steps.context.data.results}}
`, desc)
}

func (s *AuthoringService) loadWorkflowDefinition(
	workflowName string,
) (*externalWorkflowDef, string, string, error) {
	if len(s.vaultCfg.Workflows) == 0 {
		return nil, "", "", newDomainError(CodeWorkflowNotFound, "no workflows defined in raven.yaml", nil)
	}

	ref, ok := s.vaultCfg.Workflows[workflowName]
	if !ok {
		return nil, "", "", newDomainError(CodeWorkflowNotFound, fmt.Sprintf("workflow '%s' not found", workflowName), nil)
	}
	if ref == nil {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, fmt.Sprintf("workflow '%s' reference is nil", workflowName), nil)
	}
	if strings.TrimSpace(ref.File) == "" {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, fmt.Sprintf("workflow '%s' has no file reference", workflowName), nil)
	}

	fileRef, err := ResolveWorkflowFileRef(ref.File, s.vaultCfg.GetWorkflowDirectory())
	if err != nil {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, err.Error(), err)
	}
	fullPath := filepath.Join(s.vaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(s.vaultPath, fullPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideVault) {
			return nil, "", "", newDomainError(CodeFileOutsideVault, err.Error(), err)
		}
		return nil, "", "", newDomainError(CodeWorkflowInvalid, "failed to validate workflow file path", err)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", "", newDomainError(CodeFileNotFound, fmt.Sprintf("workflow file not found: %s", fileRef), err)
		}
		return nil, "", "", newDomainError(CodeFileReadError, fmt.Sprintf("read workflow file %s", fileRef), err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, "failed to parse workflow file", err)
	}
	if len(root.Content) == 0 {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, "failed to parse workflow file: empty document", nil)
	}
	if err := rejectLegacyTopLevelKeys(root.Content[0]); err != nil {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, err.Error(), err)
	}

	var def externalWorkflowDef
	if err := root.Content[0].Decode(&def); err != nil {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, "failed to decode workflow file", err)
	}
	if def.Steps == nil {
		def.Steps = []*config.WorkflowStep{}
	}

	return &def, fileRef, fullPath, nil
}

func applyStepMutation(def *externalWorkflowDef, req StepMutationRequest) (*StepMutationResult, error) {
	if def == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "workflow definition is nil", nil)
	}

	switch req.Action {
	case StepMutationAdd:
		return applyStepAdd(def, req)
	case StepMutationUpdate:
		return applyStepUpdate(def, req)
	case StepMutationRemove:
		return applyStepRemove(def, req)
	default:
		return nil, newDomainError(CodeInvalidInput, fmt.Sprintf("unknown step mutation action: %s", req.Action), nil)
	}
}

func applyStepAdd(def *externalWorkflowDef, req StepMutationRequest) (*StepMutationResult, error) {
	if req.Step == nil {
		return nil, newDomainError(CodeInvalidInput, "step payload is required", nil)
	}
	step := copyStep(req.Step)
	step.ID = strings.TrimSpace(step.ID)
	step.Type = strings.TrimSpace(step.Type)
	if step.ID == "" {
		return nil, newDomainError(CodeInvalidInput, "step id is required", nil)
	}
	if FindStepIndexInSteps(def.Steps, step.ID) >= 0 {
		return nil, newDomainError(CodeDuplicateName, fmt.Sprintf("step '%s' already exists", step.ID), nil)
	}

	insertAt, err := ResolveInsertIndex(def.Steps, req.Position)
	if err != nil {
		return nil, err
	}

	def.Steps = insertStepAt(def.Steps, insertAt, step)
	return &StepMutationResult{
		Action: req.Action,
		StepID: step.ID,
		Step:   step,
		Index:  insertAt,
	}, nil
}

func applyStepUpdate(def *externalWorkflowDef, req StepMutationRequest) (*StepMutationResult, error) {
	targetID := strings.TrimSpace(req.TargetStepID)
	if targetID == "" {
		return nil, newDomainError(CodeInvalidInput, "target step id is required", nil)
	}
	if req.Step == nil {
		return nil, newDomainError(CodeInvalidInput, "step payload is required", nil)
	}

	targetIdx := FindStepIndexInSteps(def.Steps, targetID)
	if targetIdx < 0 {
		err := newDomainError(CodeRefNotFound, fmt.Sprintf("step '%s' not found", targetID), nil)
		err.StepID = targetID
		return nil, err
	}

	updated := copyStep(req.Step)
	updated.ID = strings.TrimSpace(updated.ID)
	updated.Type = strings.TrimSpace(updated.Type)
	if updated.ID == "" {
		updated.ID = targetID
	}
	if updated.ID != targetID && FindStepIndexInSteps(def.Steps, updated.ID) >= 0 {
		return nil, newDomainError(CodeDuplicateName, fmt.Sprintf("step id '%s' already exists", updated.ID), nil)
	}

	def.Steps[targetIdx] = updated
	return &StepMutationResult{
		Action:     req.Action,
		StepID:     updated.ID,
		PreviousID: targetID,
		Step:       updated,
		Index:      targetIdx,
	}, nil
}

func applyStepRemove(def *externalWorkflowDef, req StepMutationRequest) (*StepMutationResult, error) {
	targetID := strings.TrimSpace(req.TargetStepID)
	if targetID == "" {
		return nil, newDomainError(CodeInvalidInput, "target step id is required", nil)
	}
	targetIdx := FindStepIndexInSteps(def.Steps, targetID)
	if targetIdx < 0 {
		err := newDomainError(CodeRefNotFound, fmt.Sprintf("step '%s' not found", targetID), nil)
		err.StepID = targetID
		return nil, err
	}

	def.Steps = append(def.Steps[:targetIdx], def.Steps[targetIdx+1:]...)
	return &StepMutationResult{
		Action: req.Action,
		StepID: targetID,
		Index:  targetIdx,
	}, nil
}

func insertStepAt(steps []*config.WorkflowStep, idx int, step *config.WorkflowStep) []*config.WorkflowStep {
	if idx <= 0 {
		return append([]*config.WorkflowStep{step}, steps...)
	}
	if idx >= len(steps) {
		return append(steps, step)
	}
	steps = append(steps, nil)
	copy(steps[idx+1:], steps[idx:])
	steps[idx] = step
	return steps
}

func copyStep(step *config.WorkflowStep) *config.WorkflowStep {
	if step == nil {
		return nil
	}
	clone := *step
	return &clone
}
