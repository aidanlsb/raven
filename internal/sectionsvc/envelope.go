package sectionsvc

import "github.com/aidanlsb/raven/internal/model"

// LifecycleResult is the success payload for section create/move.
type LifecycleResult struct {
	Section   string `json:"section"`
	File      string `json:"file"`
	Placement string `json:"placement"`
	Anchor    string `json:"anchor,omitempty"`
	Preview   bool   `json:"preview,omitempty"`
	Status    string `json:"status,omitempty"`
	Level     int    `json:"level,omitempty"`
}

// DeletePayload is the success payload for section subtree deletion.
type DeletePayload struct {
	Section         string            `json:"section"`
	File            string            `json:"file"`
	LineStart       int               `json:"line_start"`
	LineEnd         int               `json:"line_end"`
	RemovedContent  string            `json:"removed_content"`
	DeletedSections []string          `json:"deleted_sections"`
	Backlinks       []model.Reference `json:"backlinks"`
	Preview         bool              `json:"preview,omitempty"`
	Status          string            `json:"status"`
}

// RenamePayload is the success payload for section rename.
type RenamePayload struct {
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Preview     bool     `json:"preview,omitempty"`
	Status      string   `json:"status,omitempty"`
	UpdatedRefs []string `json:"updated_refs,omitempty"`
}

func lifecyclePayload(sectionID, file, placement, anchor string, preview bool) LifecycleResult {
	data := LifecycleResult{Section: sectionID, File: file, Placement: placement}
	if anchor != "" {
		data.Anchor = anchor
	}
	if preview {
		data.Preview = true
		data.Status = "preview"
	}
	return data
}

func (r *CreateResult) Payload(preview bool) LifecycleResult {
	data := lifecyclePayload(r.SectionID, r.FileRelative, r.Placement, r.AnchorID, preview)
	data.Level = r.Level
	return data
}

func (r *MoveResult) Payload(preview bool) LifecycleResult {
	return lifecyclePayload(r.SectionID, r.FileRelative, r.Placement, r.AnchorID, preview)
}

func (r *DeleteResult) Payload(preview bool) DeletePayload {
	data := DeletePayload{
		Section: r.SectionID, File: r.FileRelative, LineStart: r.LineStart, LineEnd: r.LineEnd,
		RemovedContent: r.RemovedContent, DeletedSections: r.DeletedSections, Backlinks: r.Backlinks,
		Status: "deleted",
	}
	if preview {
		data.Preview = true
		data.Status = "preview"
	}
	return data
}

func (r *RenameResult) Payload(preview bool) RenamePayload {
	data := RenamePayload{Source: r.SourceID, Destination: r.DestinationID}
	if preview {
		data.Preview = true
		data.Status = "preview"
	}
	if len(r.UpdatedRefs) > 0 {
		data.UpdatedRefs = r.UpdatedRefs
	}
	return data
}
