package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

type AuthoringService struct {
	keepPath string
	keepCfg  *config.KeepConfig
}

func NewAuthoringService(keepPath string, keepCfg *config.KeepConfig) *AuthoringService {
	return &AuthoringService{
		keepPath: keepPath,
		keepCfg:  keepCfg,
	}
}

func (s *AuthoringService) MutateStep(req StepMutationRequest) (*StepMutationResult, error) {
	if s == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "authoring service is nil", nil)
	}
	if strings.TrimSpace(req.WorkflowName) == "" {
		return nil, newDomainError(CodeInvalidInput, "workflow name cannot be empty", nil)
	}
	if s.keepCfg == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "keep config is nil", nil)
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

func (s *AuthoringService) loadWorkflowDefinition(
	workflowName string,
) (*externalWorkflowDef, string, string, error) {
	if len(s.keepCfg.Workflows) == 0 {
		return nil, "", "", newDomainError(CodeWorkflowNotFound, "no workflows defined in raven.yaml", nil)
	}

	ref, ok := s.keepCfg.Workflows[workflowName]
	if !ok {
		return nil, "", "", newDomainError(CodeWorkflowNotFound, fmt.Sprintf("workflow '%s' not found", workflowName), nil)
	}
	if ref == nil {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, fmt.Sprintf("workflow '%s' reference is nil", workflowName), nil)
	}
	if strings.TrimSpace(ref.File) == "" {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, fmt.Sprintf("workflow '%s' has no file reference", workflowName), nil)
	}

	fileRef, err := ResolveWorkflowFileRef(ref.File, s.keepCfg.GetWorkflowDirectory())
	if err != nil {
		return nil, "", "", newDomainError(CodeWorkflowInvalid, err.Error(), err)
	}
	fullPath := filepath.Join(s.keepPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinKeep(s.keepPath, fullPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideKeep) {
			return nil, "", "", newDomainError(CodeFileOutsideKeep, err.Error(), err)
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
	if findStepIndexByID(def.Steps, step.ID) >= 0 {
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

	targetIdx := findStepIndexByID(def.Steps, targetID)
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
	if updated.ID != targetID && findStepIndexByID(def.Steps, updated.ID) >= 0 {
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
	targetIdx := findStepIndexByID(def.Steps, targetID)
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

func findStepIndexByID(steps []*config.WorkflowStep, stepID string) int {
	for i, step := range steps {
		if step == nil {
			continue
		}
		if strings.TrimSpace(step.ID) == stepID {
			return i
		}
	}
	return -1
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
