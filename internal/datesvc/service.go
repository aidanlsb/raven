package datesvc

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerror"
	"github.com/aidanlsb/raven/internal/vault"
)

const (
	CodeInvalidInput  = "INVALID_INPUT"
	CodeConfigInvalid = "CONFIG_INVALID"
	CodeSchemaInvalid = "SCHEMA_INVALID"
	CodeDatabaseError = "DATABASE_ERROR"
	CodeQueryFailed   = "QUERY_FAILED"
	CodeFileWriteErr  = "FILE_WRITE_ERROR"
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

func EnsureDaily(req EnsureDailyRequest) (*EnsureDailyResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, svcerror.New(CodeInvalidInput, "vault path is required", "", nil)
	}

	vaultCfg, err := config.LoadVaultConfig(req.VaultPath)
	if err != nil {
		return nil, svcerror.New(CodeConfigInvalid, "failed to load vault config", "Fix raven.yaml and try again", err)
	}

	targetDate, err := vault.ParseDateArg(strings.TrimSpace(req.DateArg))
	if err != nil {
		return nil, svcerror.New(CodeInvalidInput, err.Error(), "Use today/yesterday/tomorrow or YYYY-MM-DD", err)
	}

	dateStr := vault.FormatDateISO(targetDate)
	friendlyDate := vault.FormatDateFriendly(targetDate)
	targetObjectPath := path.Join(vaultCfg.GetDailyDirectory(), dateStr)
	filePath := vaultCfg.DailyNotePath(req.VaultPath, dateStr)
	relPath := filepath.ToSlash(path.Join(vaultCfg.GetDailyDirectory(), dateStr+".md"))

	if pages.Exists(req.VaultPath, targetObjectPath) {
		return &EnsureDailyResult{
			Date:         dateStr,
			FriendlyDate: friendlyDate,
			RelativePath: relPath,
			FilePath:     filePath,
			Created:      false,
		}, nil
	}

	sch, err := schema.Load(req.VaultPath)
	if err != nil {
		return nil, svcerror.New(CodeSchemaInvalid, "failed to load schema", "Fix schema.yaml and try again", err)
	}

	var created *pages.CreateResult
	templateID := strings.TrimSpace(req.TemplateID)
	if templateID != "" {
		templateFile, err := schema.ResolveTypeTemplateFile(sch, "date", templateID)
		if err != nil {
			return nil, svcerror.New(CodeInvalidInput, err.Error(), "Use `rvn schema template list --core date` to see available template IDs", err)
		}
		created, err = pages.CreateDailyNoteWithTemplate(
			req.VaultPath,
			vaultCfg.GetDailyDirectory(),
			dateStr,
			friendlyDate,
			templateFile,
			vaultCfg.GetTemplateDirectory(),
			vaultCfg.ProtectedPrefixes,
		)
		if err != nil {
			return nil, svcerror.New(CodeFileWriteErr, "failed to create daily note", "", err)
		}
	} else {
		created, err = pages.CreateDailyNoteWithSchema(
			req.VaultPath,
			vaultCfg.GetDailyDirectory(),
			dateStr,
			friendlyDate,
			sch,
			vaultCfg.GetTemplateDirectory(),
			vaultCfg.ProtectedPrefixes,
		)
		if err != nil {
			return nil, svcerror.New(CodeFileWriteErr, "failed to create daily note", "", err)
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

func DateHub(req DateHubRequest) (*DateHubResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, svcerror.New(CodeInvalidInput, "vault path is required", "", nil)
	}

	vaultCfg, err := config.LoadVaultConfig(req.VaultPath)
	if err != nil {
		return nil, svcerror.New(CodeConfigInvalid, "failed to load vault config", "Fix raven.yaml and try again", err)
	}

	targetDate, err := vault.ParseDateArg(strings.TrimSpace(req.DateArg))
	if err != nil {
		return nil, svcerror.New(CodeInvalidInput, err.Error(), "Use today/yesterday/tomorrow or YYYY-MM-DD", err)
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

	db, err := index.Open(req.VaultPath)
	if err != nil {
		return nil, svcerror.New(CodeDatabaseError, "failed to open database", "Run 'rvn reindex' to rebuild the database", err)
	}
	defer db.Close()

	dailyNote, err := db.GetObject(result.DailyNoteID)
	if err != nil {
		return nil, svcerror.New(CodeQueryFailed, "failed to query daily note", "", err)
	}
	result.DailyNote = dailyNote
	result.DailyExists = dailyNote != nil

	items, err := db.QueryDateIndex(dateStr)
	if err != nil {
		return nil, svcerror.New(CodeQueryFailed, "failed to query date index", "", err)
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
				return nil, svcerror.New(CodeQueryFailed, fmt.Sprintf("failed to query trait %s", item.SourceID), "", err)
			}
			assoc.Trait = trait
		case "object":
			obj, err := db.GetObject(item.SourceID)
			if err != nil {
				return nil, svcerror.New(CodeQueryFailed, fmt.Sprintf("failed to query object %s", item.SourceID), "", err)
			}
			assoc.Object = obj
		}
		associations = append(associations, assoc)
	}
	result.Items = associations

	backlinks, err := db.Backlinks(dateStr)
	if err != nil {
		return nil, svcerror.New(CodeQueryFailed, "failed to query backlinks", "", err)
	}
	result.Backlinks = backlinks
	return result, nil
}

func NormalizeDateArgForDisplay(arg string) string {
	if strings.TrimSpace(arg) == "" {
		return "today"
	}
	return strings.TrimSpace(arg)
}

func NowWeekday(now time.Time) string {
	return now.Format("Monday")
}
