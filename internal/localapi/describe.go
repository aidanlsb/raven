package localapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vault"
)

// Session is the GET /v1/session payload: the bound vault, the editor
// OpenInEditor would use for this process, and today's daily-note identity.
type Session struct {
	APIVersion string       `json:"api_version"`
	VaultPath  string       `json:"vault_path"`
	Editor     string       `json:"editor"`
	EditorMode string       `json:"editor_mode"`
	Daily      SessionDaily `json:"daily"`
}

// SessionDaily is today's daily-note identity for the bound vault. It uses the
// same date argument, timezone, and directories.daily rules as `rvn daily` /
// `rvn date`. It never creates a file.
type SessionDaily struct {
	Directory   string `json:"directory"`
	TodayID     string `json:"today_id"`
	TodayPath   string `json:"today_path"`
	TodayExists bool   `json:"today_exists"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeResult(w, http.StatusOK, commandexec.Success(map[string]interface{}{
		"status":      "ok",
		"api_version": Version,
	}, nil))
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeResult(w, http.StatusOK, commandexec.Success(s.session(), nil))
}

func (s *Server) session() Session {
	editor, mode := vault.ResolvedEditor(s.loadSessionConfig())
	return Session{
		APIVersion: Version,
		VaultPath:  s.vaultPath,
		Editor:     editor,
		EditorMode: mode,
		Daily:      s.sessionDaily(),
	}
}

func (s *Server) sessionDaily() SessionDaily {
	vaultCfg := config.DefaultVaultConfig()
	if strings.TrimSpace(s.vaultPath) != "" {
		loaded, err := config.LoadVaultConfig(s.vaultPath)
		if err == nil && loaded != nil {
			vaultCfg = loaded
		}
	}
	identity, err := objectsvc.ResolveDaily(s.vaultPath, vaultCfg, "today")
	if err != nil {
		return SessionDaily{Directory: vaultCfg.GetDailyDirectory()}
	}
	return SessionDaily{
		Directory:   identity.Directory,
		TodayID:     identity.ID,
		TodayPath:   identity.RelativePath,
		TodayExists: identity.Exists,
	}
}

func (s *Server) loadSessionConfig() *config.Config {
	ctx, err := configsvc.ShowContext(configsvc.ContextOptions{
		ConfigPathOverride: s.configPath,
	})
	if err != nil || ctx == nil || ctx.Cfg == nil {
		return &config.Config{}
	}
	return ctx.Cfg
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeResult(w, http.StatusOK, s.execute(r.Context(), "schema", nil))
}

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	ids := allowedCommandIDs()
	sort.Strings(ids)

	items := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		contract, ok := commands.BuildCommandContract(id)
		if !ok {
			continue
		}
		_, write := allowedWriteCommands[id]
		items = append(items, map[string]interface{}{
			"id":           contract.CommandID,
			"summary":      contract.Summary,
			"description":  contract.Description,
			"read_only":    !write,
			"destructive":  contract.Destructive,
			"preview_mode": contract.PreviewMode,
			"args_schema":  commands.CompactArgsSchema(contract),
			"schema_hash":  contract.SchemaHash,
		})
	}

	writeResult(w, http.StatusOK, commandexec.Success(map[string]interface{}{
		"api_version": Version,
		"commands":    items,
	}, &commandexec.Meta{Count: len(items)}))
}
