package schemasvc

// SchemaChange describes one logical schema or vault-data mutation in a
// preview. Convert, field-rename, and type-rename plans share this record so
// CLI/JSON rendering cannot drift across those commands.
type SchemaChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}
