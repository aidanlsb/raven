package importsvc

import "github.com/aidanlsb/raven/internal/check"

// ImportResult is the success payload for `import` in preview or apply mode.
type ImportResult struct {
	Total   int          `json:"total"`
	Created int          `json:"created"`
	Updated int          `json:"updated"`
	Skipped int          `json:"skipped"`
	Errors  int          `json:"errors"`
	Items   []ResultItem `json:"items"`
	check.MissingReferences
}

func (r *RunResult) Payload(refs check.MissingReferences) ImportResult {
	var created, updated, skipped, errored int
	for _, item := range r.Results {
		switch item.Action {
		case "created", "create":
			created++
		case "updated", "update":
			updated++
		case "skipped":
			skipped++
		case "error":
			errored++
		}
	}
	return ImportResult{
		Total: len(r.Results), Created: created, Updated: updated, Skipped: skipped,
		Errors: errored, Items: r.Results, MissingReferences: refs,
	}
}
