package mcp

import (
	"encoding/json"
	"strings"

	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/initsvc"
	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/reindexsvc"
)

func mapDirectMaintSvcError(err error) (string, bool) {
	svcErr, ok := maintsvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}
	return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, nil), true
}

func mapDirectInitSvcError(err error) (string, bool) {
	svcErr, ok := initsvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}
	return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, nil), true
}

func (s *Server) callDirectStats(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	result, svcErr := maintsvc.Stats(vaultPath)
	if svcErr != nil {
		return mapDirectMaintSvcError(svcErr)
	}

	return successEnvelope(map[string]interface{}{
		"file_count":   result.FileCount,
		"object_count": result.ObjectCount,
		"trait_count":  result.TraitCount,
		"ref_count":    result.RefCount,
	}, nil), false
}

func (s *Server) callDirectUntyped(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	pages, svcErr := maintsvc.Untyped(vaultPath)
	if svcErr != nil {
		return mapDirectMaintSvcError(svcErr)
	}

	return successEnvelope(map[string]interface{}{
		"count": len(pages),
		"items": pages,
	}, nil), false
}

func (s *Server) callDirectVersion(args map[string]interface{}) (string, bool) {
	info := maintsvc.CurrentVersionInfoFromExecutable(s.executable)
	return successEnvelope(map[string]interface{}{
		"version":     info.Version,
		"module_path": info.ModulePath,
		"commit":      info.Commit,
		"commit_time": info.CommitTime,
		"modified":    info.Modified,
		"go_version":  info.GoVersion,
		"goos":        info.GOOS,
		"goarch":      info.GOARCH,
	}, nil), false
}

func (s *Server) callDirectInit(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	path := strings.TrimSpace(toString(normalized["path"]))
	result, svcErr := initsvc.Initialize(initsvc.InitializeRequest{
		Path:       path,
		CLIVersion: maintsvc.CurrentVersionInfo().Version,
	})
	if svcErr != nil {
		return mapDirectInitSvcError(svcErr)
	}

	data := map[string]interface{}{
		"path":            result.Path,
		"status":          result.Status,
		"created_config":  result.CreatedConfig,
		"created_schema":  result.CreatedSchema,
		"gitignore_state": result.GitignoreState,
		"docs":            result.Docs,
	}
	if len(result.Warnings) == 0 {
		return successEnvelope(data, nil), false
	}

	warnings := make([]directWarning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, directWarning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}
	return successEnvelope(data, warnings), false
}

func (s *Server) callDirectReindex(args map[string]interface{}) (string, bool) {
	vaultPath, _, _, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	result, err := reindexsvc.Run(reindexsvc.RunRequest{
		VaultPath: vaultPath,
		Full:      boolValue(normalized["full"]),
		DryRun:    boolValue(normalized["dry-run"]),
	})
	if err != nil {
		if svcErr, ok := reindexsvc.AsError(err); ok {
			return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, nil), true
		}
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	return successEnvelope(result.Data(), warningMessagesToDirectWarnings(result.WarningMessages, "INDEX_UPDATE_FAILED")), false
}

func (s *Server) callDirectCheck(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	result, err := checksvc.Run(vaultPath, vaultCfg, sch, checksvc.Options{
		PathArg:     strings.TrimSpace(toString(normalized["path"])),
		TypeFilter:  strings.TrimSpace(toString(normalized["type"])),
		TraitFilter: strings.TrimSpace(toString(normalized["trait"])),
		Issues:      strings.TrimSpace(toString(normalized["issues"])),
		Exclude:     strings.TrimSpace(toString(normalized["exclude"])),
		ErrorsOnly:  boolValue(normalized["errors-only"]),
	})
	if err != nil {
		return errorEnvelope("VALIDATION_FAILED", err.Error(), "", nil), true
	}

	if boolValue(normalized["create-missing"]) &&
		boolValue(normalized["confirm"]) &&
		result.Scope.Type == "full" {
		checksvc.CreateMissingRefsNonInteractive(
			vaultPath,
			sch,
			result.MissingRefs,
			vaultCfg.GetObjectsRoot(),
			vaultCfg.GetPagesRoot(),
			vaultCfg.GetTemplateDirectory(),
		)
	}

	jsonResult := checksvc.BuildJSON(vaultPath, result)
	encoded, err := json.Marshal(jsonResult)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", "failed to build check response", "", nil), true
	}
	var data map[string]interface{}
	if err := json.Unmarshal(encoded, &data); err != nil {
		return errorEnvelope("INTERNAL_ERROR", "failed to build check response", "", nil), true
	}
	return successEnvelope(data, nil), false
}
