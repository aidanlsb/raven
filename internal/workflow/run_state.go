package workflow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aidanlsb/raven/internal/config"
)

// AgentOutputEnvelope is the required payload for workflow continuation.
type AgentOutputEnvelope struct {
	Outputs map[string]interface{} `json:"outputs"`
}

func NewRunState(wf *Workflow, inputs map[string]interface{}) (*WorkflowRunState, error) {
	if wf == nil {
		return nil, fmt.Errorf("workflow is nil")
	}

	hash, err := WorkflowHash(wf)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	state := &WorkflowRunState{
		Version:      1,
		RunID:        newRunID(),
		WorkflowName: wf.Name,
		WorkflowHash: hash,
		Status:       RunStatusRunning,
		Cursor:       0,
		Inputs:       cloneInterfaceMap(inputs),
		Steps:        map[string]interface{}{},
		CreatedAt:    now,
		UpdatedAt:    now,
		Revision:     1,
	}
	return state, nil
}

func WorkflowHash(wf *Workflow) (string, error) {
	if wf == nil {
		return "", fmt.Errorf("workflow is nil")
	}
	payload, err := json.Marshal(wf)
	if err != nil {
		return "", fmt.Errorf("marshal workflow for hash: %w", err)
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func FindStepIndexByID(wf *Workflow, stepID string) int {
	if wf == nil || stepID == "" {
		return -1
	}
	for i, step := range wf.Steps {
		if step != nil && step.ID == stepID {
			return i
		}
	}
	return -1
}

func ApplyAgentOutputs(wf *Workflow, state *WorkflowRunState, env AgentOutputEnvelope) error {
	if wf == nil {
		return fmt.Errorf("workflow is nil")
	}
	if state == nil {
		return fmt.Errorf("run state is nil")
	}
	if state.Status != RunStatusAwaitingAgent {
		return fmt.Errorf("run is not awaiting agent output")
	}
	if state.AwaitingStep == "" {
		return fmt.Errorf("run is awaiting agent output but awaiting_step_id is empty")
	}
	if env.Outputs == nil {
		return fmt.Errorf("agent output must include an 'outputs' object")
	}

	stepIdx := FindStepIndexByID(wf, state.AwaitingStep)
	if stepIdx < 0 {
		return fmt.Errorf("awaiting step '%s' not found in workflow", state.AwaitingStep)
	}
	step := wf.Steps[stepIdx]
	if step == nil || step.Type != "agent" {
		return fmt.Errorf("awaiting step '%s' is not an agent step", state.AwaitingStep)
	}

	if err := ValidateAgentOutputs(step.Outputs, env.Outputs); err != nil {
		return err
	}

	stepState, _ := state.Steps[state.AwaitingStep].(map[string]interface{})
	if stepState == nil {
		stepState = map[string]interface{}{}
	}

	// Store canonical interpolation location and validated copy.
	stepState["outputs"] = cloneInterfaceMap(env.Outputs)
	stepState["validated_outputs"] = cloneInterfaceMap(env.Outputs)
	state.Steps[state.AwaitingStep] = stepState

	state.History = append(state.History, RunHistoryEvent{
		StepID:   state.AwaitingStep,
		StepType: "agent",
		Status:   "accepted",
		At:       time.Now().UTC(),
	})

	state.Status = RunStatusRunning
	state.Cursor = stepIdx + 1
	state.AwaitingStep = ""
	state.UpdatedAt = time.Now().UTC()
	return nil
}

func ValidateAgentOutputs(contract map[string]*config.WorkflowPromptOutput, outputs map[string]interface{}) error {
	if len(contract) == 0 {
		// No declared outputs: require empty object to keep contract strict.
		if len(outputs) > 0 {
			return fmt.Errorf("agent output includes undeclared fields")
		}
		return nil
	}

	for key, def := range contract {
		if def == nil {
			return fmt.Errorf("agent output contract for '%s' is nil", key)
		}
		val, ok := outputs[key]
		if def.Required && !ok {
			return fmt.Errorf("missing required agent output: %s", key)
		}
		if !ok {
			continue
		}
		if err := validateOutputType(key, def.Type, val); err != nil {
			return err
		}
	}

	// Strict mode: reject undeclared fields.
	for key := range outputs {
		if _, ok := contract[key]; !ok {
			return fmt.Errorf("undeclared agent output field: %s", key)
		}
	}
	return nil
}

func ApplyRetentionExpiry(state *WorkflowRunState, cfg config.ResolvedWorkflowRunsConfig, now time.Time) {
	if state == nil {
		return
	}
	var days int
	switch state.Status {
	case RunStatusCompleted:
		days = cfg.KeepCompletedForDays
	case RunStatusFailed:
		days = cfg.KeepFailedForDays
	case RunStatusAwaitingAgent:
		days = cfg.KeepAwaitingForDays
	default:
		state.ExpiresAt = nil
		return
	}
	if days <= 0 {
		state.ExpiresAt = nil
		return
	}
	exp := now.Add(time.Duration(days) * 24 * time.Hour).UTC()
	state.ExpiresAt = &exp
}

func validateOutputType(fieldName, expectedType string, value interface{}) error {
	switch expectedType {
	case "markdown", "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("agent output '%s' must be string", fieldName)
		}
	case "number":
		switch value.(type) {
		case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		default:
			return fmt.Errorf("agent output '%s' must be number", fieldName)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("agent output '%s' must be bool", fieldName)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("agent output '%s' must be object", fieldName)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("agent output '%s' must be array", fieldName)
		}
	default:
		return fmt.Errorf("agent output '%s' has unsupported declared type '%s'", fieldName, expectedType)
	}
	return nil
}

func newRunID() string {
	var b [10]byte
	_, _ = rand.Read(b[:])
	return "wrf_" + hex.EncodeToString(b[:])
}

func cloneInterfaceMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
