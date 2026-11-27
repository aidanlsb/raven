package commandimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

// TestJSONContractPinsEnvelopeData snapshots the JSON envelope for representative
// commands. UPDATE_GOLDEN=1 rewrites testdata/json_contract/*.json after an
// intentional wire-format change. Volatile fields (meta.query_time_ms, absolute
// vault paths, and FTS ranks) are normalized so the files stay portable.
func TestJSONContractPinsEnvelopeData(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		handler func(context.Context, commandexec.Request) commandexec.Result
		args    map[string]any
		confirm bool
		setup   func(*testing.T, *testutil.TestVault, *commandexec.Request)
	}{
		{
			name:    "new",
			handler: HandleNew,
			args:    map[string]any{"type": "project", "title": "Contract New"},
		},
		{
			name:    "write",
			handler: HandleWrite,
			args:    map[string]any{"type": "project", "title": "Contract Write"},
		},
		{
			name:    "query_type",
			handler: HandleQuery,
			args:    map[string]any{"query_string": "type:project"},
		},
		{
			name:    "query_trait",
			handler: HandleQuery,
			args:    map[string]any{"query_string": "trait:priority"},
		},
		{
			name:    "query_section",
			handler: HandleQuery,
			args:    map[string]any{"query_string": "section .title==Tasks"},
		},
		{
			name:    "query_link",
			handler: HandleQuery,
			args:    map[string]any{"query_string": "link"},
		},
		{
			name:    "query_ids",
			handler: HandleQuery,
			args:    map[string]any{"query_string": "type:project", "ids": true},
		},
		{
			name:    "query_count",
			handler: HandleQuery,
			args:    map[string]any{"query_string": "type:project", "count-only": true},
		},
		{
			name:    "objects",
			handler: HandleObjects,
			args:    map[string]any{},
		},
		{
			name:    "search",
			handler: HandleSearch,
			args:    map[string]any{"query": "roadmap"},
		},
		{
			name:    "read",
			handler: HandleRead,
			args:    map[string]any{"reference": "projects/raven"},
		},
		{
			name:    "read_sections",
			handler: HandleRead,
			args:    map[string]any{"reference": "projects/raven", "sections": true},
		},
		{
			name:    "schema_validate",
			handler: HandleSchemaValidate,
			args:    map[string]any{},
		},
		{
			name:    "schema_add_type",
			handler: HandleSchemaAddType,
			args:    map[string]any{"name": "note", "default-path": "notes/"},
		},
		{
			name:    "vault_config_show",
			handler: HandleVaultConfigShow,
			args:    map[string]any{},
		},
		{
			name: "vault_config_auto_reindex_set",
			handler: func(ctx context.Context, req commandexec.Request) commandexec.Result {
				return handleVaultConfigOperation(ctx, req, "vault_config_auto_reindex_set")
			},
			args: map[string]any{"value": false},
		},
		{
			name:    "set",
			handler: HandleSet,
			args:    map[string]any{"reference": "projects/raven", "field": map[string]any{"status": "done"}},
		},
		{
			name:    "check",
			handler: HandleCheck,
			args:    map[string]any{},
		},
		{
			name:    "new_missing_title",
			handler: HandleNew,
			args:    map[string]any{"type": "project"},
		},
		{
			name:    "docs",
			handler: HandleDocs,
			setup:   withJSONContractDocsCache,
		},
		{
			name:    "docs_topics",
			handler: HandleDocs,
			args:    map[string]any{"section": "getting-started"},
			setup:   withJSONContractDocsCache,
		},
		{
			name:    "docs_topic",
			handler: HandleDocs,
			args:    map[string]any{"section": "getting-started", "topic": "installation"},
			setup:   withJSONContractDocsCache,
		},
		{
			name:    "docs_search",
			handler: HandleDocsSearch,
			args:    map[string]any{"query": "Welcome"},
			setup:   withJSONContractDocsCache,
		},
		{
			name:    "skill_list",
			handler: HandleSkillList,
			setup:   withJSONContractSkillDest,
		},
		{
			name:    "skill_install",
			handler: HandleSkillInstall,
			args:    map[string]any{"names": []string{"raven-core"}},
			setup:   withJSONContractSkillDest,
		},
		{
			name:    "config_show",
			handler: HandleConfigShow,
			setup:   withJSONContractGlobalConfig,
		},
		{
			name:    "vault_list",
			handler: HandleVaultList,
			setup:   withJSONContractVaultRegistry,
		},
		{
			name:    "schema",
			handler: HandleSchema,
			args:    map[string]any{},
		},
		{
			name:    "schema_types",
			handler: HandleSchema,
			args:    map[string]any{"subcommand": "types"},
		},
		{
			name:    "schema_traits",
			handler: HandleSchema,
			args:    map[string]any{"subcommand": "traits"},
		},
		{
			name:    "schema_type",
			handler: HandleSchema,
			args:    map[string]any{"subcommand": "type", "name": "project"},
		},
		{
			name:    "schema_trait",
			handler: HandleSchema,
			args:    map[string]any{"subcommand": "trait", "name": "due"},
		},
		{
			name:    "schema_core",
			handler: HandleSchema,
			args:    map[string]any{"subcommand": "core"},
		},
		{
			name:    "schema_template_list",
			handler: HandleSchemaTemplateList,
			args:    map[string]any{},
		},
		{
			name:    "backlinks",
			handler: HandleBacklinks,
			args:    map[string]any{"reference": "projects/raven"},
		},
		{
			name:    "outlinks",
			handler: HandleOutlinks,
			args:    map[string]any{"reference": "projects/raven"},
		},
		{
			name:    "resolve",
			handler: HandleResolve,
			args:    map[string]any{"reference": "projects/raven"},
		},
		{
			name:    "open",
			handler: HandleOpen,
			args:    map[string]any{"reference": "projects/raven"},
			setup:   withJSONContractGlobalConfig,
		},
		{
			name:    "vault_stats",
			handler: HandleVaultStats,
			args:    map[string]any{},
		},
		{
			name:    "date",
			handler: HandleDate,
			args:    map[string]any{"date": "2026-01-15"},
		},
		{
			name:    "daily",
			handler: HandleDaily,
			args:    map[string]any{"date": "2026-01-15"},
		},
		{
			name:    "query_saved_list",
			handler: HandleQuerySavedList,
			args:    map[string]any{},
		},
		{
			name:    "query_apply_unknown",
			handler: HandleQuery,
			args:    map[string]any{"query_string": "type:project", "apply": []string{"bogus"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := newJSONContractVault(t)
			args := copyJSONContractArgs(tc.args)
			req := commandexec.Request{
				VaultPath: v.Path,
				Caller:    commandexec.CallerCLI,
				Confirm:   tc.confirm,
				Args:      args,
			}
			if tc.setup != nil {
				tc.setup(t, v, &req)
			}
			result := tc.handler(context.Background(), req)
			got := marshalNormalizedEnvelope(t, result, v.Path)
			path := filepath.Join("testdata", "json_contract", tc.name+".json")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run UPDATE_GOLDEN=1)", path, err)
			}
			want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
			if !bytes.Equal(got, want) {
				t.Fatalf("JSON contract mismatch for %s\ngot:\n%s\nwant:\n%s", tc.name, got, want)
			}
		})
	}
}

