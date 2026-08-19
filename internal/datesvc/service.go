package datesvc

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type EnsureDailyRequest struct {
	VaultPath  string
	DateArg    string
	TemplateID string
}

type EnsureDailyResult struct {
	Date         string
	FriendlyDate string
	RelativePath string
	FilePath     string
	Created      bool
}

func EnsureDaily(rt *vaultruntime.Runtime, req EnsureDailyRequest) (*EnsureDailyResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "vault config runtime is required").WithSuggestion("Fix raven.yaml and try again")
	}
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg

	targetDate, err := vault.ParseDateArg(strings.TrimSpace(req.DateArg))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion("Use today/yesterday/tomorrow or YYYY-MM-DD")
	}

	dateStr := vault.FormatDateISO(targetDate)
	friendlyDate := vault.FormatDateFriendly(targetDate)
	targetObjectPath := path.Join(vaultCfg.GetDailyDirectory(), dateStr)
	filePath := vaultCfg.DailyNotePath(vaultPath, dateStr)
	relPath := filepath.ToSlash(path.Join(vaultCfg.GetDailyDirectory(), dateStr+".md"))

	if pages.Exists(vaultPath, targetObjectPath) {
		return &EnsureDailyResult{
			Date:         dateStr,
			FriendlyDate: friendlyDate,
			RelativePath: relPath,
			FilePath:     filePath,
			Created:      false,
		}, nil
	}

	if rt.SchemaLoadErr != nil {
		return nil, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to load schema", rt.SchemaLoadErr).WithSuggestion("Fix schema.yaml and try again")
	}
	if rt.Schema == nil {
		return nil, svcerr.New(codes.ErrSchemaInvalid, "schema runtime is required").WithSuggestion("Fix schema.yaml and try again")
	}
	sch := rt.Schema

	var created *pages.CreateResult
	templateID := strings.TrimSpace(req.TemplateID)
	if templateID != "" {
		templateFile, err := schema.ResolveTypeTemplateFile(sch, "date", templateID)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion("Use `rvn schema template list --core date` to see available template IDs")
		}
		created, err = pages.CreateDailyNoteWithTemplate(
			vaultPath,
			vaultCfg.GetDailyDirectory(),
			dateStr,
			friendlyDate,
			templateFile,
			vaultCfg.GetTemplateDirectory(),
			vaultCfg.ProtectedPrefixes,
		)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create daily note", err)
		}
	} else {
		created, err = pages.CreateDailyNoteWithSchema(
			vaultPath,
			vaultCfg.GetDailyDirectory(),
			dateStr,
			friendlyDate,
			sch,
			vaultCfg.GetTemplateDirectory(),
			vaultCfg.ProtectedPrefixes,
		)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create daily note", err)
		}
	}

	result := &EnsureDailyResult{
		Date:         dateStr,
		FriendlyDate: friendlyDate,
		RelativePath: relPath,
		FilePath:     filePath,
		Created:      true,
	}
	if created != nil {
		result.RelativePath = filepath.ToSlash(created.RelativePath)
		result.FilePath = created.FilePath
	}
	return result, nil
}

type DateHubRequest struct {
	VaultPath string
	DateArg   string
}

type DateAssociation struct {
	Date       string        `json:"date"`
	SourceType string        `json:"source_type"`
	SourceID   string        `json:"source_id"`
	FieldName  string        `json:"field_name"`
	FilePath   string        `json:"file_path"`
	Trait      *model.Trait  `json:"trait,omitempty"`
	Object     *model.Object `json:"object,omitempty"`
}

type DateHubResult struct {
	Date        string            `json:"date"`
	DayOfWeek   string            `json:"day_of_week"`
	DailyNoteID string            `json:"daily_note_id"`
	DailyPath   string            `json:"daily_path"`
	DailyNote   *model.Object     `json:"daily_note,omitempty"`
	DailyExists bool              `json:"daily_exists"`
	Items       []DateAssociation `json:"items"`
	Backlinks   []model.Reference `json:"backlinks"`
}

func DateHub(rt *vaultruntime.Runtime, req DateHubRequest) (*DateHubResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "vault config runtime is required").WithSuggestion("Fix raven.yaml and try again")
	}
	vaultCfg := rt.VaultCfg

	targetDate, err := vault.ParseDateArg(strings.TrimSpace(req.DateArg))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion("Use today/yesterday/tomorrow or YYYY-MM-DD")
	}

	dateStr := vault.FormatDateISO(targetDate)
	result := &DateHubResult{
		Date:        dateStr,
		DayOfWeek:   targetDate.Format("Monday"),
		DailyNoteID: vaultCfg.DailyNoteID(dateStr),
		DailyPath:   filepath.ToSlash(path.Join(vaultCfg.GetDailyDirectory(), dateStr+".md")),
		Items:       []DateAssociation{},
		Backlinks:   []model.Reference{},
	}

	if err := rt.OpenDB(); err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to open database", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}
	db := rt.DB

	dailyNote, err := db.GetObject(result.DailyNoteID)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrQueryFailed, "failed to query daily note", err)
	}
	result.DailyNote = dailyNote
	result.DailyExists = dailyNote != nil

	items, err := db.QueryDateIndex(dateStr)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrQueryFailed, "failed to query date index", err)
	}

	associations := make([]DateAssociation, 0, len(items))
	for _, item := range items {
		assoc := DateAssociation{
			Date:       item.Date,
			SourceType: item.SourceType,
			SourceID:   item.SourceID,
			FieldName:  item.FieldName,
			FilePath:   item.FilePath,
		}
		switch item.SourceType {
		case "trait":
			trait, err := db.GetTrait(item.SourceID)
			if err != nil {
				return nil, svcerr.Wrap(codes.ErrQueryFailed, fmt.Sprintf("failed to query trait %s", item.SourceID), err)
			}
			assoc.Trait = trait
		case "object":
			obj, err := db.GetObject(item.SourceID)
			if err != nil {
				return nil, svcerr.Wrap(codes.ErrQueryFailed, fmt.Sprintf("failed to query object %s", item.SourceID), err)
			}
			assoc.Object = obj
		}
		associations = append(associations, assoc)
	}
	result.Items = associations

	backlinks, err := db.Backlinks(result.DailyNoteID)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrQueryFailed, "failed to query backlinks", err)
	}
	result.Backlinks = backlinks
	return result, nil
}
