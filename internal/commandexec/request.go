package commandexec

// Caller identifies which surface initiated a command execution.
type Caller string

const (
	CallerCLI Caller = "cli"
	CallerMCP Caller = "mcp"
	CallerAPI Caller = "api"
)

// IsMachine reports whether the caller is a structured machine transport
// (MCP or the local HTTP API) rather than the interactive CLI.
func (c Caller) IsMachine() bool {
	return c == CallerMCP || c == CallerAPI
}

// Request is the normalized execution request shared across adapters.
type Request struct {
	CommandID      string         `json:"command"`
	VaultPath      string         `json:"vault_path,omitempty"`
	ConfigPath     string         `json:"config_path,omitempty"`
	StatePath      string         `json:"state_path,omitempty"`
	ExecutablePath string         `json:"-"`
	Caller         Caller         `json:"caller,omitempty"`
	Args           map[string]any `json:"args,omitempty"`
	Preview        bool           `json:"preview,omitempty"`
	Confirm        bool           `json:"confirm,omitempty"`
	Stdin          []byte         `json:"-"`

	// IndexJournalOperation identifies the write-ahead journal guard created
	// by the shared invoker for commands that use post-mutation indexing.
	IndexJournalOperation string `json:"-"`
}