func newJSONContractVault(t *testing.T) *testutil.TestVault {
	t.Helper()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/raven.md", `---
type: project
title: Raven
status: active
owner: people/alice
---
# Raven

## Tasks

Ship the roadmap. See [[people/alice]].
`).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
# Alice

Leads [[projects/raven]].
`).
		Build()
	reindexForEditTest(t, v.Path)
	return v
}

func marshalNormalizedEnvelope(t *testing.T, result commandexec.Result, vaultPath string) []byte {
	t.Helper()
	if result.Meta != nil {
		result.Meta.QueryTimeMs = 0
	}
	// FTS rank bits differ across GOARCH; pin the field, not the float.
	if search, ok := result.Data.(readsvc.SearchResult); ok {
		for i := range search.Items {
			search.Items[i].Rank = 0
		}
		result.Data = search
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	// JSON encodes Windows `\` as `\\`. Flatten those to `/` so vault-relative
	// paths match the slash-style goldens without touching `\n` escapes.
	normalized := bytes.ReplaceAll(raw, []byte(`\\`), []byte(`/`))
	normalized = bytes.ReplaceAll(normalized, []byte(filepath.ToSlash(vaultPath)), []byte("VAULT"))
	normalized = bytes.ReplaceAll(normalized, []byte(vaultPath), []byte("VAULT"))
	normalized = jsonContractGeneration.ReplaceAll(normalized, []byte(`"generation":"GEN"`))
	var compact bytes.Buffer
	if err := json.Compact(&compact, normalized); err != nil {
		t.Fatalf("compact envelope: %v", err)
	}
	out := compact.Bytes()
	if !strings.HasSuffix(string(out), "\n") {
		out = append(out, '\n')
	}
	return out
}

var jsonContractGeneration = regexp.MustCompile(`"generation":"[0-9a-fA-F]{32}"`)

func copyJSONContractArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}

func withJSONContractGlobalConfig(t *testing.T, v *testutil.TestVault, req *commandexec.Request) {
	t.Helper()
	configPath := filepath.Join(v.Path, "global-config.toml")
	if err := os.WriteFile(configPath, []byte("# empty config\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	req.ConfigPath = configPath
}

func withJSONContractVaultRegistry(t *testing.T, v *testutil.TestVault, req *commandexec.Request) {
	t.Helper()
	withJSONContractGlobalConfig(t, v, req)
	config := fmt.Sprintf("default_vault = \"contract\"\n\n[vaults]\ncontract = %q\n", v.Path)
	if err := os.WriteFile(req.ConfigPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write vault registry: %v", err)
	}
}

func withJSONContractSkillDest(t *testing.T, v *testutil.TestVault, req *commandexec.Request) {
	t.Helper()
	if req.Args == nil {
		req.Args = map[string]any{}
	}
	req.Args["scope"] = "project"
	req.Args["dest"] = filepath.Join(v.Path, ".skills")
}

func withJSONContractDocsCache(t *testing.T, v *testutil.TestVault, req *commandexec.Request) {
	t.Helper()
	withJSONContractGlobalConfig(t, v, req)
	docsPath := filepath.Join(v.Path, "docs")
	writeJSONContractFile(t, filepath.Join(docsPath, "index.yaml"), `sections:
  getting-started:
    title: Getting Started
    topics:
      installation:
        path: installation.md
`)
	writeJSONContractFile(t, filepath.Join(docsPath, "getting-started", "installation.md"), "# Installation\n\nWelcome to Raven docs.\n")
	manifest, err := json.Marshal(map[string]any{
		"cli_version": versioninfo.Current().Version,
	})
	if err != nil {
		t.Fatalf("marshal docs manifest: %v", err)
	}
	writeJSONContractFile(t, filepath.Join(docsPath, "manifest.json"), string(manifest)+"\n")
}

func writeJSONContractFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
