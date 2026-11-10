package querysvc

import "github.com/aidanlsb/raven/internal/fieldvalue"

// Pagination holds the shared paging affordances emitted alongside query
// result windows. HasMore is always present so agents can loop without
// guessing; NextOffset is a forward cursor emitted only when more results
// remain.
type Pagination struct {
	Total      int  `json:"total"`
	Returned   int  `json:"returned"`
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	HasMore    bool `json:"has_more"`
	NextOffset *int `json:"next_offset,omitempty"`
}

// NewPagination builds the shared paging envelope. next_offset is included
// only when more results remain after this window.
func NewPagination(total, returned, offset, limit int) Pagination {
	paging := Pagination{
		Total:    total,
		Returned: returned,
		Offset:   offset,
		Limit:    limit,
		HasMore:  offset+returned < total,
	}
	if paging.HasMore {
		next := offset + returned
		paging.NextOffset = &next
	}
	return paging
}

// ObjectItem is a single row in a `type:` query result.
type ObjectItem struct {
	Num      int                              `json:"num"`
	ID       string                           `json:"id"`
	Type     string                           `json:"type"`
	Fields   map[string]fieldvalue.FieldValue `json:"fields"`
	FilePath string                           `json:"file_path"`
	Line     int                              `json:"line"`
}

// TraitItem is a single row in a `trait:` query result.
type TraitItem struct {
	Num       int     `json:"num"`
	ID        string  `json:"id"`
	TraitType string  `json:"trait_type"`
	Value     *string `json:"value"`
	Content   string  `json:"content"`
	FilePath  string  `json:"file_path"`
	Line      int     `json:"line"`
	ScopeID   string  `json:"object_id"` // JSON name retained for compatibility.
}

// SectionItem is a single row in a `section` query result.
type SectionItem struct {
	Num             int     `json:"num"`
	ID              string  `json:"id"`
	FileObjectID    string  `json:"file_object_id"`
	FilePath        string  `json:"file_path"`
	Slug            string  `json:"slug"`
	Title           string  `json:"title"`
	Level           int     `json:"level"`
	LineStart       int     `json:"line_start"`
	LineEnd         *int    `json:"line_end"`
	DirectLineEnd   *int    `json:"direct_line_end"`
	SubtreeLineEnd  *int    `json:"subtree_line_end"`
	ParentSectionID *string `json:"parent_section_id"`
}

// LinkItem is one outgoing Markdown link edge returned by a `link` query.
type LinkItem struct {
	Num           int    `json:"num"`
	SourceID      string `json:"source_id"`
	SourceType    string `json:"source_type"`
	FilePath      string `json:"file_path"`
	Line          int    `json:"line"`
	PositionStart int    `json:"position_start"`
	PositionEnd   int    `json:"position_end"`
	RawTarget     string `json:"raw_target"`
	Display       string `json:"display"`
	IsImage       bool   `json:"is_image"`
	Scheme        string `json:"scheme"`
	Ext           string `json:"ext"`
	NormalizedKey string `json:"normalized_key"`
}

// QueryObjectResult is the success payload for a `type:` query.
type QueryObjectResult struct {
	QueryKind  string       `json:"query_kind"`
	Type       string       `json:"type,omitempty"`
	SavedQuery string       `json:"saved_query,omitempty"`
	Items      []ObjectItem `json:"items"`
	Pagination
}

// QueryTraitResult is the success payload for a `trait:` query.
type QueryTraitResult struct {
	QueryKind  string      `json:"query_kind"`
	Trait      string      `json:"trait,omitempty"`
	SavedQuery string      `json:"saved_query,omitempty"`
	Items      []TraitItem `json:"items"`
	Pagination
}

// QuerySectionResult is the success payload for a `section` query.
type QuerySectionResult struct {
	QueryKind  string        `json:"query_kind"`
	SavedQuery string        `json:"saved_query,omitempty"`
	Items      []SectionItem `json:"items"`
	Pagination
}

// QueryLinkResult is the success payload for a `link` query.
type QueryLinkResult struct {
	QueryKind  string     `json:"query_kind"`
	SavedQuery string     `json:"saved_query,omitempty"`
	Items      []LinkItem `json:"items"`
	Pagination
}

// QueryIDsResult is the success payload for any query run with --ids.
type QueryIDsResult struct {
	IDs []string `json:"ids"`
	Pagination
}

// QueryCountResult is the success payload for any query run with --count-only.
// Type and Trait are mutually exclusive discriminators; section and link
// count responses omit both.
type QueryCountResult struct {
	QueryKind string `json:"query_kind"`
	Type      string `json:"type,omitempty"`
	Trait     string `json:"trait,omitempty"`
	Total     int    `json:"total"`
}

