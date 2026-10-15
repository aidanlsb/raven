package readsvc

import (
	"encoding/json"
	"errors"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// ObjectSummary is a picker-safe object row: identity and a display name, no body.
type ObjectSummary struct {
	ID       string
	Type     string
	Name     string
	FilePath string
}

// ListObjectsResult is one page of object summaries from the index.
type ListObjectsResult struct {
	Items  []ObjectSummary
	Total  int
	Offset int
	Limit  int
}

// ListObjects returns every indexed file-backed object as id/type/name/path.
// limit 0 means no limit. Name is the schema name_field, else title, else name.
func ListObjects(rt *vaultruntime.Runtime, limit, offset int) (*ListObjectsResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if limit < 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, "--limit must be >= 0").
			WithSuggestion("Use --limit 0 for no limit")
	}
	if offset < 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, "--offset must be >= 0").
			WithSuggestion("Use --offset 0 for no offset")
	}
	if err := rt.OpenDB(); err != nil {
		if errors.Is(err, index.ErrIndexLocked) {
			return nil, svcerr.Wrap(codes.ErrIndexLocked, "index is locked for rebuild", err).
				WithSuggestion("Wait for the other process to finish and retry")
		}
		if errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, svcerr.Wrap(codes.ErrDatabaseVersion, "index schema is stale or a rebuild was interrupted", err).
				WithSuggestion("Run 'rvn reindex --full' to rebuild the index")
		}
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to open database", err).
			WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}

	rows, total, err := rt.DB.ListObjectSummaries(limit, offset)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrQueryFailed, "failed to list objects", err)
	}

	nameFields := indexschema.BuildTypeNameFields(rt.Schema)
	items := make([]ObjectSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, ObjectSummary{
			ID:       row.ID,
			Type:     row.Type,
			Name:     objectDisplayName(row.Type, row.FieldsJSON, nameFields),
			FilePath: row.FilePath,
		})
	}
	return &ListObjectsResult{
		Items:  items,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}, nil
}

func objectDisplayName(objType, fieldsJSON string, nameFields map[string]string) string {
	if name, ok := indexschema.ExtractNameFieldValue(nameFields, objType, fieldsJSON); ok {
		return name
	}
	if fieldsJSON == "" {
		return ""
	}
	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return ""
	}
	for _, key := range []string{"title", "name"} {
		if s, ok := fields[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
