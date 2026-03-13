package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/workflow"
)

type workflowValidationItem struct {
	Name  string `json:"name"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

func (s *Server) resolveDirectWorkflowArgs(args map[string]interface{}) (string, *config.VaultConfig, map[string]interface{}, string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return "", nil, nil, errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return "", nil, nil, errorEnvelope("CONFIG_INVALID", "failed to load vault config", "Fix raven.yaml and try again", nil), true
	}
	return vaultPath, vaultCfg, normalizeArgs(args), "", false
}

func mapWorkflowDomainCodeToCLI(code workflow.Code) string {
	switch code {
	case workflow.CodeInvalidInput:
		return "INVALID_INPUT"
	case workflow.CodeDuplicateName:
		return "DUPLICATE_NAME"
	case workflow.CodeRefNotFound:
		return "REF_NOT_FOUND"
	case workflow.CodeFileNotFound:
		return "FILE_NOT_FOUND"
	case workflow.CodeFileReadError:
		return "FILE_READ_ERROR"
	case workflow.CodeFileWriteError:
		return "FILE_WRITE_ERROR"
	case workflow.CodeFileOutsideVault:
		return "FILE_OUTSIDE_VAULT"
	case workflow.CodeWorkflowNotFound:
		return "WORKFLOW_NOT_FOUND"
	case workflow.CodeWorkflowInvalid:
		return "WORKFLOW_INVALID"
	case workflow.CodeWorkflowChanged:
		return "WORKFLOW_CHANGED"
	case workflow.CodeWorkflowRunNotFound:
		return "WORKFLOW_RUN_NOT_FOUND"
	case workflow.CodeWorkflowNotAwaitingAgent:
		return "WORKFLOW_NOT_AWAITING_AGENT"
	case workflow.CodeWorkflowTerminalState:
		return "WORKFLOW_TERMINAL_STATE"
	case workflow.CodeWorkflowConflict:
		return "WORKFLOW_CONFLICT"
	case workflow.CodeWorkflowStateCorrupt:
		return "WORKFLOW_STATE_CORRUPT"
	case workflow.CodeWorkflowInputInvalid:
		return "WORKFLOW_INPUT_INVALID"
	case workflow.CodeWorkflowAgentOutputInvalid:
		return "WORKFLOW_AGENT_OUTPUT_INVALID"
	case workflow.CodeWorkflowInterpolationError:
		return "WORKFLOW_INTERPOLATION_ERROR"
	case workflow.CodeWorkflowToolExecutionFailed:
		return "WORKFLOW_TOOL_EXECUTION_FAILED"
	default:
		return "INTERNAL_ERROR"
	}
}

func workflowHintForDomainCode(code workflow.Code) string {
	switch code {
	case workflow.CodeWorkflowNotFound:
		return "Use 'rvn workflow list' to see available workflows"
	case workflow.CodeRefNotFound:
		return "Use 'rvn workflow show <name>' to inspect step ids"
	case workflow.CodeWorkflowChanged:
		return "Start a new run to use the latest workflow definition"
	case workflow.CodeWorkflowConflict:
		return "Fetch latest run state and retry"
	case workflow.CodeWorkflowAgentOutputInvalid:
		return "Provide valid agent output JSON with top-level 'outputs'"
	default:
		return ""
	}
}

func mapDirectWorkflowDomainError(err error, fallbackSuggestion string) (string, bool) {
	de, ok := workflow.AsDomainError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), fallbackSuggestion, nil), true
	}

	suggestion := workflowHintForDomainCode(de.Code)
	if suggestion == "" {
		suggestion = fallbackSuggestion
	}
	return errorEnvelope(mapWorkflowDomainCodeToCLI(de.Code), de.Error(), suggestion, de.Details), true
}

func validateWorkflowCreateName(name string) error {
	if name == "runs" {
		return fmt.Errorf("workflow name 'runs' is reserved for workflows.runs config")
	}
	return nil
}

func registerWorkflowInConfig(
	vaultPath string,
	vaultCfg *config.VaultConfig,
	name string,
	ref *config.WorkflowRef,
) (*workflow.Workflow, string, error) {
	if ref == nil {
		return nil, "WORKFLOW_INVALID", fmt.Errorf("workflow reference is nil")
	}

	if vaultCfg == nil {
		loadedCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return nil, "INTERNAL_ERROR", err
		}
		vaultCfg = loadedCfg
	}
	if vaultCfg.Workflows == nil {
		vaultCfg.Workflows = make(map[string]*config.WorkflowRef)
	}
	if _, exists := vaultCfg.Workflows[name]; exists {
		return nil, "DUPLICATE_NAME", fmt.Errorf("workflow '%s' already exists", name)
	}

	loaded, err := workflow.LoadWithConfig(vaultPath, name, ref, vaultCfg)
	if err != nil {
		return nil, "WORKFLOW_INVALID", err
	}

	vaultCfg.Workflows[name] = ref
	if err := config.SaveVaultConfig(vaultPath, vaultCfg); err != nil {
		return nil, "INTERNAL_ERROR", err
	}

	return loaded, "", nil
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

func parseWorkflowRunStatusFilter(raw string) (map[workflow.RunStatus]bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	statuses := map[workflow.RunStatus]bool{}
	for _, part := range strings.Split(raw, ",") {
		status := workflow.RunStatus(strings.TrimSpace(part))
		switch status {
		case workflow.RunStatusRunning, workflow.RunStatusAwaitingAgent, workflow.RunStatusCompleted, workflow.RunStatusFailed, workflow.RunStatusCancelled:
			statuses[status] = true
		default:
			return nil, fmt.Errorf("unknown status: %s", part)
		}
	}
	return statuses, nil
}

func parseWorkflowOlderThan(raw string) (*time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil {
			return nil, fmt.Errorf("invalid days duration: %s", raw)
		}
		duration := time.Duration(days) * 24 * time.Hour
		return &duration, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return nil, err
	}
	return &duration, nil
}

func workflowWarningsToDirect(warnings []workflow.RunStoreWarning) []directWarning {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]directWarning, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, directWarning{
			Code:    warning.Code,
			Message: warning.Message,
			Ref:     warning.File,
		})
	}
	return out
}

func parseJSONObjectArg(raw interface{}) (map[string]interface{}, error) {
	switch typed := raw.(type) {
	case nil:
		return nil, fmt.Errorf("expected JSON object")
	case map[string]interface{}:
		obj := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			obj[k] = v
		}
		return obj, nil
	case map[string]string:
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
		b, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("expected JSON object")
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(b, &obj); err != nil {
			return nil, fmt.Errorf("expected JSON object")
		}
		if obj == nil {
			return nil, fmt.Errorf("expected JSON object")
		}
		return obj, nil
	}
}

func readJSONFileObject(path string) (map[string]interface{}, error) {
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

func mergeObject(dst, src map[string]interface{}) {
	for k, v := range src {
		dst[k] = v
	}
}

func toJSONMap(v interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return out, nil
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

func parseWorkflowInputs(inputFile string, inputJSON interface{}, rawInput interface{}) (map[string]interface{}, error) {
	inputs := map[string]interface{}{}

	if strings.TrimSpace(inputFile) != "" {
		obj, err := readJSONFileObject(inputFile)
		if err != nil {
			return nil, fmt.Errorf("parse --input-file: %w", err)
		}
		mergeObject(inputs, obj)
	}

	if inputJSON != nil {
		obj, err := parseJSONObjectArg(inputJSON)
		if err != nil {
			return nil, fmt.Errorf("parse --input-json: %w", err)
		}
		mergeObject(inputs, obj)
	}

	for _, pair := range keyValuePairs(rawInput) {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --input value: %s (expected key=value)", pair)
		}
		k := strings.TrimSpace(parts[0])
		if k == "" {
			return nil, fmt.Errorf("input key cannot be empty")
		}
		inputs[k] = parts[1]
	}

	return inputs, nil
}

func parseAgentOutputEnvelope(outputFile string, outputJSON interface{}, outputInline string) (workflow.AgentOutputEnvelope, error) {
	if strings.TrimSpace(outputFile) == "" && outputJSON == nil && strings.TrimSpace(outputInline) == "" {
		return workflow.AgentOutputEnvelope{}, fmt.Errorf("provide --agent-output-json, --agent-output, or --agent-output-file")
	}

	var (
		obj map[string]interface{}
		err error
	)
	switch {
	case outputJSON != nil:
		obj, err = parseJSONObjectArg(outputJSON)
		if err != nil {
			return workflow.AgentOutputEnvelope{}, err
		}
	case strings.TrimSpace(outputInline) != "":
		obj, err = parseJSONObjectArg(outputInline)
		if err != nil {
			return workflow.AgentOutputEnvelope{}, err
		}
	case strings.TrimSpace(outputFile) != "":
		obj, err = readJSONFileObject(outputFile)
		if err != nil {
			return workflow.AgentOutputEnvelope{}, err
		}
	}

	rawOutputs, ok := obj["outputs"]
	if !ok {
		return workflow.AgentOutputEnvelope{}, fmt.Errorf("agent output must contain an 'outputs' key")
	}
	outputs, ok := rawOutputs.(map[string]interface{})
	if !ok {
		return workflow.AgentOutputEnvelope{}, fmt.Errorf("'outputs' must be an object")
	}
	return workflow.AgentOutputEnvelope{Outputs: outputs}, nil
}

func validateWorkflowStepJSONKeys(obj map[string]interface{}) error {
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

func parseWorkflowStepObject(raw interface{}, requireID bool) (*config.WorkflowStep, error) {
	obj, err := parseJSONObjectArg(raw)
	if err != nil {
		return nil, err
	}
	if err := validateWorkflowStepJSONKeys(obj); err != nil {
		return nil, err
	}

	stepBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var step config.WorkflowStep
	if err := json.Unmarshal(stepBytes, &step); err != nil {
		return nil, err
	}

	step.ID = strings.TrimSpace(step.ID)
	step.Type = strings.TrimSpace(step.Type)
	if requireID && step.ID == "" {
		return nil, fmt.Errorf("step id is required")
	}
	return &step, nil
}

func applyWorkflowStepPatch(existing *config.WorkflowStep, patchRaw interface{}) (*config.WorkflowStep, error) {
	if existing == nil {
		return nil, fmt.Errorf("existing step is nil")
	}
	patch, err := parseJSONObjectArg(patchRaw)
	if err != nil {
		return nil, err
	}
	if err := validateWorkflowStepJSONKeys(patch); err != nil {
		return nil, err
	}

	existingBytes, err := json.Marshal(existing)
	if err != nil {
		return nil, err
	}
	var merged map[string]interface{}
	if err := json.Unmarshal(existingBytes, &merged); err != nil {
		return nil, err
	}
	deepMergeObject(merged, patch)

	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	var updated config.WorkflowStep
	if err := json.Unmarshal(mergedBytes, &updated); err != nil {
		return nil, err
	}

	updated.ID = strings.TrimSpace(updated.ID)
	updated.Type = strings.TrimSpace(updated.Type)
	return &updated, nil
}

func findWorkflowStepIndex(steps []*config.WorkflowStep, stepID string) int {
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

func mergeDetails(primary, secondary map[string]interface{}) map[string]interface{} {
	if len(primary) == 0 && len(secondary) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for k, v := range secondary {
		out[k] = v
	}
	for k, v := range primary {
		out[k] = v
	}
	return out
}

func outcomeWorkflow(outcome *workflow.RunExecutionOutcome) *workflow.Workflow {
	if outcome == nil {
		return nil
	}
	return outcome.Workflow
}

func outcomeState(outcome *workflow.RunExecutionOutcome) *workflow.WorkflowRunState {
	if outcome == nil {
		return nil
	}
	return outcome.State
}

func runStateErrorDetails(wf *workflow.Workflow, state *workflow.WorkflowRunState, failedStepID string) map[string]interface{} {
	if state == nil {
		return nil
	}

	available := make([]string, 0, len(state.Steps))
	for id := range state.Steps {
		available = append(available, id)
	}
	sort.Strings(available)

	details := map[string]interface{}{
		"run_id":          state.RunID,
		"workflow_name":   state.WorkflowName,
		"status":          state.Status,
		"revision":        state.Revision,
		"cursor":          state.Cursor,
		"available_steps": available,
		"updated_at":      state.UpdatedAt.Format(time.RFC3339),
	}
	if wf != nil {
		details["step_summaries"] = workflow.BuildStepSummaries(wf, state)
	}
	if failedStepID != "" {
		details["failed_step_id"] = failedStepID
	}
	if state.Failure != nil {
		details["failure"] = state.Failure
	}
	return details
}

func (s *Server) makeDirectWorkflowToolFunc() func(tool string, args map[string]interface{}) (interface{}, error) {
	return func(tool string, args map[string]interface{}) (interface{}, error) {
		envelope, err := ExecuteToolDirect(s.vaultPath, tool, args)
		if err != nil {
			return nil, err
		}
		return envelope, nil
	}
}

func (s *Server) callDirectWorkflowList(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, _, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	items, err := workflow.List(vaultPath, vaultCfg)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	workflows := make([]map[string]interface{}, len(items))
	for i, item := range items {
		entry := map[string]interface{}{
			"name":        item.Name,
			"description": item.Description,
		}
		if len(item.Inputs) > 0 {
			entry["inputs"] = item.Inputs
		}
		workflows[i] = entry
	}

	return successEnvelope(map[string]interface{}{"workflows": workflows}, nil), false
}

func (s *Server) callDirectWorkflowAdd(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name cannot be empty", "", nil), true
	}
	if err := validateWorkflowCreateName(name); err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "Choose a different workflow name", nil), true
	}

	workflowDir := vaultCfg.GetWorkflowDirectory()
	rawFileRef := strings.TrimSpace(toString(normalized["file"]))
	if rawFileRef == "" {
		return errorEnvelope("MISSING_ARGUMENT", "--file is required", "Use --file <workflow YAML path>", nil), true
	}

	fileRef, err := workflow.ResolveWorkflowFileRef(rawFileRef, workflowDir)
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), fmt.Sprintf("Use a file path like %s<name>.yaml", workflowDir), nil), true
	}

	ref := &config.WorkflowRef{File: fileRef}
	loaded, code, err := registerWorkflowInConfig(vaultPath, vaultCfg, name, ref)
	if err != nil {
		suggestion := ""
		if code == "DUPLICATE_NAME" {
			suggestion = fmt.Sprintf("Use 'rvn workflow remove %s' first to replace it", name)
		}
		return errorEnvelope(code, err.Error(), suggestion, nil), true
	}

	out := map[string]interface{}{
		"name":        loaded.Name,
		"description": loaded.Description,
		"source":      "file",
		"file":        ref.File,
	}
	if len(loaded.Inputs) > 0 {
		out["inputs"] = loaded.Inputs
	}
	if len(loaded.Steps) > 0 {
		out["steps"] = loaded.Steps
	}
	return successEnvelope(out, nil), false
}

func (s *Server) callDirectWorkflowScaffold(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name cannot be empty", "", nil), true
	}
	if err := validateWorkflowCreateName(name); err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "Choose a different workflow name", nil), true
	}

	workflowDir := vaultCfg.GetWorkflowDirectory()
	fileRef := strings.TrimSpace(toString(normalized["file"]))
	if fileRef == "" {
		fileRef = fmt.Sprintf("%s%s.yaml", workflowDir, name)
	}

	fileRef, err := workflow.ResolveWorkflowFileRef(fileRef, workflowDir)
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), fmt.Sprintf("Use a file path like %s<name>.yaml", workflowDir), nil), true
	}

	fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		return errorEnvelope("FILE_OUTSIDE_VAULT", err.Error(), "Workflow files must be within the vault", nil), true
	}

	if _, err := os.Stat(fullPath); err == nil && !boolValue(normalized["force"]) {
		return errorEnvelope(
			"FILE_EXISTS",
			fmt.Sprintf("workflow file already exists: %s", fileRef),
			"Use --force to overwrite, or choose a different --file path",
			nil,
		), true
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return errorEnvelope("FILE_WRITE_ERROR", err.Error(), "", nil), true
	}

	content := buildWorkflowScaffoldYAML(name, toString(normalized["description"]))
	if err := atomicfile.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return errorEnvelope("FILE_WRITE_ERROR", err.Error(), "", nil), true
	}

	ref := &config.WorkflowRef{File: fileRef}
	loaded, code, err := registerWorkflowInConfig(vaultPath, vaultCfg, name, ref)
	if err != nil {
		suggestion := ""
		if code == "DUPLICATE_NAME" {
			suggestion = fmt.Sprintf("A scaffold file was written to %s. Remove the existing workflow first or use a different name.", fileRef)
		}
		return errorEnvelope(code, err.Error(), suggestion, nil), true
	}

	return successEnvelope(map[string]interface{}{
		"name":        loaded.Name,
		"description": loaded.Description,
		"file":        fileRef,
		"source":      "file",
		"scaffolded":  true,
	}, nil), false
}

func (s *Server) callDirectWorkflowRemove(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name cannot be empty", "", nil), true
	}

	if vaultCfg.Workflows == nil {
		return errorEnvelope(
			"WORKFLOW_NOT_FOUND",
			fmt.Sprintf("workflow '%s' not found", name),
			"Run 'rvn workflow list' to see available workflows",
			nil,
		), true
	}
	if _, exists := vaultCfg.Workflows[name]; !exists {
		return errorEnvelope(
			"WORKFLOW_NOT_FOUND",
			fmt.Sprintf("workflow '%s' not found", name),
			"Run 'rvn workflow list' to see available workflows",
			nil,
		), true
	}

	delete(vaultCfg.Workflows, name)
	if len(vaultCfg.Workflows) == 0 {
		vaultCfg.Workflows = nil
	}
	if err := config.SaveVaultConfig(vaultPath, vaultCfg); err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	return successEnvelope(map[string]interface{}{
		"name":    name,
		"removed": true,
	}, nil), false
}

func (s *Server) callDirectWorkflowShow(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name is required", "Usage: rvn workflow show <name>", nil), true
	}

	wf, err := workflow.Get(vaultPath, name, vaultCfg)
	if err != nil {
		return errorEnvelope("QUERY_NOT_FOUND", err.Error(), "Use 'rvn workflow list' to see available workflows", nil), true
	}

	data := map[string]interface{}{
		"name":        wf.Name,
		"description": wf.Description,
	}
	if len(wf.Inputs) > 0 {
		data["inputs"] = wf.Inputs
	}
	if len(wf.Steps) > 0 {
		data["steps"] = wf.Steps
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectWorkflowValidate(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	if len(vaultCfg.Workflows) == 0 {
		return successEnvelope(map[string]interface{}{
			"valid":   true,
			"checked": 0,
			"results": []workflowValidationItem{},
		}, nil), false
	}

	var names []string
	name := strings.TrimSpace(toString(normalized["name"]))
	if name != "" {
		if _, ok := vaultCfg.Workflows[name]; !ok {
			return errorEnvelope("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow '%s' not found", name), "Run 'rvn workflow list' to see available workflows", nil), true
		}
		names = []string{name}
	} else {
		names = make([]string, 0, len(vaultCfg.Workflows))
		for workflowName := range vaultCfg.Workflows {
			names = append(names, workflowName)
		}
		sort.Strings(names)
	}

	results := make([]workflowValidationItem, 0, len(names))
	invalidCount := 0
	for _, workflowName := range names {
		_, loadErr := workflow.LoadWithConfig(vaultPath, workflowName, vaultCfg.Workflows[workflowName], vaultCfg)
		item := workflowValidationItem{
			Name:  workflowName,
			Valid: loadErr == nil,
		}
		if loadErr != nil {
			item.Error = loadErr.Error()
			invalidCount++
		}
		results = append(results, item)
	}

	payload := map[string]interface{}{
		"valid":   invalidCount == 0,
		"checked": len(results),
		"invalid": invalidCount,
		"results": results,
	}
	if invalidCount > 0 {
		return errorEnvelope(
			"WORKFLOW_INVALID",
			fmt.Sprintf("%d workflow(s) invalid", invalidCount),
			"Use 'rvn workflow show <name>' to inspect a workflow definition",
			payload,
		), true
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowStepAdd(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	workflowName := strings.TrimSpace(toString(normalized["workflow-name"]))
	if workflowName == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name cannot be empty", "", nil), true
	}
	stepRaw, ok := normalized["step-json"]
	if !ok {
		return errorEnvelope("MISSING_ARGUMENT", "step-json is required", "", nil), true
	}
	step, err := parseWorkflowStepObject(stepRaw, true)
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}

	before := strings.TrimSpace(toString(normalized["before"]))
	after := strings.TrimSpace(toString(normalized["after"]))
	if before != "" && after != "" {
		return errorEnvelope("INVALID_INPUT", "use either --before or --after, not both", "", nil), true
	}

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.MutateStep(workflow.StepMutationRequest{
		WorkflowName: workflowName,
		Action:       workflow.StepMutationAdd,
		Step:         step,
		Position: workflow.PositionHint{
			BeforeStepID: before,
			AfterStepID:  after,
		},
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
	}

	payload := map[string]interface{}{
		"workflow_name": workflowName,
		"file":          result.FileRef,
		"action":        "add",
		"step_id":       result.StepID,
		"step":          result.Step,
		"index":         result.Index,
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowStepUpdate(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	workflowName := strings.TrimSpace(toString(normalized["workflow-name"]))
	stepID := strings.TrimSpace(toString(normalized["step-id"]))
	if workflowName == "" || stepID == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name and step id are required", "", nil), true
	}

	stepPatchRaw, ok := normalized["step-json"]
	if !ok {
		return errorEnvelope("MISSING_ARGUMENT", "step-json is required", "", nil), true
	}

	wf, err := workflow.Get(vaultPath, workflowName, vaultCfg)
	if err != nil {
		return errorEnvelope("WORKFLOW_NOT_FOUND", err.Error(), "Run 'rvn workflow list' to see available workflows", nil), true
	}

	targetIdx := findWorkflowStepIndex(wf.Steps, stepID)
	if targetIdx < 0 {
		return errorEnvelope(
			"REF_NOT_FOUND",
			fmt.Sprintf("step '%s' not found", stepID),
			"Use 'rvn workflow show <name>' to inspect step ids",
			map[string]interface{}{"workflow_name": workflowName, "step_id": stepID},
		), true
	}

	updatedStep, err := applyWorkflowStepPatch(wf.Steps[targetIdx], stepPatchRaw)
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}
	if updatedStep.ID == "" {
		updatedStep.ID = stepID
	}

	if updatedStep.ID != stepID {
		if idx := findWorkflowStepIndex(wf.Steps, updatedStep.ID); idx >= 0 {
			return errorEnvelope(
				"DUPLICATE_NAME",
				fmt.Sprintf("step id '%s' already exists", updatedStep.ID),
				"Use a unique step id",
				nil,
			), true
		}
	}

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.MutateStep(workflow.StepMutationRequest{
		WorkflowName: workflowName,
		Action:       workflow.StepMutationUpdate,
		TargetStepID: stepID,
		Step:         updatedStep,
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
	}

	payload := map[string]interface{}{
		"workflow_name": workflowName,
		"file":          result.FileRef,
		"action":        "update",
		"step_id":       result.StepID,
		"previous_id":   stepID,
		"step":          result.Step,
		"index":         result.Index,
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowStepRemove(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	workflowName := strings.TrimSpace(toString(normalized["workflow-name"]))
	stepID := strings.TrimSpace(toString(normalized["step-id"]))
	if workflowName == "" || stepID == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name and step id are required", "", nil), true
	}

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.MutateStep(workflow.StepMutationRequest{
		WorkflowName: workflowName,
		Action:       workflow.StepMutationRemove,
		TargetStepID: stepID,
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
	}

	payload := map[string]interface{}{
		"workflow_name": workflowName,
		"file":          result.FileRef,
		"action":        "remove",
		"step_id":       result.StepID,
		"index":         result.Index,
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowRun(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	workflowName := strings.TrimSpace(toString(normalized["name"]))
	if workflowName == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name is required", "Usage: rvn workflow run <name>", nil), true
	}

	inputs, err := parseWorkflowInputs(toString(normalized["input-file"]), normalized["input-json"], normalized["input"])
	if err != nil {
		return errorEnvelope("WORKFLOW_INPUT_INVALID", err.Error(), "", nil), true
	}

	svc := workflow.NewRunService(vaultPath, vaultCfg, s.makeDirectWorkflowToolFunc())
	outcome, err := svc.Start(workflow.StartRunRequest{
		WorkflowName: workflowName,
		Inputs:       inputs,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			details := mergeDetails(de.Details, runStateErrorDetails(outcomeWorkflow(outcome), outcomeState(outcome), de.StepID))
			return errorEnvelope(mapWorkflowDomainCodeToCLI(de.Code), de.Error(), workflowHintForDomainCode(de.Code), details), true
		}
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	resultData, err := toJSONMap(outcome.Result)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	return successEnvelope(resultData, nil), false
}

func (s *Server) callDirectWorkflowContinue(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	runID := strings.TrimSpace(toString(normalized["run-id"]))
	if runID == "" {
		runID = strings.TrimSpace(toString(normalized["run_id"]))
	}
	if runID == "" {
		return errorEnvelope("MISSING_ARGUMENT", "run id is required", "Usage: rvn workflow continue <run-id>", nil), true
	}

	outputEnv, err := parseAgentOutputEnvelope(
		toString(normalized["agent-output-file"]),
		normalized["agent-output-json"],
		toString(normalized["agent-output"]),
	)
	if err != nil {
		return errorEnvelope("WORKFLOW_AGENT_OUTPUT_INVALID", err.Error(), "", nil), true
	}

	svc := workflow.NewRunService(vaultPath, vaultCfg, s.makeDirectWorkflowToolFunc())
	outcome, err := svc.Continue(workflow.ContinueRunRequest{
		RunID:            runID,
		ExpectedRevision: intValueDefault(normalized["expected-revision"], 0),
		AgentOutput:      outputEnv,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			details := mergeDetails(de.Details, runStateErrorDetails(outcomeWorkflow(outcome), outcomeState(outcome), de.StepID))
			return errorEnvelope(mapWorkflowDomainCodeToCLI(de.Code), de.Error(), workflowHintForDomainCode(de.Code), details), true
		}
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	resultData, err := toJSONMap(outcome.Result)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	return successEnvelope(resultData, nil), false
}

func (s *Server) callDirectWorkflowRunsList(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	statuses, err := parseWorkflowRunStatusFilter(toString(normalized["status"]))
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}

	svc := workflow.NewRunService(vaultPath, vaultCfg, nil)
	runs, runWarnings, err := svc.ListRuns(workflow.RunListFilter{
		Workflow: toString(normalized["workflow"]),
		Statuses: statuses,
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "")
	}

	workflowCache := map[string]*workflow.Workflow{}
	workflowMissing := map[string]bool{}
	outRuns := make([]map[string]interface{}, 0, len(runs))
	for _, run := range runs {
		item := map[string]interface{}{
			"version":       run.Version,
			"run_id":        run.RunID,
			"workflow_name": run.WorkflowName,
			"workflow_hash": run.WorkflowHash,
			"status":        run.Status,
			"cursor":        run.Cursor,
			"revision":      run.Revision,
			"created_at":    run.CreatedAt.Format(time.RFC3339),
			"updated_at":    run.UpdatedAt.Format(time.RFC3339),
			"history":       run.History,
			"failure":       run.Failure,
		}

		availableSteps := make([]string, 0, len(run.Steps))
		for stepID := range run.Steps {
			availableSteps = append(availableSteps, stepID)
		}
		sort.Strings(availableSteps)
		item["available_steps"] = availableSteps

		if run.AwaitingStep != "" {
			item["awaiting_step_id"] = run.AwaitingStep
		}
		if run.CompletedAt != nil {
			item["completed_at"] = run.CompletedAt.Format(time.RFC3339)
		}
		if run.ExpiresAt != nil {
			item["expires_at"] = run.ExpiresAt.Format(time.RFC3339)
		}

		if !workflowMissing[run.WorkflowName] {
			wf := workflowCache[run.WorkflowName]
			if wf == nil {
				loadedWorkflow, loadErr := workflow.Get(vaultPath, run.WorkflowName, vaultCfg)
				if loadErr != nil {
					workflowMissing[run.WorkflowName] = true
				} else {
					wf = loadedWorkflow
					workflowCache[run.WorkflowName] = wf
				}
			}
			if wf != nil {
				item["step_summaries"] = workflow.BuildStepSummaries(wf, run)
			}
		}

		outRuns = append(outRuns, item)
	}

	return successEnvelope(map[string]interface{}{"runs": outRuns}, workflowWarningsToDirect(runWarnings)), false
}

func (s *Server) callDirectWorkflowRunsStep(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	runID := strings.TrimSpace(toString(normalized["run-id"]))
	if runID == "" {
		runID = strings.TrimSpace(toString(normalized["run_id"]))
	}
	stepID := strings.TrimSpace(toString(normalized["step-id"]))
	if stepID == "" {
		stepID = strings.TrimSpace(toString(normalized["step_id"]))
	}
	if runID == "" || stepID == "" {
		return errorEnvelope("MISSING_ARGUMENT", "run_id and step_id are required", "Usage: rvn workflow runs step <run-id> <step-id>", nil), true
	}

	paginationRequested := hasAnyArg(args, "path", "offset", "limit")

	svc := workflow.NewRunService(vaultPath, vaultCfg, nil)
	stepResult, err := svc.StepOutput(workflow.StepOutputRequest{
		RunID:      runID,
		StepID:     stepID,
		Paginated:  paginationRequested,
		Path:       toString(normalized["path"]),
		Offset:     intValueDefault(normalized["offset"], 0),
		Limit:      intValueDefault(normalized["limit"], 100),
		IncludeSum: true,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			hint := workflowHintForDomainCode(de.Code)
			if de.Code == workflow.CodeInvalidInput {
				hint = "Use --path for nested fields and provide valid --offset/--limit values"
			}
			return errorEnvelope(mapWorkflowDomainCodeToCLI(de.Code), de.Error(), hint, de.Details), true
		}
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	state := stepResult.State
	payload := map[string]interface{}{
		"run_id":        state.RunID,
		"workflow_name": state.WorkflowName,
		"status":        state.Status,
		"revision":      state.Revision,
		"step_id":       stepID,
	}
	if paginationRequested {
		payload["step_output_page"] = stepResult.StepOutputPage
	} else {
		payload["step_output"] = stepResult.StepOutput
	}
	if len(stepResult.Summaries) > 0 {
		payload["step_summaries"] = stepResult.Summaries
	}

	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowRunsPrune(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	statuses, err := parseWorkflowRunStatusFilter(toString(normalized["status"]))
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}
	olderThan, err := parseWorkflowOlderThan(toString(normalized["older-than"]))
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}

	confirm := boolValue(normalized["confirm"])

	svc := workflow.NewRunService(vaultPath, vaultCfg, nil)
	result, err := svc.PruneRuns(workflow.RunPruneOptions{
		Statuses:  statuses,
		OlderThan: olderThan,
		Apply:     confirm,
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "")
	}

	data := map[string]interface{}{
		"dry_run": !confirm,
		"prune":   result,
	}
	return successEnvelope(data, workflowWarningsToDirect(result.Warnings)), false
}
