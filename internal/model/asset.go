package model

// Asset represents a vault-local non-Markdown file resource.
//
// Assets are not schema objects. They are path-backed graph resources that can
// be referenced from Markdown and organized by asset kind rules.
type Asset struct {
	ID           string `json:"id"`
	FilePath     string `json:"file_path"`
	Kind         string `json:"kind,omitempty"`
	MediaType    string `json:"media_type,omitempty"`
	SizeBytes    int64  `json:"size_bytes"`
	FileMtime    int64  `json:"file_mtime"`
	Extension    string `json:"extension,omitempty"`
	Filename     string `json:"filename"`
	IndexedAt    int64  `json:"indexed_at,omitempty"`
	DefaultPath  string `json:"default_path,omitempty"`
	NonCanonical bool   `json:"non_canonical,omitempty"`
}
