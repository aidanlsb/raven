package readsvc

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type DateAssociation struct {
	Date       string
	SourceType string
	SourceID   string
	FieldName  string
	FilePath   string
	Trait      *model.Trait
	Object     *model.Object
}

type DateResult struct {
	Date        string
	DayOfWeek   string
	DailyNoteID string
	DailyPath   string
	DailyNote   *model.Object
	DailyExists bool
	Items       []DateAssociation
	Backlinks   []model.Reference
}

func Date(rt *vaultruntime.Runtime, dateArg string) (*DateResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "vault config runtime is required").
			WithSuggestion("Fix raven.yaml and try again")
	}
	targetDate, err := vault.ParseDateArg(strings.TrimSpace(dateArg))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).
			WithSuggestion("Use today/yesterday/tomorrow or YYYY-MM-DD")
	}
	dateStr := vault.FormatDateISO(targetDate)
	result := &DateResult{
		Date: dateStr, DayOfWeek: targetDate.Format("Monday"),
		DailyNoteID: rt.VaultCfg.DailyNoteID(dateStr),
		DailyPath:   filepath.ToSlash(path.Join(rt.VaultCfg.GetDailyDirectory(), dateStr+".md")),
		Items:       []DateAssociation{}, Backlinks: []model.Reference{},
	}
	if err := rt.OpenDB(); err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to open database", err).
			WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}

	dailyNote, err := rt.DB.GetObject(result.DailyNoteID)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrQueryFailed, "failed to query daily note", err)
	}
	result.DailyNote = dailyNote
	result.DailyExists = dailyNote != nil
	items, err := rt.DB.QueryDateIndex(dateStr)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrQueryFailed, "failed to query date index", err)
	}
	for _, item := range items {
		association := DateAssociation{
			Date: item.Date, SourceType: item.SourceType, SourceID: item.SourceID,
			FieldName: item.FieldName, FilePath: item.FilePath,
		}
		switch item.SourceType {
		case "trait":
			trait, err := rt.DB.GetTrait(item.SourceID)
			if err != nil {
				return nil, svcerr.Wrap(codes.ErrQueryFailed, fmt.Sprintf("failed to query trait %s", item.SourceID), err)
			}
			association.Trait = trait
		case "object":
			object, err := rt.DB.GetObject(item.SourceID)
			if err != nil {
				return nil, svcerr.Wrap(codes.ErrQueryFailed, fmt.Sprintf("failed to query object %s", item.SourceID), err)
			}
			association.Object = object
		}
		result.Items = append(result.Items, association)
	}
	backlinks, err := rt.DB.Backlinks(result.DailyNoteID)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrQueryFailed, "failed to query backlinks", err)
	}
	result.Backlinks = backlinks
	return result, nil
}
