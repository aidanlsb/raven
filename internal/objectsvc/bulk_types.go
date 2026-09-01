package objectsvc

// BulkResult represents the outcome of a single object in a bulk operation.
// Used by AddBulk, SetBulk, DeleteBulk, and MoveBulk.
type BulkResult struct {
	ID      string
	Status  string
	Reason  string
	Details string
}

// BulkPreviewItem represents a preview of a single object change in a bulk operation.
// Used by AddBulk, SetBulk, DeleteBulk, and MoveBulk.
type BulkPreviewItem struct {
	ID      string
	Action  string
	Details string
	Changes map[string]string
}
