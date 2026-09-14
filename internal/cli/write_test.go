package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/ui"
)

// resetCommandFlags resets all flags on a command to their default values and clears the changed state.
func resetWriteFlags() {
	writeCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		if f.Value.Type() == "stringArray" || f.Value.Type() == "stringSlice" {
			return
		}
		_ = f.Value.Set(f.DefValue)
	})
}

// setupJSONMode sets up JSON mode for tests by setting both the global and the persistent flag.
func setupWriteJSONMode() error {
	jsonOutput = true
	if writeCmd.Parent() != nil {
		return writeCmd.Parent().PersistentFlags().Set("json", "true")
	}
	return nil
}

func TestWriteCreateUpdateUnchanged(t *testing.T) {
	vaultPath := t.TempDir()
	writeCommandTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		resetWriteFlags()
	})

	resolvedVaultPath = vaultPath

	run := func(content string) (status string, file string) {
		resetWriteFlags()
		if err := setupWriteJSONMode(); err != nil {
			t.Fatalf("setupJSONMode: %v", err)
		}
		if err := writeCmd.ParseFlags([]string{"--content", content}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		out := captureStdout(t, func() {
			if err := writeCmd.RunE(writeCmd, []string{"brief", "Daily Brief 2026-02-14"}); err != nil {
				t.Fatalf("writeCmd.RunE: %v", err)
			}
		})

		var resp struct {
			OK   bool `json:"ok"`
			Data struct {
				Status string `json:"status"`
				File   string `json:"file"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatalf("failed to parse JSON response: %v; out=%s", err, out)
		}
		if !resp.OK {
			t.Fatalf("expected ok=true, got false; out=%s", out)
		}
		return resp.Data.Status, resp.Data.File
	}

	status, file := run("# Brief V1")
	if status != "created" {
		t.Fatalf("expected status=created, got %q", status)
	}

	status, _ = run("# Brief V1")
	if status != "unchanged" {
		t.Fatalf("expected status=unchanged, got %q", status)
	}

	status, _ = run("# Brief V2")
	if status != "updated" {
		t.Fatalf("expected status=updated, got %q", status)
	}

	b, err := os.ReadFile(filepath.Join(vaultPath, file))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "# Brief V2") {
		t.Fatalf("expected updated body content, got:\n%s", got)
	}
	if strings.Contains(got, "# Brief V1") {
		t.Fatalf("expected old body content to be replaced, got:\n%s", got)
	}
}

func TestWriteVsAddBoundary(t *testing.T) {
	vaultPath := t.TempDir()
	writeCommandTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		resetWriteFlags()
		// Also reset add command flags
		addCmd.Flags().VisitAll(func(f *pflag.Flag) {
			f.Changed = false
			_ = f.Value.Set(f.DefValue)
		})
	})

	resolvedVaultPath = vaultPath

	resetWriteFlags()
	if err := setupWriteJSONMode(); err != nil {
		t.Fatalf("setupJSONMode: %v", err)
	}
	if err := writeCmd.ParseFlags([]string{"--content", "Canonical body"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	var objectID string
	var relFile string
	out := captureStdout(t, func() {
		if err := writeCmd.RunE(writeCmd, []string{"brief", "Daily Brief Boundary"}); err != nil {
			t.Fatalf("write create failed: %v", err)
		}
	})
	var createResp struct {
		OK   bool `json:"ok"`
		Data struct {
			ID   string `json:"id"`
			File string `json:"file"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &createResp); err != nil {
		t.Fatalf("parse create response: %v; out=%s", err, out)
	}
	objectID = createResp.Data.ID
	relFile = createResp.Data.File

	// Reset and parse flags for add command
	addCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
	if err := addCmd.ParseFlags([]string{"--to", objectID}); err != nil {
		t.Fatalf("ParseFlags for add: %v", err)
	}
	_ = captureStdout(t, func() {
		if err := addCmd.RunE(addCmd, []string{"appended line"}); err != nil {
			t.Fatalf("addCmd.RunE failed: %v", err)
		}
	})

	withAppendBytes, err := os.ReadFile(filepath.Join(vaultPath, relFile))
	if err != nil {
		t.Fatalf("read file after add: %v", err)
	}
	withAppend := string(withAppendBytes)
	if !strings.Contains(withAppend, "Canonical body") || !strings.Contains(withAppend, "appended line") {
		t.Fatalf("expected add to append content, got:\n%s", withAppend)
	}

	resetWriteFlags()
	if err := writeCmd.ParseFlags([]string{"--content", "Canonical replacement"}); err != nil {
		t.Fatalf("ParseFlags for write: %v", err)
	}
	_ = captureStdout(t, func() {
		if err := writeCmd.RunE(writeCmd, []string{"brief", "Daily Brief Boundary"}); err != nil {
			t.Fatalf("write update failed: %v", err)
		}
	})

	finalBytes, err := os.ReadFile(filepath.Join(vaultPath, relFile))
	if err != nil {
		t.Fatalf("read file after write replace: %v", err)
	}
	final := string(finalBytes)
	if !strings.Contains(final, "Canonical replacement") {
		t.Fatalf("expected replacement body, got:\n%s", final)
	}
	if strings.Contains(final, "appended line") {
		t.Fatalf("expected write to replace body (remove appended line), got:\n%s", final)
	}
}

func TestWriteSlugifiesTitleWithPathSeparator(t *testing.T) {
	vaultPath := t.TempDir()
	writeCommandTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		resetWriteFlags()
	})

	resolvedVaultPath = vaultPath

	title := "config.VaultConfig duplicates internal/paths"
	resetWriteFlags()
	if err := setupWriteJSONMode(); err != nil {
		t.Fatalf("setupJSONMode: %v", err)
	}
	if err := writeCmd.ParseFlags([]string{}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	out := captureStdout(t, func() {
		if err := writeCmd.RunE(writeCmd, []string{"brief", title}); err != nil {
			t.Fatalf("writeCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Status string `json:"status"`
			File   string `json:"file"`
			ID     string `json:"id"`
			Title  string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true, got false; out=%s", out)
	}
	if resp.Data.Status != "created" {
		t.Fatalf("expected status=created, got %q", resp.Data.Status)
	}
	wantFile := "brief/config-vaultconfig-duplicates-internal-paths.md"
	if resp.Data.File != wantFile {
		t.Fatalf("expected file %q, got %q", wantFile, resp.Data.File)
	}
	wantID := "brief/config-vaultconfig-duplicates-internal-paths"
	if resp.Data.ID != wantID {
		t.Fatalf("expected id %q, got %q", wantID, resp.Data.ID)
	}
	if resp.Data.Title != title {
		t.Fatalf("expected response title %q, got %q", title, resp.Data.Title)
	}

	created := filepath.Join(vaultPath, "brief", "config-vaultconfig-duplicates-internal-paths.md")
	b, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if got := string(b); !strings.Contains(got, "title: "+title) {
		t.Fatalf("expected verbatim title in frontmatter, got:\n%s", got)
	}
}

func TestWriteUsesExplicitPathWhenProvided(t *testing.T) {
	vaultPath := t.TempDir()
	writeCommandTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		resetWriteFlags()
	})

	resolvedVaultPath = vaultPath

	resetWriteFlags()
	if err := setupWriteJSONMode(); err != nil {
		t.Fatalf("setupJSONMode: %v", err)
	}
	if err := writeCmd.ParseFlags([]string{"--object-path", "custom/brief-daily", "--content", "Body V1"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	out := captureStdout(t, func() {
		if err := writeCmd.RunE(writeCmd, []string{"brief", "Daily Brief"}); err != nil {
			t.Fatalf("writeCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Status string `json:"status"`
			File   string `json:"file"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true, got false; out=%s", out)
	}
	if resp.Data.Status != "created" {
		t.Fatalf("expected status=created, got %q", resp.Data.Status)
	}
	if resp.Data.File != "custom/brief-daily.md" {
		t.Fatalf("expected explicit path to be used, got %q", resp.Data.File)
	}
}

func TestWriteRejectsDirectoryOnlyPath(t *testing.T) {
	vaultPath := t.TempDir()
	writeCommandTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		resetWriteFlags()
	})

	resolvedVaultPath = vaultPath

	resetWriteFlags()
	if err := setupWriteJSONMode(); err != nil {
		t.Fatalf("setupJSONMode: %v", err)
	}
	if err := writeCmd.ParseFlags([]string{"--object-path", "brief/"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	out := captureStdout(t, func() {
		requireJSONResponseFailure(t, writeCmd.RunE(writeCmd, []string{"brief", "Daily Brief"}))
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false, got true; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != string(ErrInvalidInput) {
		t.Fatalf("expected error.code=%s, got %#v; out=%s", ErrInvalidInput, resp.Error, out)
	}
}

func TestWriteHumanOutputShowsLinkAs(t *testing.T) {
	vaultPath := t.TempDir()
	writeCommandTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevCfg := cfg
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		cfg = prevCfg
		resetWriteFlags()
	})

	resolvedVaultPath = vaultPath
	jsonOutput = false
	// GetEditor() falls back to $EDITOR, so clear it or this test launches a
	// real editor and hangs.
	t.Setenv("EDITOR", "")
	cfg = &config.Config{}

	resetWriteFlags()
	if err := writeCmd.ParseFlags([]string{"--content", "# Brief"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	out := captureStdout(t, func() {
		if err := writeCmd.RunE(writeCmd, []string{"brief", "Daily Brief 2026-02-14"}); err != nil {
			t.Fatalf("writeCmd.RunE: %v", err)
		}
	})

	if !strings.Contains(out, "link as") {
		t.Fatalf("expected 'link as' hint in human output, got:\n%s", out)
	}
	if !strings.Contains(out, ui.LinkAs("brief/daily-brief-2026-02-14")) {
		t.Fatalf("expected link-as line for %q in human output, got:\n%s", "brief/daily-brief-2026-02-14", out)
	}
}

func writeCommandTestSchema(t *testing.T, vaultPath string) {
	t.Helper()
	schemaYAML := strings.TrimSpace(`
version: 2
types:
  brief:
    default_path: brief/
    name_field: title
    fields:
      title:
        type: string
        required: true
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}
}

func TestUpsertCommandRemoved(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "upsert" {
			t.Fatal("upsert command must not exist after the hard rename to write")
		}
	}
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "write" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("write command is missing")
	}
}
