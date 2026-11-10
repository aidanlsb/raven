package objectsvc

// BulkResult represents the outcome of a single object in a bulk operation.
// Used by AddBulk, SetBulk, DeleteBulk, and MoveBulk.
type BulkResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Details string `json:"details,omitempty"`
}

// BulkPreviewItem represents a preview of a single object change in a bulk operation.
// Used by AddBulk, SetBulk, DeleteBulk, and MoveBulk.
type BulkPreviewItem struct {
	ID      string            `json:"id"`
	Changes map[string]string `json:"changes,omitempty"`
	Action  string            `json:"action"`
	Details string            `json:"details,omitempty"`
}
