package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

const CommandContractSchemaVersion = "2"

type ParameterType string

const (
	ParameterTypeString      ParameterType = "string"
	ParameterTypeBool        ParameterType = "boolean"
	ParameterTypeInteger     ParameterType = "integer"
	ParameterTypeObject      ParameterType = "object"
	ParameterTypeStringArray ParameterType = "string_array"
)

type ParameterSpec struct {
	Name        string
	Type        ParameterType
	Required    bool
	Description string
	Aliases     []string
	Examples    []string
}

type CommandContract struct {
	CommandID      string
	ToolName       string
	CLIName        string
	CLIUsage       string
	Summary        string
	Description    string
	Category       string
	ReadOnly       bool
	Destructive    bool
	PreviewMode    string
	Parameters     map[string]ParameterSpec
	ParameterOrder []string
	Required       []string
	Examples       []string
	UseCases       []string
	Policy         Policy
	SchemaVersion  string
	SchemaHash     string
}

type ValidationIssue struct {
	Field    string `json:"field"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

func DiscoverableContracts() []CommandContract {
	ids := make([]string, 0, len(Registry))
	for commandID := range Registry {
		policy := PolicyForCommandID(commandID)
		if !policy.Discoverable {
			continue
		}
		ids = append(ids, commandID)
	}
	sort.Strings(ids)

	out := make([]CommandContract, 0, len(ids))
	for _, commandID := range ids {
		contract, ok := BuildCommandContract(commandID)
		if ok {
			out = append(out, contract)
		}
	}
	return out
}

func BuildCommandContract(commandID string) (CommandContract, bool) {
	meta, ok := EffectiveMeta(commandID)
	if !ok {
		return CommandContract{}, false
	}

	parameters := make(map[string]ParameterSpec, len(meta.Args)+len(meta.Flags))
	paramOrder := make([]string, 0, len(meta.Args)+len(meta.Flags))
	required := make([]string, 0, len(meta.Args))

	for _, arg := range meta.Args {
		spec := ParameterSpec{
			Name:        arg.Name,
			Type:        ParameterTypeString,
			Required:    arg.Required,
			Description: arg.Description,
		}
		parameters[arg.Name] = spec
		paramOrder = append(paramOrder, arg.Name)
		if arg.Required {
			required = append(required, arg.Name)
		}
	}

	for _, flag := range meta.Flags {
		spec := ParameterSpec{
			Name:        flag.Name,
			Type:        flagTypeToParameterType(flag.Type),
			Required:    false,
			Description: flag.Description,
			Examples:    append([]string{}, flag.Examples...),
		}
		parameters[flag.Name] = spec
		paramOrder = append(paramOrder, flag.Name)
	}
	if hasStdinFlag(meta.Flags) {
		name, spec := stdinReplacementParameter(meta)
		parameters[name] = spec
		paramOrder = append(paramOrder, name)
	}

	policy := PolicyForCommandID(commandID)
	description := strings.TrimSpace(meta.LongDesc)
	if description == "" {
		description = strings.TrimSpace(meta.Description)
	} else {
		description = withExampleSection(description, meta.Examples)
	}

	contract := CommandContract{
		CommandID:      commandID,
		ToolName:       toolNameForCommand(commandID),
		CLIName:        meta.Name,
		CLIUsage:       FullCLIUsage(commandID),
		Summary:        strings.TrimSpace(meta.Description),
		Description:    description,
		Category:       string(meta.Category),
		ReadOnly:       meta.Access == AccessRead,
		Destructive:    meta.Risk == RiskDestructive,
		PreviewMode:    previewModeForCommand(meta),
		Parameters:     parameters,
		ParameterOrder: paramOrder,
		Required:       required,
		Examples:       append([]string{}, meta.Examples...),
		UseCases:       append([]string{}, meta.UseCases...),
		Policy:         policy,
		SchemaVersion:  CommandContractSchemaVersion,
	}
	contract.SchemaHash = commandSchemaHash(contract, meta)
	return contract, true
}

func ContractParameterSchema(contract CommandContract) map[string]interface{} {
	out := make(map[string]interface{}, len(contract.Parameters))
	for _, name := range contract.ParameterOrder {
		spec := contract.Parameters[name]
		property := map[string]interface{}{
			"description": spec.Description,
		}
		switch spec.Type {
		case ParameterTypeString:
			property["type"] = "string"
		case ParameterTypeBool:
			property["type"] = "boolean"
		case ParameterTypeInteger:
			property["type"] = "integer"
		case ParameterTypeObject:
			property["type"] = "object"
		case ParameterTypeStringArray:
			property["type"] = "array"
			property["items"] = map[string]interface{}{"type": "string"}
		default:
			property["type"] = "string"
		}
		if len(spec.Examples) > 0 {
			property["examples"] = append([]string{}, spec.Examples...)
		}
		out[name] = property
	}
	return out
}

func CompactArgsSchema(contract CommandContract) map[string]interface{} {
	return map[string]interface{}{
		"required":   append([]string{}, contract.Required...),
		"properties": ContractParameterSchema(contract),
	}
}

func CompactInvokeExample(contract CommandContract) map[string]interface{} {
	args := make(map[string]interface{})
	for _, name := range contract.ParameterOrder {
		spec := contract.Parameters[name]
		if !spec.Required {
			continue
		}
		args[name] = exampleValueForParam(spec)
	}
	return map[string]interface{}{
		"command":     contract.CommandID,
		"schema_hash": contract.SchemaHash,
		"args":        args,
	}
}

func BuildInvokeParamSpec(contract CommandContract) map[string]ParameterSpec {
	paramSpec := make(map[string]ParameterSpec, len(contract.Parameters))
	for name, p := range contract.Parameters {
		paramSpec[name] = p
	}
	return paramSpec
}

func ValidateArgumentsStrict(spec map[string]ParameterSpec, raw map[string]interface{}) (map[string]interface{}, []ValidationIssue) {
	if raw == nil {
		raw = map[string]interface{}{}
	}

	normalized := make(map[string]interface{}, len(raw))
	issues := make([]ValidationIssue, 0)
	seenKeys := make(map[string]string)

	for key, value := range raw {
		canonical, ok := canonicalSpecKey(spec, key)
		if !ok {
			issues = append(issues, ValidationIssue{
				Field:   key,
				Code:    "UNKNOWN_ARGUMENT",
				Message: "unknown argument",
			})
			continue
		}
		if first, exists := seenKeys[canonical]; exists {
			issues = append(issues, ValidationIssue{
				Field:   key,
				Code:    "DUPLICATE_ARGUMENT",
				Message: fmt.Sprintf("argument duplicates '%s'", first),
			})
			continue
		}
		seenKeys[canonical] = key
		normalized[canonical] = value
	}

	for name, p := range spec {
		if p.Required {
			if _, ok := normalized[name]; !ok {
				issues = append(issues, ValidationIssue{
					Field:   name,
					Code:    "MISSING_REQUIRED_ARGUMENT",
					Message: "required argument is missing",
				})
			}
		}
	}

	for name, value := range normalized {
		p := spec[name]
		if matchesExpectedType(value, p.Type) {
			continue
		}
		issues = append(issues, ValidationIssue{
			Field:    name,
			Code:     "INVALID_ARGUMENT_TYPE",
			Message:  fmt.Sprintf("expected %s", expectedTypeLabel(p.Type)),
			Expected: expectedTypeLabel(p.Type),
			Actual:   actualTypeLabel(value),
		})
	}

	return normalized, issues
}

func WithInvokeWrapperHints(issues []ValidationIssue, invokeArgsSpec map[string]ParameterSpec) []ValidationIssue {
	if len(issues) == 0 {
		return issues
	}

	out := make([]ValidationIssue, len(issues))
	copy(out, issues)
	for i := range out {
		if out[i].Code != "UNKNOWN_ARGUMENT" {
			continue
		}
		if canonical, ok := canonicalSpecKey(invokeArgsSpec, out[i].Field); ok {
			out[i].Message = "unknown top-level argument"
			out[i].Hint = fmt.Sprintf("Did you mean args.%s? Command-specific parameters must be nested under args.", canonical)
		}
	}
	return out
}

func toolNameForCommand(commandID string) string {
	return "raven_" + strings.ReplaceAll(commandID, " ", "_")
}

func withExampleSection(description string, examples []string) string {
	if len(examples) == 0 {
		return strings.TrimSpace(description)
	}

	const maxExamples = 3

	exampleCount := len(examples)
	if exampleCount > maxExamples {
		exampleCount = maxExamples
	}

	b := strings.Builder{}
	b.WriteString(strings.TrimSpace(description))
	b.WriteString("\n\nExamples:")
	for _, example := range examples[:exampleCount] {
		b.WriteString("\n- `")
		b.WriteString(example)
		b.WriteString("`")
	}
	if len(examples) > maxExamples {
		b.WriteString(fmt.Sprintf("\n- ... (%d more in CLI help)", len(examples)-maxExamples))
	}

	return b.String()
}

