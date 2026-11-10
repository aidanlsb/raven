package vaultconfigsvc

// ProtectedPrefixesListResult is the protected-prefix list payload.
type ProtectedPrefixesListResult struct {
	ConfigPath        string   `json:"config_path"`
	Exists            bool     `json:"exists"`
	ProtectedPrefixes []string `json:"protected_prefixes"`
}

// ProtectedPrefixAddResult is the add-prefix mutation payload.
type ProtectedPrefixAddResult struct {
	ConfigPath        string   `json:"config_path"`
	Created           bool     `json:"created"`
	Changed           bool     `json:"changed"`
	Prefix            string   `json:"prefix"`
	ProtectedPrefixes []string `json:"protected_prefixes"`
}

// ProtectedPrefixRemoveResult is the remove-prefix mutation payload.
type ProtectedPrefixRemoveResult struct {
	ConfigPath        string   `json:"config_path"`
	Changed           bool     `json:"changed"`
	Removed           string   `json:"removed"`
	ProtectedPrefixes []string `json:"protected_prefixes"`
}

// ExcludeListResult is the exclude-pattern list payload.
type ExcludeListResult struct {
	ConfigPath string   `json:"config_path"`
	Exists     bool     `json:"exists"`
	Exclude    []string `json:"exclude"`
}

// ExcludeAddResult is the add-exclude mutation payload.
type ExcludeAddResult struct {
	ConfigPath string   `json:"config_path"`
	Created    bool     `json:"created"`
	Changed    bool     `json:"changed"`
	Pattern    string   `json:"pattern"`
	Exclude    []string `json:"exclude"`
}

// ExcludeRemoveResult is the remove-exclude mutation payload.
type ExcludeRemoveResult struct {
	ConfigPath string   `json:"config_path"`
	Changed    bool     `json:"changed"`
	Removed    string   `json:"removed"`
	Exclude    []string `json:"exclude"`
}
