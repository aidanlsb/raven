package mcp

import (
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/templatesvc"
)

func (s *Server) resolveDirectTemplateArgs(args map[string]interface{}) (string, *config.VaultConfig, map[string]interface{}, string, bool) {
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

func (s *Server) callDirectTemplateList(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, _, errOut, isErr := s.resolveDirectTemplateArgs(args)
	if isErr {
		return errOut, true
	}

	result, err := templatesvc.List(templatesvc.ListRequest{
		VaultPath:   vaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
	})
	if err != nil {
		return mapDirectTemplateSvcError(err)
	}

	return successEnvelope(map[string]interface{}{
		"template_dir": result.TemplateDir,
		"templates":    result.Templates,
	}, nil), false
}

func (s *Server) callDirectTemplateWrite(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectTemplateArgs(args)
	if isErr {
		return errOut, true
	}

	path := strings.TrimSpace(toString(normalized["path"]))
	if path == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires path argument", "Usage: rvn template write <path> --content <text>", nil), true
	}
	if !hasAnyArg(args, "content") {
		return errorEnvelope("MISSING_ARGUMENT", "--content is required", "Provide template markdown with --content", nil), true
	}

	result, err := templatesvc.Write(templatesvc.WriteRequest{
		VaultPath:   vaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		Path:        path,
		Content:     toString(normalized["content"]),
	})
	if err != nil {
		return mapDirectTemplateSvcError(err)
	}

	if result.Changed && result.ChangedPath != "" {
		maybeDirectReindexFile(vaultPath, result.ChangedPath, vaultCfg)
	}

	return successEnvelope(map[string]interface{}{
		"path":         result.Path,
		"status":       result.Status,
		"template_dir": result.TemplateDir,
	}, nil), false
}

func (s *Server) callDirectTemplateDelete(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectTemplateArgs(args)
	if isErr {
		return errOut, true
	}

	path := strings.TrimSpace(toString(normalized["path"]))
	if path == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires path argument", "Usage: rvn template delete <path>", nil), true
	}

	result, err := templatesvc.Delete(templatesvc.DeleteRequest{
		VaultPath:   vaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		Path:        path,
		Force:       boolValue(normalized["force"]),
	})
	if err != nil {
		return mapDirectTemplateSvcError(err)
	}

	warnings := make([]directWarning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, directWarning{
			Code:    warning.Code,
			Message: warning.Message,
			Ref:     warning.Ref,
		})
	}

	return successEnvelope(map[string]interface{}{
		"deleted":      result.DeletedPath,
		"trash_path":   result.TrashPath,
		"forced":       result.Forced,
		"template_ids": result.TemplateIDs,
	}, warnings), false
}

func mapDirectTemplateSvcError(err error) (string, bool) {
	svcErr, ok := templatesvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}
	return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, nil), true
}
