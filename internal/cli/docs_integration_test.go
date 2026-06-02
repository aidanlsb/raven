//go:build integration

package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_DocsListOpenSearch(t *testing.T) {
	t.Parallel()
	configPath := seedGlobalDocsConfig(t, map[string]string{
		"index.yaml": `sections:
  getting-started:
    topics:
      installation:
        path: installation.md
  querying:
    topics:
      query-language:
        path: query-language.md
`,
		"getting-started/installation.md": "# Installation\n\nWelcome.\n",
		"querying/query-language.md":      "# Query Language\n\nquery predicate examples.\n",
	})

	list := runDocsCLI(t, configPath, "docs")
	list.MustSucceed(t)
	sections := list.DataList("sections")
	if len(sections) == 0 {
		t.Fatalf("expected docs sections, got none")
	}

	listAlias := runDocsCLI(t, configPath, "docs", "list")
	listAlias.MustSucceed(t)
	aliasSections := listAlias.DataList("sections")
	if len(aliasSections) != len(sections) {
		t.Fatalf("expected docs list alias to return %d sections, got %d", len(sections), len(aliasSections))
	}

	requireSection(t, sections, "getting-started")
	requireSection(t, sections, "querying")
	requireSection(t, aliasSections, "getting-started")
	requireSection(t, aliasSections, "querying")

	querying := runDocsCLI(t, configPath, "docs", "querying")
	querying.MustSucceed(t)
	topics := querying.DataList("topics")
	if len(topics) == 0 {
		t.Fatalf("expected querying topics, got none")
	}
	requireTopic(t, topics, "query-language")

	open := runDocsCLI(t, configPath, "docs", "querying", "query-language")
	open.MustSucceed(t)
	if title := open.DataString("title"); title == "" {
		t.Fatalf("expected non-empty title in docs open response")
	}
	content := open.DataString("content")
	if content == "" {
		t.Fatalf("expected non-empty content in docs open response")
	}

	search := runDocsCLI(t, configPath, "docs", "search", "query", "--section", "querying", "--limit", "5")
	search.MustSucceed(t)
	if count, ok := search.Data["count"].(float64); !ok || count < 1 {
		t.Fatalf("expected search count >= 1, got %v", search.Data["count"])
	}
}

func TestIntegration_DocsCommandRedirectToHelp(t *testing.T) {
	t.Parallel()
	configPath := seedGlobalDocsConfig(t, map[string]string{
		"index.yaml": `sections:
  getting-started:
    topics:
      installation:
        path: installation.md
`,
		"getting-started/installation.md": "# Installation\n",
	})

	res := runDocsCLI(t, configPath, "docs", "query")
	res.MustFail(t, "INVALID_INPUT")
	res.MustFailWithMessage(t, "rvn help query")
}

func seedGlobalDocsConfig(t *testing.T, files map[string]string) string {
	t.Helper()
	globalDir := t.TempDir()
	configPath := filepath.Join(globalDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("# test config\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	for relPath, content := range files {
		fullPath := filepath.Join(globalDir, "docs", filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir docs path: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write docs file: %v", err)
		}
	}
	return configPath
}

func runDocsCLI(t *testing.T, configPath string, args ...string) *testutil.CLIResult {
	t.Helper()
	binary := testutil.BuildCLI(t)
	statePath := filepath.Join(filepath.Dir(configPath), "state.toml")
	cmdArgs := []string{"--config", configPath, "--state", statePath, "--json"}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(binary, cmdArgs...)
	output, err := cmd.CombinedOutput()
	result := &testutil.CLIResult{RawJSON: string(output)}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	var resp struct {
		OK       bool                   `json:"ok"`
		Data     map[string]interface{} `json:"data,omitempty"`
		Error    *testutil.CLIError     `json:"error,omitempty"`
		Warnings []testutil.CLIWarning  `json:"warnings,omitempty"`
		Meta     *testutil.CLIMeta      `json:"meta,omitempty"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		result.OK = false
		result.Error = &testutil.CLIError{
			Code:    "PARSE_ERROR",
			Message: "Failed to parse JSON output: " + err.Error(),
			Details: map[string]interface{}{"raw": string(output)},
		}
		return result
	}
	result.OK = resp.OK
	result.Data = resp.Data
	result.Error = resp.Error
	result.Warnings = resp.Warnings
	result.Meta = resp.Meta
	return result
}

func requireSection(t *testing.T, sections []interface{}, id string) {
	t.Helper()
	for _, raw := range sections {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if got, _ := item["id"].(string); got == id {
			return
		}
	}
	t.Fatalf("expected section %q in %+v", id, sections)
}

func requireTopic(t *testing.T, topics []interface{}, id string) {
	t.Helper()
	for _, raw := range topics {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if got, _ := item["id"].(string); got == id {
			return
		}
	}
	t.Fatalf("expected topic %q in %+v", id, topics)
}
