package mcp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/datesvc"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/vault"
)

func mapDirectDateSvcError(err error) (string, bool) {
	svcErr, ok := datesvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}
	return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, nil), true
}

func mapDirectQuerySvcError(err error) (string, bool) {
	svcErr, ok := querysvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}
	return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, nil), true
}

func mapDirectOpenResolveError(err error, notFoundSuggestion string) (string, bool) {
	var ambiguous *readsvc.AmbiguousRefError
	if errors.As(err, &ambiguous) {
		return errorEnvelope("REF_AMBIGUOUS", ambiguous.Error(), "Use a full object ID/path to disambiguate", nil), true
	}
	var notFound *readsvc.RefNotFoundError
	if errors.As(err, &notFound) {
		return errorEnvelope("REF_NOT_FOUND", notFound.Error(), notFoundSuggestion, nil), true
	}
	return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
}

func (s *Server) directEditorConfig() (*config.Config, string) {
	ctx, err := configsvc.ShowContext(s.directConfigContextOptions())
	if err != nil || ctx == nil || ctx.Cfg == nil {
		return nil, ""
	}
	return ctx.Cfg, ctx.Cfg.GetEditor()
}

func (s *Server) callDirectDaily(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	result, svcErr := datesvc.EnsureDaily(datesvc.EnsureDailyRequest{
		VaultPath:  vaultPath,
		DateArg:    strings.TrimSpace(toString(normalized["date"])),
		TemplateID: strings.TrimSpace(toString(normalized["template"])),
	})
	if svcErr != nil {
		return mapDirectDateSvcError(svcErr)
	}

	cfg, editor := s.directEditorConfig()
	opened := false
	if boolValue(normalized["edit"]) {
		opened = vault.OpenInEditor(cfg, result.FilePath)
	}

	return successEnvelope(map[string]interface{}{
		"file":    result.RelativePath,
		"date":    result.Date,
		"created": result.Created,
		"opened":  opened,
		"editor":  editor,
	}, nil), false
}

func (s *Server) callDirectDate(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	result, svcErr := datesvc.DateHub(datesvc.DateHubRequest{
		VaultPath: vaultPath,
		DateArg:   strings.TrimSpace(toString(normalized["date"])),
	})
	if svcErr != nil {
		return mapDirectDateSvcError(svcErr)
	}

	data := map[string]interface{}{
		"date":          result.Date,
		"day_of_week":   result.DayOfWeek,
		"daily_note_id": result.DailyNoteID,
		"daily_path":    result.DailyPath,
		"daily_exists":  result.DailyExists,
		"items":         result.Items,
		"backlinks":     result.Backlinks,
	}
	if result.DailyNote != nil {
		data["daily_note"] = result.DailyNote
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectOpen(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: false})
	if err != nil {
		return errorEnvelope("CONFIG_INVALID", "failed to load vault config", "Fix raven.yaml and try again", nil), true
	}
	defer rt.Close()

	cfg, editor := s.directEditorConfig()

	if boolValue(normalized["stdin"]) {
		references := extractSetObjectIDs(normalized, true)
		if len(references) == 0 {
			return errorEnvelope("MISSING_ARGUMENT", "no object IDs provided via stdin", "Provide object IDs via object_ids (array/string) when using raven_open bulk mode", nil), true
		}

		targets, failures := readsvc.ResolveOpenTargets(rt, references)
		if len(targets) == 0 {
			if len(failures) > 0 {
				return errorEnvelope("REF_NOT_FOUND", fmt.Sprintf("no files to open: %s: %s", failures[0].Reference, failures[0].Message), "Check references and run 'rvn reindex' if needed", nil), true
			}
			return errorEnvelope("REF_NOT_FOUND", "no files to open", "Check references and run 'rvn reindex' if needed", nil), true
		}

		filePaths := make([]string, 0, len(targets))
		relPaths := make([]string, 0, len(targets))
		for _, target := range targets {
			filePaths = append(filePaths, target.FilePath)
			relPaths = append(relPaths, target.RelativePath)
		}
		errs := make([]string, 0, len(failures))
		for _, failure := range failures {
			errs = append(errs, fmt.Sprintf("%s: %s", failure.Reference, failure.Message))
		}

		opened := vault.OpenFilesInEditor(cfg, filePaths)
		return successEnvelope(map[string]interface{}{
			"files":  relPaths,
			"opened": opened,
			"editor": editor,
			"errors": errs,
		}, nil), false
	}

	reference := strings.TrimSpace(toString(normalized["reference"]))
	if reference == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires reference argument", "Usage: rvn open <reference>", nil), true
	}

	target, err := readsvc.ResolveOpenTarget(rt, reference)
	if err != nil {
		return mapDirectOpenResolveError(err, "Check the reference and try again")
	}

	opened := vault.OpenInEditor(cfg, target.FilePath)
	return successEnvelope(map[string]interface{}{
		"file":   target.RelativePath,
		"opened": opened,
		"editor": editor,
	}, nil), false
}

func (s *Server) callDirectQueryAdd(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	result, svcErr := querysvc.Add(querysvc.AddRequest{
		VaultPath:   vaultPath,
		Name:        strings.TrimSpace(toString(normalized["name"])),
		QueryString: strings.TrimSpace(toString(normalized["query_string"])),
		Args:        stringSliceValues(normalized["arg"]),
		Description: strings.TrimSpace(toString(normalized["description"])),
	})
	if svcErr != nil {
		return mapDirectQuerySvcError(svcErr)
	}

	return successEnvelope(map[string]interface{}{
		"name":        result.Name,
		"query":       result.Query,
		"args":        result.Args,
		"description": result.Description,
	}, nil), false
}

func (s *Server) callDirectQueryRemove(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	result, svcErr := querysvc.Remove(querysvc.RemoveRequest{
		VaultPath: vaultPath,
		Name:      strings.TrimSpace(toString(normalized["name"])),
	})
	if svcErr != nil {
		return mapDirectQuerySvcError(svcErr)
	}

	return successEnvelope(map[string]interface{}{
		"name":    result.Name,
		"removed": result.Removed,
	}, nil), false
}
