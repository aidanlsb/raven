package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/commands"
)

const commandContractSchemaVersion = commands.CommandContractSchemaVersion

type parameterType = commands.ParameterType

const (
	paramTypeString      parameterType = commands.ParameterTypeString
	paramTypeBool        parameterType = commands.ParameterTypeBool
	paramTypeInteger     parameterType = commands.ParameterTypeInteger
	paramTypeObject      parameterType = commands.ParameterTypeObject
	paramTypeStringArray parameterType = commands.ParameterTypeStringArray
)

type parameterSpec = commands.ParameterSpec

type commandContract = commands.CommandContract

type validationIssue = commands.ValidationIssue

func (s *Server) callCompactToolWithContext(ctx context.Context, name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case compactToolDiscover:
		out, isErr := s.callCompactDiscover(args)
		return out, isErr, true
	case compactToolDescribe:
		out, isErr := s.callCompactDescribe(args)
		return out, isErr, true
	case compactToolInvoke:
		out, isErr := s.callCompactInvokeWithContext(ctx, args)
		return out, isErr, true
	default:
		return "", false, false
	}
}

func (s *Server) callCompactDiscover(args map[string]interface{}) (string, bool) {
	_, issues := validateArgumentsStrict(map[string]parameterSpec{}, args)
	if len(issues) > 0 {
		return validationErrorEnvelope("raven_discover", issues), true
	}

	contracts := discoverableContracts()

	out := make([]map[string]interface{}, 0, len(contracts))
	for _, c := range contracts {
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
		"total":        len(out),
		"returned":     len(out),
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
	commandID, ok := commands.ResolveCommandID(commandRef)
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
		"cli_usage":    contract.CLIUsage,
		"args_schema":  compactArgsSchema(contract),
		"read_only":    contract.ReadOnly,
		"destructive":  contract.Destructive,
		"preview_mode": contract.PreviewMode,
		"invokable":    contract.Policy.Invokable,
		"schema_hash":  contract.SchemaHash,
		"invoke_shape": map[string]interface{}{
			"wrapper": "args",
			"note":    "Pass command-specific parameters under args when calling raven_invoke. Optional top-level wrapper fields: vault, vault_path, schema_hash, strict_schema.",
		},
		"invoke_example": compactInvokeExample(contract),
	}, nil), false
}

func (s *Server) callCompactInvoke(args map[string]interface{}) (string, bool) {
	return s.callCompactInvokeWithContext(context.Background(), args)
}

func (s *Server) callCompactInvokeWithContext(ctx context.Context, args map[string]interface{}) (string, bool) {
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
		"vault": {
			Name: "vault",
			Type: paramTypeString,
		},
		"vault_path": {
			Name: "vault_path",
			Type: paramTypeString,
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
	vaultName := strings.TrimSpace(toString(validated["vault"]))
	vaultPath := strings.TrimSpace(toString(validated["vault_path"]))
	if vaultName != "" && vaultPath != "" {
		return validationErrorEnvelope("raven_invoke", []validationIssue{
			{
				Field:   "vault_path",
				Code:    "CONFLICT",
				Message: "vault and vault_path are mutually exclusive",
				Hint:    "Pass either vault or vault_path, not both.",
			},
		}), true
	}

	commandID, ok := commands.ResolveCommandID(commandRef)
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

	if out, isErr, handled := s.callCanonicalCommandWithContext(ctx, commandID, rawInvokeArgs, vaultName, vaultPath); handled {
		return out, isErr
	}

	return errorEnvelope(
		"INTERNAL_ERROR",
		"canonical handler is not configured for command",
		"report this issue with the failing command id",
		map[string]interface{}{"command": commandID},
	), true
}

func discoverableContracts() []commandContract {
	return commands.DiscoverableContracts()
}

func buildCommandContract(commandID string) (commandContract, bool) {
	return commands.BuildCommandContract(commandID)
}

func compactArgsSchema(contract commandContract) map[string]interface{} {
	return commands.CompactArgsSchema(contract)
}

func compactInvokeExample(contract commandContract) map[string]interface{} {
	return commands.CompactInvokeExample(contract)
}

func buildInvokeParamSpec(contract commandContract) map[string]parameterSpec {
	return commands.BuildInvokeParamSpec(contract)
}

func validateArgumentsStrict(spec map[string]parameterSpec, raw map[string]interface{}) (map[string]interface{}, []validationIssue) {
	return commands.ValidateArgumentsStrict(spec, raw)
}

func withInvokeWrapperHints(issues []validationIssue, invokeArgsSpec map[string]parameterSpec) []validationIssue {
	return commands.WithInvokeWrapperHints(issues, invokeArgsSpec)
}

func withCommandArgumentHints(commandID string, rawArgs map[string]interface{}, issues []validationIssue) []validationIssue {
	if len(issues) == 0 {
		return issues
	}

	out := make([]validationIssue, len(issues))
	copy(out, issues)

	switch commandID {
	case "query":
		_, hasSaved := rawArgs["saved"]
		if !hasSaved {
			return out
		}
		for i := range out {
			switch {
			case out[i].Field == "saved" && out[i].Code == "UNKNOWN_ARGUMENT":
				out[i].Hint = "Pass the saved query name as args.query_string and any saved-query parameters in args.inputs."
			case out[i].Field == "query_string" && out[i].Code == "MISSING_REQUIRED_ARGUMENT":
				out[i].Hint = "Use args.query_string for either raw RQL or a saved query name."
			}
		}
	}

	return out
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
