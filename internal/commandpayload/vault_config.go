package commandpayload

// VaultDirectories contains effective directory settings in `vault config show`.
type VaultDirectories struct {
	Configured bool   `json:"configured"`
	Daily      string `json:"daily"`
	Type       string `json:"type"`
	Page       string `json:"page"`
	Template   string `json:"template"`
}

// VaultCapture contains effective capture settings in `vault config show`.
type VaultCapture struct {
	Destination string `json:"destination"`
	Heading     string `json:"heading"`
}

// VaultDeletion contains effective deletion settings in `vault config show`.
type VaultDeletion struct {
	Behavior string `json:"behavior"`
	TrashDir string `json:"trash_dir"`
}

// VaultConfigShowResult is the success payload for `vault config show`.
type VaultConfigShowResult struct {
	ConfigPath             string           `json:"config_path"`
	Exists                 bool             `json:"exists"`
	AutoReindex            bool             `json:"auto_reindex"`
	AutoReindexExplicit    bool             `json:"auto_reindex_explicit"`
	DailyTemplate          string           `json:"daily_template"`
	Directories            VaultDirectories `json:"directories"`
	Capture                VaultCapture     `json:"capture"`
	Deletion               VaultDeletion    `json:"deletion"`
	QueriesCount           int              `json:"queries_count"`
	ProtectedPrefixes      []string         `json:"protected_prefixes"`
	ProtectedPrefixesCount int              `json:"protected_prefixes_count"`
	Exclude                []string         `json:"exclude"`
	ExcludeCount           int              `json:"exclude_count"`
}

// VaultConfigAutoReindexResult is shared by auto-reindex set/unset. Created is
// present only for set; a pointer preserves an explicit false value.
type VaultConfigAutoReindexResult struct {
	ConfigPath          string `json:"config_path"`
	Created             *bool  `json:"created,omitempty"`
	Changed             bool   `json:"changed"`
	AutoReindex         bool   `json:"auto_reindex"`
	AutoReindexExplicit bool   `json:"auto_reindex_explicit"`
}

// VaultConfigProtectedPrefixesListResult is the protected-prefix list payload.
type VaultConfigProtectedPrefixesListResult struct {
	ConfigPath        string   `json:"config_path"`
	Exists            bool     `json:"exists"`
	ProtectedPrefixes []string `json:"protected_prefixes"`
}

// VaultConfigProtectedPrefixAddResult is the add-prefix mutation payload.
type VaultConfigProtectedPrefixAddResult struct {
	ConfigPath        string   `json:"config_path"`
	Created           bool     `json:"created"`
	Changed           bool     `json:"changed"`
	Prefix            string   `json:"prefix"`
	ProtectedPrefixes []string `json:"protected_prefixes"`
}

// VaultConfigProtectedPrefixRemoveResult is the remove-prefix mutation payload.
type VaultConfigProtectedPrefixRemoveResult struct {
	ConfigPath        string   `json:"config_path"`
	Changed           bool     `json:"changed"`
	Removed           string   `json:"removed"`
	ProtectedPrefixes []string `json:"protected_prefixes"`
}

// VaultConfigExcludeListResult is the exclude-pattern list payload.
type VaultConfigExcludeListResult struct {
	ConfigPath string   `json:"config_path"`
	Exists     bool     `json:"exists"`
	Exclude    []string `json:"exclude"`
}

// VaultConfigExcludeAddResult is the add-exclude mutation payload.
type VaultConfigExcludeAddResult struct {
	ConfigPath string   `json:"config_path"`
	Created    bool     `json:"created"`
	Changed    bool     `json:"changed"`
	Pattern    string   `json:"pattern"`
	Exclude    []string `json:"exclude"`
}

// VaultConfigExcludeRemoveResult is the remove-exclude mutation payload.
type VaultConfigExcludeRemoveResult struct {
	ConfigPath string   `json:"config_path"`
	Changed    bool     `json:"changed"`
	Removed    string   `json:"removed"`
	Exclude    []string `json:"exclude"`
}

// VaultConfigDirectoriesResult is shared by directories get/set/unset.
// Mutation-only fields use pointers so get omits them while mutations retain
// explicit false values.
type VaultConfigDirectoriesResult struct {
	ConfigPath string `json:"config_path"`
	Exists     bool   `json:"exists"`
	Configured bool   `json:"configured"`
	Daily      string `json:"daily"`
	Type       string `json:"type"`
	Page       string `json:"page"`
	Template   string `json:"template"`
	Created    *bool  `json:"created,omitempty"`
	Changed    *bool  `json:"changed,omitempty"`
}

// VaultConfigCaptureResult is shared by capture get/set/unset.
type VaultConfigCaptureResult struct {
	ConfigPath  string `json:"config_path"`
	Exists      bool   `json:"exists"`
	Configured  bool   `json:"configured"`
	Destination string `json:"destination"`
	Heading     string `json:"heading"`
	Created     *bool  `json:"created,omitempty"`
	Changed     *bool  `json:"changed,omitempty"`
}

// VaultConfigDeletionResult is shared by deletion get/set/unset.
type VaultConfigDeletionResult struct {
	ConfigPath string `json:"config_path"`
	Exists     bool   `json:"exists"`
	Configured bool   `json:"configured"`
	Behavior   string `json:"behavior"`
	TrashDir   string `json:"trash_dir"`
	Created    *bool  `json:"created,omitempty"`
	Changed    *bool  `json:"changed,omitempty"`
}