// QueryApplyEmptyResult is the no-match payload for `query --apply`. It keeps
// the legacy minimal bulk shape because no nested mutation command runs.
type QueryApplyEmptyResult struct {
	Preview bool          `json:"preview"`
	Action  string        `json:"action"`
	Items   []interface{} `json:"items"`
	Total   int           `json:"total"`
}

// EnvelopeOptions contains transport-neutral response choices for shaping a
// query ExecuteResult into the JSON envelope data object.
type EnvelopeOptions struct {
	IDsOnly        bool
	CountOnly      bool
	IsSavedQuery   bool
	SavedQueryName string
}

// Envelope maps an ExecuteResult onto the stable query payload. Count is the
// envelope meta.count value (total for --count-only, returned otherwise).
func Envelope(result *ExecuteResult, options EnvelopeOptions) (data any, count int) {
	if options.CountOnly {
		payload := QueryCountResult{
			QueryKind: result.QueryKind,
			Total:     result.Total,
		}
		switch result.QueryKind {
		case "trait":
			payload.Trait = result.TypeName
		case "section", "link":
			// Bare-root counts carry no type/trait discriminator.
		default:
			payload.Type = result.TypeName
		}
		return payload, result.Total
	}

	if options.IDsOnly {
		return QueryIDsResult{
			IDs:        result.IDs,
			Pagination: queryPagination(result),
		}, result.Returned
	}

	switch result.QueryKind {
	case "type":
		payload := QueryObjectResult{
			QueryKind:  "type",
			Items:      objectQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
		} else {
			payload.Type = result.TypeName
		}
		return payload, result.Returned
	case "section":
		payload := QuerySectionResult{
			QueryKind:  "section",
			Items:      sectionQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
		}
		return payload, result.Returned
	case "link":
		payload := QueryLinkResult{
			QueryKind:  "link",
			Items:      linkQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
		}
		return payload, result.Returned
	default:
		payload := QueryTraitResult{
			QueryKind:  "trait",
			Items:      traitQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
		} else {
			payload.Trait = result.TypeName
		}
		return payload, result.Returned
	}
}

func queryPagination(result *ExecuteResult) Pagination {
	return NewPagination(result.Total, result.Returned, result.Offset, result.Limit)
}

func objectQueryItems(result *ExecuteResult) []ObjectItem {
	items := make([]ObjectItem, len(result.Objects))
	for i, row := range result.Objects {
		items[i] = ObjectItem{
			Num:      result.Offset + i + 1,
			ID:       row.ID,
			Type:     row.Type,
			Fields:   row.Fields,
			FilePath: row.FilePath,
			Line:     row.LineStart,
		}
	}
	return items
}

func traitQueryItems(result *ExecuteResult) []TraitItem {
	items := make([]TraitItem, len(result.Traits))
	for i, row := range result.Traits {
		items[i] = TraitItem{
			Num:       result.Offset + i + 1,
			ID:        row.ID,
			TraitType: row.TraitType,
			Value:     row.IndexValueString(),
			Content:   row.Content,
			FilePath:  row.FilePath,
			Line:      row.Line,
			ScopeID:   row.ParentScopeID,
		}
	}
	return items
}

func sectionQueryItems(result *ExecuteResult) []SectionItem {
	items := make([]SectionItem, len(result.Sections))
	for i, row := range result.Sections {
		items[i] = SectionItem{
			Num:             result.Offset + i + 1,
			ID:              row.ID,
			FileObjectID:    row.FileObjectID,
			FilePath:        row.FilePath,
			Slug:            row.Slug,
			Title:           row.Title,
			Level:           row.Level,
			LineStart:       row.LineStart,
			LineEnd:         row.LineEnd,
			DirectLineEnd:   row.LineEnd,
			SubtreeLineEnd:  row.SubtreeLineEnd,
			ParentSectionID: row.ParentSectionID,
		}
	}
	return items
}

func linkQueryItems(result *ExecuteResult) []LinkItem {
	items := make([]LinkItem, len(result.Links))
	for i, row := range result.Links {
		items[i] = LinkItem{
			Num:           result.Offset + i + 1,
			SourceID:      row.SourceID,
			SourceType:    row.SourceType,
			FilePath:      row.FilePath,
			Line:          row.Line,
			PositionStart: row.PositionStart,
			PositionEnd:   row.PositionEnd,
			RawTarget:     row.RawTarget,
			Display:       row.Display,
			IsImage:       row.IsImage,
			Scheme:        row.Scheme,
			Ext:           row.Ext,
			NormalizedKey: row.NormalizedKey,
		}
	}
	return items
}
