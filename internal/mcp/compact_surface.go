package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/commands"
)

const commandContractSchemaVersion = "2"

type parameterType string

const (
	paramTypeString      parameterType = "string"
	paramTypeBool        parameterType = "boolean"
	paramTypeInteger     parameterType = "integer"
	paramTypeObject      parameterType = "object"
	paramTypeStringArray parameterType = "string_array"
)

type parameterSpec struct {
	Name        string
	Type        parameterType
	Required    bool
	Description string
	Examples    []string
}

type commandContract struct {
	CommandID      string
	ToolName       string
	CLIName        string
	Summary        string
	Description    string
	Category       string
	ReadOnly       bool
	Destructive    bool
	PreviewMode    string
	Parameters     map[string]parameterSpec
	ParameterOrder []string
	Required       []string
	Examples       []string
	UseCases       []string
	Policy         commands.Policy
	SchemaVersion  string
	SchemaHash     string
}

type validationIssue struct {
	Field    string `json:"field"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

func (s *Server) callCompactTool(name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case compactToolDiscover:
		out, isErr := s.callCompactDiscover(args)
		return out, isErr, true
	case compactToolDescribe:
		out, isErr := s.callCompactDescribe(args)
		return out, isErr, true
	case compactToolInvoke:
		out, isErr := s.callCompactInvoke(args)
		return out, isErr, true
	default:
		return "", false, false
	}
}

func (s *Server) callCompactDiscover(args map[string]interface{}) (string, bool) {
	spec := map[string]parameterSpec{
		"query": {
			Name: "query",
			Type: paramTypeString,
		},
		"category": {
			Name: "category",
			Type: paramTypeString,
		},
		"mode": {
			Name: "mode",
			Type: paramTypeString,
		},
		"risk": {
			Name: "risk",
			Type: paramTypeString,
		},
		"limit": {
			Name: "limit",
			Type: paramTypeInteger,
		},
		"cursor": {
			Name: "cursor",
			Type: paramTypeString,
		},
	}

	validated, issues := validateArgumentsStrict(spec, args)
	if len(issues) > 0 {
		return validationErrorEnvelope("raven_discover", issues), true
	}

	query := strings.ToLower(strings.TrimSpace(toString(validated["query"])))
	category := strings.ToLower(strings.TrimSpace(toString(validated["category"])))
	mode := strings.ToLower(strings.TrimSpace(toString(validated["mode"])))
	risk := strings.ToLower(strings.TrimSpace(toString(validated["risk"])))

	if mode != "" && mode != "read" && mode != "write" {
		return validationErrorEnvelope("raven_discover", []validationIssue{
			{
				Field:    "mode",
				Code:     "INVALID_ENUM",
				Message:  "mode must be one of: read, write",
				Expected: "read|write",
				Actual:   mode,
			},
		}), true
	}
	if risk != "" && risk != "safe" && risk != "mutating" && risk != "destructive" {
		return validationErrorEnvelope("raven_discover", []validationIssue{
			{
				Field:    "risk",
				Code:     "INVALID_ENUM",
				Message:  "risk must be one of: safe, mutating, destructive",
				Expected: "safe|mutating|destructive",
				Actual:   risk,
			},
		}), true
	}

	limit := 25
	if raw, ok := validated["limit"]; ok {
		limit = intValueDefault(raw, 25)
	}
	if limit <= 0 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}

	offset := 0
	if rawCursor := strings.TrimSpace(toString(validated["cursor"])); rawCursor != "" {
		n, err := strconv.Atoi(rawCursor)
		if err != nil || n < 0 {
			return validationErrorEnvelope("raven_discover", []validationIssue{
				{
					Field:    "cursor",
					Code:     "INVALID_CURSOR",
					Message:  "cursor must be a non-negative integer string",
					Expected: "string(int>=0)",
					Actual:   rawCursor,
				},
			}), true
		}
		offset = n
	}

	contracts := discoverableContracts()
	matches := make([]commandContract, 0, len(contracts))
	for _, contract := range contracts {
		if category != "" && contract.Category != category {
			continue
		}
		if mode == "read" && !contract.ReadOnly {
			continue
		}
		if mode == "write" && contract.ReadOnly {
			continue
		}
		if risk == "safe" && (!contract.ReadOnly || contract.Destructive) {
			continue
		}
		if risk == "mutating" && contract.ReadOnly {
			continue
		}
		if risk == "destructive" && !contract.Destructive {
			continue
		}

		if query != "" {
			searchable := strings.ToLower(strings.Join([]string{
				contract.CommandID,
				contract.ToolName,
				contract.CLIName,
				contract.Summary,
				contract.Category,
			}, " "))
			if !strings.Contains(searchable, query) {
				continue
			}
		}

		matches = append(matches, contract)
	}

	total := len(matches)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}

	out := make([]map[string]interface{}, 0, end-offset)
	for _, c := range matches[offset:end] {
		out = append(out, map[string]interface{}{
			"command":     c.CommandID,
			"tool_name":   c.ToolName,
			"summary":     c.Summary,
			"required":    append([]string{}, c.Required...),
			"category":    c.Category,
			"read_only":   c.ReadOnly,
			"destructive": c.Destructive,
			"schema_hash": c.SchemaHash,
		})
	}

	nextCursor := ""
	if end < total {
		nextCursor = strconv.Itoa(end)
	}

	categories := make(map[string]struct{})
	for _, c := range contracts {
		categories[c.Category] = struct{}{}
	}
	sortedCategories := make([]string, 0, len(categories))
	for categoryName := range categories {
		sortedCategories = append(sortedCategories, categoryName)
	}
	sort.Strings(sortedCategories)

	return successEnvelope(map[string]interface{}{
		"matches":      out,
		"total":        total,
		"returned":     len(out),
		"next_cursor":  nextCursor,
		"categories":   sortedCategories,
		"schema_epoch": commandContractSchemaVersion,
	}, nil), false
}

func (s *Server) callCompactDescribe(args map[string]interface{}) (string, bool) {
	spec := map[string]parameterSpec{
		"command": {
			Name:     "command",
			Type:     paramTypeString,
			Required: true,
		},
	}
	validated, issues := validateArgumentsStrict(spec, args)
	if len(issues) > 0 {
		return validationErrorEnvelope("raven_describe", issues), true
	}

	commandRef := strings.TrimSpace(toString(validated["command"]))
	commandID, ok := commands.ResolveToolCommandID(commandRef)
	if !ok {
		return errorEnvelope(
			"COMMAND_NOT_FOUND",
			fmt.Sprintf("unknown command: %s", commandRef),
			"Call raven_discover to find available commands",
			map[string]interface{}{"command": commandRef},
		), true
	}

	contract, ok := buildCommandContract(commandID)
	if !ok {
		return errorEnvelope(
			"COMMAND_NOT_FOUND",
			fmt.Sprintf("command '%s' is not available for MCP", commandRef),
			"Call raven_discover to find available commands",
			map[string]interface{}{"command": commandRef},
		), true
	}

	return successEnvelope(map[string]interface{}{
		"command":      contract.CommandID,
		"summary":      contract.Summary,
		"args_schema":  compactArgsSchema(contract),
		"read_only":    contract.ReadOnly,
		"destructive":  contract.Destructive,
		"preview_mode": contract.PreviewMode,
		"invokable":    contract.Policy.Invokable,
		"schema_hash":  contract.SchemaHash,
		"invoke_shape": map[string]interface{}{
			"wrapper": "args",
			"note":    "Pass command-specific parameters under args when calling raven_invoke.",
		},
		"invoke_example": compactInvokeExample(contract),
	}, nil), false
}

func (s *Server) callCompactInvoke(args map[string]interface{}) (string, bool) {
	spec := map[string]parameterSpec{
		"command": {
			Name:     "command",
			Type:     paramTypeString,
			Required: true,
		},
		"args": {
			Name: "args",
			Type: paramTypeObject,
		},
		"schema_hash": {
			Name: "schema_hash",
			Type: paramTypeString,
		},
		"strict_schema": {
			Name: "strict_schema",
			Type: paramTypeBool,
		},
	}
	validated, issues := validateArgumentsStrict(spec, args)
	commandRef := strings.TrimSpace(toString(validated["command"]))
	if len(issues) > 0 && commandRef == "" {
		return validationErrorEnvelope("raven_invoke", issues), true
	}

	commandID, ok := commands.ResolveToolCommandID(commandRef)
	if !ok {
		if len(issues) > 0 {
			return validationErrorEnvelope("raven_invoke", issues), true
		}
		return errorEnvelope(
			"COMMAND_NOT_FOUND",
			fmt.Sprintf("unknown command: %s", commandRef),
			"Call raven_discover to find available commands",
			map[string]interface{}{"command": commandRef},
		), true
	}

	contract, ok := buildCommandContract(commandID)
	if !ok {
		return errorEnvelope(
			"COMMAND_NOT_FOUND",
			fmt.Sprintf("command '%s' is not available for MCP", commandRef),
			"Call raven_discover to find available commands",
			map[string]interface{}{"command": commandRef},
		), true
	}
	if len(issues) > 0 {
		return validationErrorEnvelope("raven_invoke", withInvokeWrapperHints(issues, buildInvokeParamSpec(contract))), true
	}

	if !contract.Policy.Invokable {
		return errorEnvelope(
			"COMMAND_NOT_INVOKABLE",
			fmt.Sprintf("command '%s' cannot be invoked via raven_invoke", commandID),
			"Choose an invokable command from raven_discover",
			map[string]interface{}{
				"command":   commandID,
				"policy":    "invokable=false",
				"tool_hint": compactToolDiscover,
			},
		), true
	}

	strictSchema := true
	if raw, present := validated["strict_schema"]; present {
		strictSchema = boolValue(raw)
	}
	schemaHash := strings.TrimSpace(toString(validated["schema_hash"]))
	if strictSchema && schemaHash != "" && schemaHash != contract.SchemaHash {
		return errorEnvelope(
			"SCHEMA_MISMATCH",
			"provided schema_hash does not match current command schema",
			fmt.Sprintf("Call %s with command '%s' and retry with the returned schema_hash", compactToolDescribe, commandID),
			map[string]interface{}{
				"command":              commandID,
				"provided_schema_hash": schemaHash,
				"current_schema_hash":  contract.SchemaHash,
			},
		), true
	}

	rawInvokeArgs := map[string]interface{}{}
	if raw, ok := validated["args"]; ok && raw != nil {
		if cast, ok := raw.(map[string]interface{}); ok {
			rawInvokeArgs = cast
		} else if cast, ok := raw.(map[string]string); ok {
			rawInvokeArgs = make(map[string]interface{}, len(cast))
			for k, v := range cast {
				rawInvokeArgs[k] = v
			}
		}
	}

	paramSpec := buildInvokeParamSpec(contract)
	invokeArgs, argIssues := validateArgumentsStrict(paramSpec, rawInvokeArgs)
	if len(argIssues) > 0 {
		return errorEnvelope(
			"INVALID_ARGS",
			"argument validation failed",
			fmt.Sprintf("Call %s with command '%s' for the strict contract", compactToolDescribe, commandID),
			map[string]interface{}{
				"command": commandID,
				"issues":  argIssues,
			},
		), true
	}

	toolName := mcpToolName(commandID)
	op, ok := compatibilityToolSemanticMap[toolName]
	if !ok {
		return errorEnvelope(
			"INTERNAL_ERROR",
			"semantic handler is not configured for command",
			"report this issue with the failing command id",
			map[string]interface{}{"command": commandID},
		), true
	}

	out, isErr, handled := s.callSemanticTool(op, invokeArgs)
	if !handled {
		return errorEnvelope(
			"INTERNAL_ERROR",
			"semantic handler is not configured",
			"report this issue with the failing command id",
			map[string]interface{}{"command": commandID, "semantic_op": op},
		), true
	}
	return out, isErr
}

func discoverableContracts() []commandContract {
	ids := make([]string, 0, len(commands.Registry))
	for commandID := range commands.Registry {
		policy := commands.PolicyForCommandID(commandID)
		if !policy.Discoverable {
			continue
		}
		ids = append(ids, commandID)
	}
	sort.Strings(ids)

	out := make([]commandContract, 0, len(ids))
	for _, commandID := range ids {
		contract, ok := buildCommandContract(commandID)
		if ok {
			out = append(out, contract)
		}
	}
	return out
}

func buildCommandContract(commandID string) (commandContract, bool) {
	meta, ok := commands.Registry[commandID]
	if !ok {
		return commandContract{}, false
	}

	parameters := make(map[string]parameterSpec, len(meta.Args)+len(meta.Flags))
	paramOrder := make([]string, 0, len(meta.Args)+len(meta.Flags))
	required := make([]string, 0, len(meta.Args))

	for _, arg := range meta.Args {
		spec := parameterSpec{
			Name:        arg.Name,
			Type:        paramTypeString,
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
		spec := parameterSpec{
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
		parameters["object_ids"] = parameterSpec{
			Name:        "object_ids",
			Type:        paramTypeStringArray,
			Required:    false,
			Description: "Object IDs used as MCP stdin replacement for bulk mode",
		}
		paramOrder = append(paramOrder, "object_ids")
	}

	policy := commands.PolicyForCommandID(commandID)
	description := strings.TrimSpace(meta.LongDesc)
	if description == "" {
		description = strings.TrimSpace(meta.Description)
	} else {
		description = withExampleSection(description, meta.Examples)
	}

	contract := commandContract{
		CommandID:      commandID,
		ToolName:       mcpToolName(commandID),
		CLIName:        meta.Name,
		Summary:        strings.TrimSpace(meta.Description),
		Description:    description,
		Category:       categoryForCommandID(commandID),
		ReadOnly:       isReadOnlyCommand(commandID),
		Destructive:    isDestructiveCommand(commandID),
		PreviewMode:    previewModeForCommand(meta),
		Parameters:     parameters,
		ParameterOrder: paramOrder,
		Required:       required,
		Examples:       append([]string{}, meta.Examples...),
		UseCases:       append([]string{}, meta.UseCases...),
		Policy:         policy,
		SchemaVersion:  commandContractSchemaVersion,
	}
	contract.SchemaHash = commandSchemaHash(contract, meta)
	return contract, true
}

func commandSchemaHash(contract commandContract, meta commands.Meta) string {
	hashSource := struct {
		SchemaVersion string              `json:"schema_version"`
		CommandID     string              `json:"command_id"`
		CLIName       string              `json:"cli_name"`
		Description   string              `json:"description"`
		Args          []commands.ArgMeta  `json:"args"`
		Flags         []commands.FlagMeta `json:"flags"`
		Policy        commands.Policy     `json:"policy"`
		Category      string              `json:"category"`
		ReadOnly      bool                `json:"read_only"`
		Destructive   bool                `json:"destructive"`
		PreviewMode   string              `json:"preview_mode"`
	}{
		SchemaVersion: contract.SchemaVersion,
		CommandID:     contract.CommandID,
		CLIName:       contract.CLIName,
		Description:   contract.Description,
		Args:          append([]commands.ArgMeta{}, meta.Args...),
		Flags:         append([]commands.FlagMeta{}, meta.Flags...),
		Policy:        contract.Policy,
		Category:      contract.Category,
		ReadOnly:      contract.ReadOnly,
		Destructive:   contract.Destructive,
		PreviewMode:   contract.PreviewMode,
	}
	b, _ := json.Marshal(hashSource)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func contractParameterSchema(contract commandContract) map[string]interface{} {
	out := make(map[string]interface{}, len(contract.Parameters))
	for _, name := range contract.ParameterOrder {
		spec := contract.Parameters[name]
		property := map[string]interface{}{
			"description": spec.Description,
		}
		switch spec.Type {
		case paramTypeString:
			property["type"] = "string"
		case paramTypeBool:
			property["type"] = "boolean"
		case paramTypeInteger:
			property["type"] = "integer"
		case paramTypeObject:
			property["type"] = "object"
		case paramTypeStringArray:
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

func compactArgsSchema(contract commandContract) map[string]interface{} {
	return map[string]interface{}{
		"required":   append([]string{}, contract.Required...),
		"properties": contractParameterSchema(contract),
	}
}

func compactInvokeExample(contract commandContract) map[string]interface{} {
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

func exampleValueForParam(spec parameterSpec) interface{} {
	if len(spec.Examples) > 0 {
		return spec.Examples[0]
	}
	switch spec.Type {
	case paramTypeBool:
		return true
	case paramTypeInteger:
		return 1
	case paramTypeObject:
		return map[string]interface{}{}
	case paramTypeStringArray:
		return []string{}
	default:
		return fmt.Sprintf("<%s>", spec.Name)
	}
}

func buildInvokeParamSpec(contract commandContract) map[string]parameterSpec {
	paramSpec := make(map[string]parameterSpec, len(contract.Parameters))
	for name, p := range contract.Parameters {
		paramSpec[name] = p
	}
	return paramSpec
}

func validateArgumentsStrict(spec map[string]parameterSpec, raw map[string]interface{}) (map[string]interface{}, []validationIssue) {
	if raw == nil {
		raw = map[string]interface{}{}
	}

	normalized := make(map[string]interface{}, len(raw))
	issues := make([]validationIssue, 0)
	seenKeys := make(map[string]string)

	for key, value := range raw {
		canonical, ok := canonicalSpecKey(spec, key)
		if !ok {
			issues = append(issues, validationIssue{
				Field:   key,
				Code:    "UNKNOWN_ARGUMENT",
				Message: "unknown argument",
			})
			continue
		}
		if first, exists := seenKeys[canonical]; exists {
			issues = append(issues, validationIssue{
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
				issues = append(issues, validationIssue{
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
		issues = append(issues, validationIssue{
			Field:    name,
			Code:     "INVALID_ARGUMENT_TYPE",
			Message:  fmt.Sprintf("expected %s", expectedTypeLabel(p.Type)),
			Expected: expectedTypeLabel(p.Type),
			Actual:   actualTypeLabel(value),
		})
	}

	return normalized, issues
}

func withInvokeWrapperHints(issues []validationIssue, invokeArgsSpec map[string]parameterSpec) []validationIssue {
	if len(issues) == 0 {
		return issues
	}

	out := make([]validationIssue, len(issues))
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

func canonicalSpecKey(spec map[string]parameterSpec, key string) (string, bool) {
	if _, ok := spec[key]; ok {
		return key, true
	}
	if strings.Contains(key, "_") {
		alt := strings.ReplaceAll(key, "_", "-")
		if _, ok := spec[alt]; ok {
			return alt, true
		}
	}
	if strings.Contains(key, "-") {
		alt := strings.ReplaceAll(key, "-", "_")
		if _, ok := spec[alt]; ok {
			return alt, true
		}
	}
	return "", false
}

func matchesExpectedType(value interface{}, expected parameterType) bool {
	switch expected {
	case paramTypeString:
		_, ok := value.(string)
		return ok
	case paramTypeBool:
		_, ok := value.(bool)
		return ok
	case paramTypeInteger:
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
	case paramTypeObject:
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
	case paramTypeStringArray:
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

func expectedTypeLabel(t parameterType) string {
	switch t {
	case paramTypeString:
		return "string"
	case paramTypeBool:
		return "boolean"
	case paramTypeInteger:
		return "integer"
	case paramTypeObject:
		return "object"
	case paramTypeStringArray:
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

func validationErrorEnvelope(command string, issues []validationIssue) string {
	return errorEnvelope(
		"INVALID_ARGS",
		"argument validation failed",
		fmt.Sprintf("Call %s to get the strict parameter schema", compactToolDescribe),
		map[string]interface{}{
			"command": command,
			"issues":  issues,
		},
	)
}

func flagTypeToParameterType(flagType commands.FlagType) parameterType {
	switch flagType {
	case commands.FlagTypeBool:
		return paramTypeBool
	case commands.FlagTypeInt:
		return paramTypeInteger
	case commands.FlagTypeStringSlice:
		return paramTypeStringArray
	case commands.FlagTypeJSON, commands.FlagTypeKeyValue, commands.FlagTypePosKeyValue:
		return paramTypeObject
	default:
		return paramTypeString
	}
}

func previewModeForCommand(meta commands.Meta) string {
	for _, flag := range meta.Flags {
		if flag.Name == "confirm" && flag.Type == commands.FlagTypeBool {
			return "preview_default"
		}
	}
	return "none"
}

func hasStdinFlag(flags []commands.FlagMeta) bool {
	for _, flag := range flags {
		if flag.Name == "stdin" && flag.Type == commands.FlagTypeBool {
			return true
		}
	}
	return false
}

func categoryForCommandID(commandID string) string {
	switch {
	case commandID == "query" || commandID == "query_add" || commandID == "query_remove" ||
		commandID == "search" || commandID == "backlinks" || commandID == "outlinks" || commandID == "resolve":
		return "query"
	case commandID == "new" || commandID == "add" || commandID == "upsert" || commandID == "set" ||
		commandID == "delete" || commandID == "move" || commandID == "reclassify" || commandID == "import" ||
		commandID == "edit" || commandID == "update":
		return "content"
	case commandID == "schema" || strings.HasPrefix(commandID, "schema_") || commandID == "template" || strings.HasPrefix(commandID, "template_"):
		return "schema"
	case commandID == "workflow" || strings.HasPrefix(commandID, "workflow_"):
		return "workflow"
	case commandID == "read" || commandID == "open" || commandID == "daily" || commandID == "date" || commandID == "last":
		return "navigation"
	case commandID == "check" || commandID == "reindex" || commandID == "stats" || commandID == "untyped" || commandID == "version":
		return "maintenance"
	default:
		return "vault"
	}
}

func isReadOnlyCommand(commandID string) bool {
	switch commandID {
	case "read", "search", "backlinks", "outlinks", "resolve", "query",
		"schema", "schema_validate", "schema_template_list", "schema_template_get",
		"schema_type_template_list", "schema_core_template_list",
		"docs", "docs_list", "docs_search",
		"stats", "untyped", "version",
		"vault", "vault_list", "vault_current", "vault_path",
		"workflow_list", "workflow_show", "workflow_validate", "workflow_runs_list", "workflow_runs_step",
		"config", "config_show":
		return true
	default:
		return false
	}
}

func isDestructiveCommand(commandID string) bool {
	if commandID == "delete" || commandID == "move" || commandID == "reclassify" {
		return true
	}
	if strings.Contains(commandID, "remove") || strings.Contains(commandID, "delete") {
		return true
	}
	if commandID == "schema_rename_field" || commandID == "schema_rename_type" {
		return true
	}
	return false
}
