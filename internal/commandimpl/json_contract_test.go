package commandimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/testutil"
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := newJSONContractVault(t)
			result := tc.handler(context.Background(), commandexec.Request{
				VaultPath: v.Path,
				Caller:    commandexec.CallerCLI,
				Confirm:   tc.confirm,
				Args:      tc.args,
			})
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
	normalized := bytes.ReplaceAll(raw, []byte(strings.ReplaceAll(vaultPath, `\`, `\\`)), []byte("VAULT"))
	normalized = bytes.ReplaceAll(normalized, []byte(filepath.ToSlash(vaultPath)), []byte("VAULT"))
	normalized = bytes.ReplaceAll(normalized, []byte(`VAULT\\`), []byte("VAULT/"))
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
