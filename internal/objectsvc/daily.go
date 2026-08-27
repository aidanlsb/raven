package objectsvc

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type EnsureDailyResult struct {
	Date         string
	FriendlyDate string
	RelativePath string
	FilePath     string
	Created      bool
}

func EnsureDaily(rt *vaultruntime.Runtime, dateArg, templateID string) (*EnsureDailyResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "vault config runtime is required").
			WithSuggestion("Fix raven.yaml and try again")
	}
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg
	targetDate, err := vault.ParseDateArg(strings.TrimSpace(dateArg))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).
			WithSuggestion("Use today/yesterday/tomorrow or YYYY-MM-DD")
	}

	dateStr := vault.FormatDateISO(targetDate)
	friendlyDate := vault.FormatDateFriendly(targetDate)
	targetObjectPath := path.Join(vaultCfg.GetDailyDirectory(), dateStr)
	filePath := vaultCfg.DailyNotePath(vaultPath, dateStr)
	relPath := filepath.ToSlash(path.Join(vaultCfg.GetDailyDirectory(), dateStr+".md"))
	if pages.Exists(vaultPath, targetObjectPath) {
		return &EnsureDailyResult{
			Date: dateStr, FriendlyDate: friendlyDate, RelativePath: relPath,
			FilePath: filePath, Created: false,
		}, nil
	}
	if rt.SchemaLoadErr != nil {
		return nil, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to load schema", rt.SchemaLoadErr).
			WithSuggestion("Fix schema.yaml and try again")
	}
	if rt.Schema == nil {
		return nil, svcerr.New(codes.ErrSchemaInvalid, "schema runtime is required").
			WithSuggestion("Fix schema.yaml and try again")
	}

	var created *pages.CreateResult
	templateID = strings.TrimSpace(templateID)
	if templateID != "" {
		templateFile, err := schema.ResolveTypeTemplateFile(rt.Schema, "date", templateID)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).
				WithSuggestion("Use `rvn schema template list --core date` to see available template IDs")
		}
		created, err = pages.CreateDailyNoteWithTemplate(
			vaultPath, vaultCfg.GetDailyDirectory(), dateStr, friendlyDate,
			templateFile, vaultCfg.GetTemplateDirectory(), vaultCfg.ProtectedPrefixes,
		)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create daily note", err)
		}
	} else {
		created, err = pages.CreateDailyNoteWithSchema(
			vaultPath, vaultCfg.GetDailyDirectory(), dateStr, friendlyDate,
			rt.Schema, vaultCfg.GetTemplateDirectory(), vaultCfg.ProtectedPrefixes,
		)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create daily note", err)
		}
	}

	result := &EnsureDailyResult{
		Date: dateStr, FriendlyDate: friendlyDate, RelativePath: relPath,
		FilePath: filePath, Created: true,
	}
	if created != nil {
		result.RelativePath = filepath.ToSlash(created.RelativePath)
		result.FilePath = created.FilePath
	}
	return result, nil
}
