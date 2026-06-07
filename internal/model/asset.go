package model

// Asset represents a vault-local non-Markdown file resource.
//
// Assets are not schema objects. They are path-backed graph resources that can
// be referenced from Markdown. All asset metadata is derived from the filesystem
// and index; user-authored metadata should live in Markdown objects that
// reference assets.
type Asset struct {
	ID        string `json:"id"`
	FilePath  string `json:"file_path"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	FileMtime int64  `json:"file_mtime"`
	Extension string `json:"extension,omitempty"`
	Filename  string `json:"filename"`
	IndexedAt int64  `json:"indexed_at,omitempty"`
}
