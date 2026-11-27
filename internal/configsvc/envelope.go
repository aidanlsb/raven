package configsvc

import "strings"

// UISettings is the nested `ui` object in global config payloads.
type UISettings struct {
	MarkdownStyle string `json:"markdown_style"`
}

// ShowResult is the success payload for `config show`.
type ShowResult struct {
	ConfigPath   string            `json:"config_path"`
	DefaultVault string            `json:"default_vault"`
	Editor       string            `json:"editor"`
	EditorMode   string            `json:"editor_mode"`
	Exists       bool              `json:"exists"`
	StatePath    string            `json:"state_path"`
	UI           UISettings        `json:"ui"`
	Vaults       map[string]string `json:"vaults"`
}

// MutateResult is the success payload for `config set` / `config unset`.
type MutateResult struct {
	Changed      []string          `json:"changed"`
	ConfigPath   string            `json:"config_path"`
	DefaultVault string            `json:"default_vault"`
	Editor       string            `json:"editor"`
	EditorMode   string            `json:"editor_mode"`
	Exists       bool              `json:"exists"`
	StatePath    string            `json:"state_path"`
	UI           UISettings        `json:"ui"`
	Vaults       map[string]string `json:"vaults"`
}

func (ctx *GlobalConfigContext) Payload() ShowResult {
	if ctx == nil {
		return ShowResult{Vaults: map[string]string{}}
	}

	vaults := make(map[string]string, len(ctx.Cfg.Vaults))
	for name, p := range ctx.Cfg.Vaults {
		vaults[name] = p
	}

	return ShowResult{
		ConfigPath:   ctx.ConfigPath,
		DefaultVault: DefaultVaultName(ctx.Cfg),
		Editor:       strings.TrimSpace(ctx.Cfg.GetEditor()),
		EditorMode:   effectiveValue(ctx.Cfg.EditorMode, "auto"),
		Exists:       ctx.ConfigExists,
		StatePath:    ctx.StatePath,
		UI:           UISettings{MarkdownStyle: effectiveValue(ctx.Cfg.UI.MarkdownStyle, "auto")},
		Vaults:       vaults,
	}
}

func (r *SetResult) Payload() MutateResult {
	return mutatePayload(r.Context, r.Changed)
}

func (r *UnsetResult) Payload() MutateResult {
	return mutatePayload(r.Context, r.Changed)
}

func mutatePayload(ctx *GlobalConfigContext, changed []string) MutateResult {
	show := ctx.Payload()
	return MutateResult{
		Changed:      changed,
		ConfigPath:   show.ConfigPath,
		DefaultVault: show.DefaultVault,
		Editor:       show.Editor,
		EditorMode:   show.EditorMode,
		Exists:       show.Exists,
		StatePath:    show.StatePath,
		UI:           show.UI,
		Vaults:       show.Vaults,
	}
}

func (r *InitResult) Payload() InitResult { return *r }

func (r *VaultListResult) Payload() VaultListResult { return *r }

func (r *VaultUseResult) Payload() VaultUseResult { return *r }

func (r *VaultClearResult) Payload() VaultClearResult { return *r }

func (r *VaultPinResult) Payload() VaultPinResult { return *r }

func (r *VaultAddResult) Payload() VaultAddResult { return *r }

func (r *VaultRemoveResult) Payload() VaultRemoveResult { return *r }

// VaultPathResult is the success payload for `vault list --path-only`.
type VaultPathResult struct {
	Path string `json:"path"`
}

// VaultFocusPayload is the success payload for `vault focus`.
type VaultFocusPayload struct {
	Applied     bool   `json:"applied"`
	Cleared     bool   `json:"cleared"`
	Hint        string `json:"hint,omitempty"`
	Name        string `json:"name,omitempty"`
	Path        string `json:"path,omitempty"`
	Scope       string `json:"scope"`
	SessionOnly bool   `json:"session_only"`
}

func (r *VaultFocusResult) Payload(applied bool, hint string) VaultFocusPayload {
	return VaultFocusPayload{
		Applied:     applied,
		Cleared:     r.Cleared,
		Hint:        hint,
		Name:        r.Name,
		Path:        r.Path,
		Scope:       "mcp_session",
		SessionOnly: true,
	}
}
