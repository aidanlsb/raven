package objectsvc

func bulkFailureStatus(preview bool) string {
	if preview {
		return "skipped"
	}
	return "error"
}

func countBulkFailure(preview bool, skipped, errors *int) {
	if preview {
		*skipped++
		return
	}
	*errors++
}
