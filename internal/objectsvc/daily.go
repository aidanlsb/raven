package objectsvc

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
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

// DailyIdentity is the resolved daily-note location for a date argument. It
// never creates a file. Exists reports whether the note is already on disk
// (the same check `rvn daily` uses before creating).
type DailyIdentity struct {
	Directory    string
	Date         string
	ID           string
	RelativePath string
	FilePath     string
	Exists       bool
	FriendlyDate string
}

// ResolveDaily maps a date argument (today/yesterday/tomorrow/YYYY-MM-DD,
// empty = today) onto the vault's daily-note identity using the same timezone
// and directory rules as `rvn daily` and `rvn date`.
func ResolveDaily(vaultPath string, vaultCfg *config.VaultConfig, dateArg string) (*DailyIdentity, error) {
	if vaultCfg == nil {
		vaultCfg = config.DefaultVaultConfig()
	}
	targetDate, err := vault.ParseDateArg(strings.TrimSpace(dateArg))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).
			WithSuggestion("Use today/yesterday/tomorrow or YYYY-MM-DD")
	}
	dateStr := vault.FormatDateISO(targetDate)
	directory := vaultCfg.GetDailyDirectory()
	identity := &DailyIdentity{
		Directory:    directory,
		Date:         dateStr,
		ID:           vaultCfg.DailyNoteID(dateStr),
		RelativePath: vaultCfg.DailyNoteRelPath(dateStr),
		FilePath:     vaultCfg.DailyNotePath(vaultPath, dateStr),
		FriendlyDate: vault.FormatDateFriendly(targetDate),
	}
	if strings.TrimSpace(vaultPath) != "" {
		identity.Exists = pages.Exists(vaultPath, path.Join(directory, dateStr))
	}
	return identity, nil
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
	identity, err := ResolveDaily(vaultPath, vaultCfg, dateArg)
	if err != nil {
		return nil, err
	}
	if identity.Exists {
		return &EnsureDailyResult{
			Date: identity.Date, FriendlyDate: identity.FriendlyDate, RelativePath: identity.RelativePath,
			FilePath: identity.FilePath, Created: false,
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
			vaultPath, vaultCfg.GetDailyDirectory(), identity.Date, identity.FriendlyDate,
			templateFile, vaultCfg.GetTemplateDirectory(), vaultCfg.ProtectedPrefixes,
		)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create daily note", err)
		}
	} else {
		created, err = pages.CreateDailyNoteWithSchema(
			vaultPath, vaultCfg.GetDailyDirectory(), identity.Date, identity.FriendlyDate,
			rt.Schema, vaultCfg.GetTemplateDirectory(), vaultCfg.ProtectedPrefixes,
		)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create daily note", err)
		}
	}

	result := &EnsureDailyResult{
		Date: identity.Date, FriendlyDate: identity.FriendlyDate, RelativePath: identity.RelativePath,
		FilePath: identity.FilePath, Created: true,
	}
	if created != nil {
		result.RelativePath = filepath.ToSlash(created.RelativePath)
		result.FilePath = created.FilePath
	}
	return result, nil
}