func commandSchemaHash(contract CommandContract, meta Meta) string {
	hashSource := struct {
		SchemaVersion  string                   `json:"schema_version"`
		CommandID      string                   `json:"command_id"`
		CLIName        string                   `json:"cli_name"`
		CLIUsage       string                   `json:"cli_usage"`
		Description    string                   `json:"description"`
		Parameters     map[string]ParameterSpec `json:"parameters"`
		ParameterOrder []string                 `json:"parameter_order"`
		Args           []ArgMeta                `json:"args"`
		Flags          []FlagMeta               `json:"flags"`
		Policy         Policy                   `json:"policy"`
		Category       string                   `json:"category"`
		ReadOnly       bool                     `json:"read_only"`
		Destructive    bool                     `json:"destructive"`
		PreviewMode    string                   `json:"preview_mode"`
	}{
		SchemaVersion:  contract.SchemaVersion,
		CommandID:      contract.CommandID,
		CLIName:        contract.CLIName,
		CLIUsage:       contract.CLIUsage,
		Description:    contract.Description,
		Parameters:     contract.Parameters,
		ParameterOrder: append([]string{}, contract.ParameterOrder...),
		Args:           append([]ArgMeta{}, meta.Args...),
		Flags:          append([]FlagMeta{}, meta.Flags...),
		Policy:         contract.Policy,
		Category:       contract.Category,
		ReadOnly:       contract.ReadOnly,
		Destructive:    contract.Destructive,
		PreviewMode:    contract.PreviewMode,
	}
	b, _ := json.Marshal(hashSource)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func exampleValueForParam(spec ParameterSpec) interface{} {
	if len(spec.Examples) > 0 {
		return spec.Examples[0]
	}
	switch spec.Type {
	case ParameterTypeBool:
		return true
	case ParameterTypeInteger:
		return 1
	case ParameterTypeObject:
		return map[string]interface{}{}
	case ParameterTypeStringArray:
		return []string{}
	default:
		return fmt.Sprintf("<%s>", spec.Name)
	}
}

func canonicalSpecKey(spec map[string]ParameterSpec, key string) (string, bool) {
	if _, ok := spec[key]; ok {
		return key, true
	}
	normalizedKey := normalizeArgumentName(key)
	for name := range spec {
		if normalizeArgumentName(name) == normalizedKey {
			return name, true
		}
	}
	for name, param := range spec {
		for _, alias := range param.Aliases {
			if normalizeArgumentName(alias) == normalizedKey {
				return name, true
			}
		}
	}
	return "", false
}

func matchesExpectedType(value interface{}, expected ParameterType) bool {
	switch expected {
	case ParameterTypeString:
		_, ok := value.(string)
		return ok
	case ParameterTypeBool:
		_, ok := value.(bool)
		return ok
	case ParameterTypeInteger:
		switch v := value.(type) {
		case int, int8, int16, int32, int64:
			return true
		case uint, uint8, uint16, uint32, uint64:
			return true
		case float32:
			return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && math.Trunc(float64(v)) == float64(v)
		case float64:
			return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Trunc(v) == v
		case json.Number:
			_, err := v.Int64()
			return err == nil
		default:
			return false
		}
	case ParameterTypeObject:
		if value == nil {
			return false
		}
		if _, ok := value.(map[string]interface{}); ok {
			return true
		}
		if _, ok := value.(map[string]string); ok {
			return true
		}
		return false
	case ParameterTypeStringArray:
		switch typed := value.(type) {
		case []string:
			return true
		case []interface{}:
			for _, item := range typed {
				if _, ok := item.(string); !ok {
					return false
				}
			}
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func expectedTypeLabel(t ParameterType) string {
	switch t {
	case ParameterTypeString:
		return "string"
	case ParameterTypeBool:
		return "boolean"
	case ParameterTypeInteger:
		return "integer"
	case ParameterTypeObject:
		return "object"
	case ParameterTypeStringArray:
		return "array[string]"
	default:
		return "unknown"
	}
}

func actualTypeLabel(v interface{}) string {
	switch typed := v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "number"
	case json.Number:
		return "number"
	case map[string]interface{}, map[string]string:
		return "object"
	case []interface{}, []string:
		return "array"
	default:
		_ = typed
		return fmt.Sprintf("%T", v)
	}
}

func flagTypeToParameterType(flagType FlagType) ParameterType {
	switch flagType {
	case FlagTypeBool:
		return ParameterTypeBool
	case FlagTypeInt:
		return ParameterTypeInteger
	case FlagTypeStringSlice:
		return ParameterTypeStringArray
	case FlagTypeJSON, FlagTypeKeyValue, FlagTypePosKeyValue:
		return ParameterTypeObject
	default:
		return ParameterTypeString
	}
}

func previewModeForCommand(meta Meta) string {
	if meta.Name == "delete" {
		return "bulk_preview_default"
	}
	for _, flag := range meta.Flags {
		if flag.Name == "confirm" && flag.Type == FlagTypeBool {
			return "preview_default"
		}
	}
	return "none"
}

func hasStdinFlag(flags []FlagMeta) bool {
	for _, flag := range flags {
		if flag.Name == "stdin" && flag.Type == FlagTypeBool {
			return true
		}
	}
	return false
}

func stdinReplacementParameter(meta Meta) (string, ParameterSpec) {
	name := strings.TrimSpace(meta.BulkStdinArgName)
	if name == "" {
		name = "object_ids"
	}
	return name, ParameterSpec{
		Name:        name,
		Type:        ParameterTypeStringArray,
		Required:    false,
		Description: stdinReplacementDescription(name),
		Aliases:     sanitizeParameterAliases(meta.BulkStdinArgAliases, name),
	}
}

func stdinReplacementDescription(name string) string {
	switch strings.TrimSpace(name) {
	case "trait_ids":
		return "Trait IDs used as MCP stdin replacement for bulk mode"
	default:
		return "Object IDs used as MCP stdin replacement for bulk mode"
	}
}

func sanitizeParameterAliases(aliases []string, canonical string) []string {
	seen := map[string]struct{}{
		normalizeArgumentName(canonical): {},
	}
	out := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		normalized := normalizeArgumentName(alias)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, alias)
	}
	return out
}

func normalizeArgumentName(name string) string {
	name = strings.TrimSpace(name)
	return strings.ReplaceAll(name, "-", "_")
}
