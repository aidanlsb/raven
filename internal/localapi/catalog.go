package localapi

import (
	"github.com/aidanlsb/raven/internal/commands"
)

// APIVersion is the local HTTP API contract version advertised to clients.
const APIVersion = "1"

type commandCatalog struct {
	APIVersion string        `json:"api_version"`
	Commands   []commandInfo `json:"commands"`
}

type commandInfo struct {
	ID             string                 `json:"id"`
	CLIName        string                 `json:"cli_name"`
	CLIUsage       string                 `json:"cli_usage"`
	Summary        string                 `json:"summary"`
	Description    string                 `json:"description,omitempty"`
	Category       string                 `json:"category"`
	ReadOnly       bool                   `json:"read_only"`
	Destructive    bool                   `json:"destructive"`
	PreviewMode    string                 `json:"preview_mode"`
	Parameters     map[string]paramInfo   `json:"parameters"`
	ParameterOrder []string               `json:"parameter_order"`
	Required       []string               `json:"required"`
	SchemaHash     string                 `json:"schema_hash"`
	ArgsSchema     map[string]interface{} `json:"args_schema"`
}

type paramInfo struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

func buildCommandCatalog() commandCatalog {
	ids := allowedCommandIDsSorted()
	out := make([]commandInfo, 0, len(ids))
	for _, commandID := range ids {
		contract, ok := commands.BuildCommandContract(commandID)
		if !ok {
			continue
		}
		params := make(map[string]paramInfo, len(contract.Parameters))
		for name, spec := range contract.Parameters {
			params[name] = paramInfo{
				Name:        spec.Name,
				Type:        string(spec.Type),
				Required:    spec.Required,
				Description: spec.Description,
				Examples:    append([]string{}, spec.Examples...),
			}
		}
		out = append(out, commandInfo{
			ID:             contract.CommandID,
			CLIName:        contract.CLIName,
			CLIUsage:       contract.CLIUsage,
			Summary:        contract.Summary,
			Description:    contract.Description,
			Category:       contract.Category,
			ReadOnly:       contract.ReadOnly,
			Destructive:    contract.Destructive,
			PreviewMode:    contract.PreviewMode,
			Parameters:     params,
			ParameterOrder: append([]string{}, contract.ParameterOrder...),
			Required:       append([]string{}, contract.Required...),
			SchemaHash:     contract.SchemaHash,
			ArgsSchema:     commands.CompactArgsSchema(contract),
		})
	}
	return commandCatalog{
		APIVersion: APIVersion,
		Commands:   out,
	}
}
