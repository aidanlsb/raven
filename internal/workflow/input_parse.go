package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
)

func ParseRunStatusFilter(raw string) (map[RunStatus]bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	statuses := map[RunStatus]bool{}
	for _, part := range strings.Split(raw, ",") {
		status := RunStatus(strings.TrimSpace(part))
		switch status {
		case RunStatusRunning, RunStatusAwaitingAgent, RunStatusCompleted, RunStatusFailed:
			statuses[status] = true
		default:
			return nil, fmt.Errorf("unknown status: %s", part)
		}
	}
	return statuses, nil
}

func ParseOlderThan(raw string) (*time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil {
			return nil, fmt.Errorf("invalid days duration: %s", raw)
		}
		dur := time.Duration(days) * 24 * time.Hour
		return &dur, nil
	}

	dur, err := time.ParseDuration(raw)
	if err != nil {
		return nil, err
	}
	return &dur, nil
}

func ParseInputs(inputFile string, inputJSON interface{}, kvPairs []string) (map[string]interface{}, error) {
	inputs := map[string]interface{}{}

	if strings.TrimSpace(inputFile) != "" {
		obj, err := ReadJSONFileObject(inputFile)
		if err != nil {
			return nil, fmt.Errorf("parse --input-file: %w", err)
		}
		mergeObject(inputs, obj)
	}

	if !isEmptyJSONArg(inputJSON) {
		obj, err := ParseJSONObject(inputJSON)
		if err != nil {
			return nil, fmt.Errorf("parse --input-json: %w", err)
		}
		mergeObject(inputs, obj)
	}

	for _, pair := range kvPairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --input value: %s (expected key=value)", pair)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("input key cannot be empty")
		}
		inputs[key] = parts[1]
	}

	return inputs, nil
}

func ParseAgentOutputEnvelope(outputFile string, outputJSON interface{}, outputInline string) (AgentOutputEnvelope, error) {
	if strings.TrimSpace(outputFile) == "" && isEmptyJSONArg(outputJSON) && strings.TrimSpace(outputInline) == "" {
		return AgentOutputEnvelope{}, fmt.Errorf("provide --agent-output-json, --agent-output, or --agent-output-file")
	}

	var (
		obj map[string]interface{}
		err error
	)
	switch {
	case !isEmptyJSONArg(outputJSON):
		obj, err = ParseJSONObject(outputJSON)
		if err != nil {
			return AgentOutputEnvelope{}, err
		}
	case strings.TrimSpace(outputInline) != "":
		obj, err = ParseJSONObject(outputInline)
		if err != nil {
			return AgentOutputEnvelope{}, err
		}
	case strings.TrimSpace(outputFile) != "":
		obj, err = ReadJSONFileObject(outputFile)
		if err != nil {
			return AgentOutputEnvelope{}, err
		}
	}

	rawOutputs, ok := obj["outputs"]
	if !ok {
		return AgentOutputEnvelope{}, fmt.Errorf("agent output must contain an 'outputs' key")
	}

	outputs, ok := rawOutputs.(map[string]interface{})
	if !ok {
		return AgentOutputEnvelope{}, fmt.Errorf("'outputs' must be an object")
	}

	return AgentOutputEnvelope{Outputs: outputs}, nil
}

func ParseStepObject(raw interface{}, requireID bool) (*config.WorkflowStep, error) {
	obj, err := ParseJSONObject(raw)
	if err != nil {
		return nil, err
	}
	if err := validateStepJSONKeys(obj); err != nil {
		return nil, err
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	var step config.WorkflowStep
	if err := json.Unmarshal(data, &step); err != nil {
		return nil, err
	}

	if requireID && strings.TrimSpace(step.ID) == "" {
		return nil, fmt.Errorf("step id is required")
	}

	return &step, nil
}

type stepBatchInput struct {
	Operations []stepBatchOperationInput `json:"operations"`
}

type stepBatchOperationInput struct {
	Action string          `json:"action"`
	StepID string          `json:"step_id,omitempty"`
	Step   json.RawMessage `json:"step,omitempty"`
	Patch  json.RawMessage `json:"patch,omitempty"`
	Before string          `json:"before,omitempty"`
	After  string          `json:"after,omitempty"`
}

func ParseStepBatchMutations(raw interface{}) ([]StepMutationRequest, error) {
	data, err := normalizeJSONInput(raw)
	if err != nil {
		return nil, err
	}

	var input stepBatchInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing content")
		}
		return nil, err
	}
	if len(input.Operations) == 0 {
		return nil, fmt.Errorf("operations must contain at least one item")
	}

	mutations := make([]StepMutationRequest, 0, len(input.Operations))
	for i, op := range input.Operations {
		mutation, err := parseStepBatchOperation(op)
		if err != nil {
			return nil, fmt.Errorf("operations[%d]: %w", i, err)
		}
		mutations = append(mutations, mutation)
	}
	return mutations, nil
}

func ApplyStepPatch(existing *config.WorkflowStep, patchRaw interface{}) (*config.WorkflowStep, error) {
	if existing == nil {
		return nil, fmt.Errorf("existing step is required")
	}

	patch, err := ParseJSONObject(patchRaw)
	if err != nil {
		return nil, err
	}
	if err := validateStepJSONKeys(patch); err != nil {
		return nil, err
	}

	existingObj, err := toJSONMap(existing)
	if err != nil {
		return nil, err
	}

	merged := cloneObject(existingObj)
	deepMergeObject(merged, patch)

	data, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}

	var step config.WorkflowStep
	if err := json.Unmarshal(data, &step); err != nil {
		return nil, err
	}

	return &step, nil
}

