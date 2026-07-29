package sectionsvc

// IndexWarning preserves a post-mutation projection failure for command-layer
// warning classification and dirty-journal recovery.
type IndexWarning struct {
	FilePath string
	Stage    string
	Err      error
}
