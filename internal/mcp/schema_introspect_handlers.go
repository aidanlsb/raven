package mcp

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/schemasvc"
)

func (s *Server) callDirectSchema(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	subcommand := strings.TrimSpace(toString(normalized["subcommand"]))
	name := strings.TrimSpace(toString(normalized["name"]))

	if subcommand == "" {
		result, err := schemasvc.FullSchema(vaultPath)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		data := map[string]interface{}{
			"version": result.Version,
			"types":   result.Types,
			"traits":  result.Traits,
		}
		if len(result.Core) > 0 {
			data["core"] = result.Core
		}
		if len(result.Templates) > 0 {
			data["templates"] = result.Templates
		}
		if len(result.Queries) > 0 {
			data["queries"] = result.Queries
		}
		return successEnvelope(data, nil), false
	}

	switch subcommand {
	case "types":
		result, err := schemasvc.Types(vaultPath)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		data := map[string]interface{}{"types": result.Types}
		if result.Hint != nil {
			data["hint"] = result.Hint
		}
		return successEnvelope(data, nil), false
	case "traits":
		result, err := schemasvc.Traits(vaultPath)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		return successEnvelope(map[string]interface{}{"traits": result.Traits}, nil), false
	case "core":
		if name == "" {
			result, err := schemasvc.CoreList(vaultPath)
			if err != nil {
				return mapDirectSchemaServiceError(err)
			}
			return successEnvelope(map[string]interface{}{"core": result.Core}, nil), false
		}
		result, err := schemasvc.CoreByName(vaultPath, name)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		return successEnvelope(map[string]interface{}{"core": result.Core}, nil), false
	case "type":
		if name == "" {
			return errorEnvelope("MISSING_ARGUMENT", "specify a type name", "Usage: rvn schema type <name>", nil), true
		}
		result, err := schemasvc.TypeByName(vaultPath, name)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		return successEnvelope(map[string]interface{}{"type": result.Type}, nil), false
	case "trait":
		if name == "" {
			return errorEnvelope("MISSING_ARGUMENT", "specify a trait name", "Usage: rvn schema trait <name>", nil), true
		}
		result, err := schemasvc.TraitByName(vaultPath, name)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		return successEnvelope(map[string]interface{}{"trait": result.Trait}, nil), false
	default:
		return errorEnvelope("INVALID_INPUT", fmt.Sprintf("unknown schema subcommand: %s", subcommand), "Use: types, traits, type <name>, trait <name>, core [name], or template ...", nil), true
	}
}