func ParseJSONObject(raw interface{}) (map[string]interface{}, error) {
	switch typed := raw.(type) {
	case nil:
		return nil, fmt.Errorf("expected JSON object")
	case map[string]interface{}:
		obj := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			obj[k] = v
		}
		return obj, nil
	case string:
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(typed), &obj); err != nil {
			return nil, err
		}
		if obj == nil {
			return nil, fmt.Errorf("expected JSON object")
		}
		return obj, nil
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, err
		}
		if obj == nil {
			return nil, fmt.Errorf("expected JSON object")
		}
		return obj, nil
	}
}

func normalizeJSONInput(raw interface{}) ([]byte, error) {
	switch typed := raw.(type) {
	case nil:
		return nil, fmt.Errorf("expected JSON object")
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, fmt.Errorf("expected JSON object")
		}
		return []byte(typed), nil
	default:
		return json.Marshal(typed)
	}
}

func ReadJSONFileObject(path string) (map[string]interface{}, error) {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(content, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return obj, nil
}

func validateStepJSONKeys(obj map[string]interface{}) error {
	allowed := map[string]struct{}{
		"id":          {},
		"type":        {},
		"description": {},
		"rql":         {},
		"ref":         {},
		"term":        {},
		"limit":       {},
		"target":      {},
		"prompt":      {},
		"outputs":     {},
		"tool":        {},
		"arguments":   {},
		"foreach":     {},
		"switch":      {},
	}

	for key := range obj {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown step field: %s", key)
		}
	}
	return nil
}

func isEmptyJSONArg(raw interface{}) bool {
	if raw == nil {
		return true
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

func mergeObject(dst, src map[string]interface{}) {
	for k, v := range src {
		dst[k] = v
	}
}

func deepMergeObject(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		srcMap, srcIsMap := srcVal.(map[string]interface{})
		if !srcIsMap {
			dst[key] = srcVal
			continue
		}

		dstVal, dstExists := dst[key]
		if !dstExists {
			dst[key] = srcMap
			continue
		}

		dstMap, dstIsMap := dstVal.(map[string]interface{})
		if !dstIsMap {
			dst[key] = srcMap
			continue
		}

		deepMergeObject(dstMap, srcMap)
		dst[key] = dstMap
	}
}

func cloneObject(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		if child, ok := value.(map[string]interface{}); ok {
			dst[key] = cloneObject(child)
			continue
		}
		dst[key] = value
	}
	return dst
}

func toJSONMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return out, nil
}

func parseStepBatchOperation(op stepBatchOperationInput) (StepMutationRequest, error) {
	action := StepMutationAction(strings.TrimSpace(op.Action))
	switch action {
	case StepMutationAdd:
		if strings.TrimSpace(op.StepID) != "" {
			return StepMutationRequest{}, fmt.Errorf("step_id is not valid for add")
		}
		if hasJSONPayload(op.Patch) {
			return StepMutationRequest{}, fmt.Errorf("patch is not valid for add")
		}
		if strings.TrimSpace(op.Before) != "" && strings.TrimSpace(op.After) != "" {
			return StepMutationRequest{}, fmt.Errorf("use either before or after, not both")
		}
		if !hasJSONPayload(op.Step) {
			return StepMutationRequest{}, fmt.Errorf("step is required for add")
		}
		stepObj, err := parseRawJSONObject(op.Step)
		if err != nil {
			return StepMutationRequest{}, fmt.Errorf("parse step: %w", err)
		}
		step, err := ParseStepObject(stepObj, true)
		if err != nil {
			return StepMutationRequest{}, err
		}
		return StepMutationRequest{
			Action: action,
			Step:   step,
			Position: PositionHint{
				BeforeStepID: strings.TrimSpace(op.Before),
				AfterStepID:  strings.TrimSpace(op.After),
			},
		}, nil
	case StepMutationUpdate:
		if strings.TrimSpace(op.StepID) == "" {
			return StepMutationRequest{}, fmt.Errorf("step_id is required for update")
		}
		if !hasJSONPayload(op.Patch) {
			return StepMutationRequest{}, fmt.Errorf("patch is required for update")
		}
		if hasJSONPayload(op.Step) {
			return StepMutationRequest{}, fmt.Errorf("step is not valid for update")
		}
		if strings.TrimSpace(op.Before) != "" || strings.TrimSpace(op.After) != "" {
			return StepMutationRequest{}, fmt.Errorf("before/after are only valid for add")
		}
		patchObj, err := parseRawJSONObject(op.Patch)
		if err != nil {
			return StepMutationRequest{}, fmt.Errorf("parse patch: %w", err)
		}
		return StepMutationRequest{
			Action:       action,
			TargetStepID: strings.TrimSpace(op.StepID),
			StepPatch:    patchObj,
		}, nil
	case StepMutationRemove:
		if strings.TrimSpace(op.StepID) == "" {
			return StepMutationRequest{}, fmt.Errorf("step_id is required for remove")
		}
		if hasJSONPayload(op.Step) || hasJSONPayload(op.Patch) {
			return StepMutationRequest{}, fmt.Errorf("step and patch are not valid for remove")
		}
		if strings.TrimSpace(op.Before) != "" || strings.TrimSpace(op.After) != "" {
			return StepMutationRequest{}, fmt.Errorf("before/after are only valid for add")
		}
		return StepMutationRequest{
			Action:       action,
			TargetStepID: strings.TrimSpace(op.StepID),
		}, nil
	default:
		return StepMutationRequest{}, fmt.Errorf("unknown action %q", op.Action)
	}
}

func parseRawJSONObject(raw json.RawMessage) (map[string]interface{}, error) {
	if !hasJSONPayload(raw) {
		return nil, fmt.Errorf("expected JSON object")
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return obj, nil
}

func hasJSONPayload(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && string(trimmed) != "null"
}
