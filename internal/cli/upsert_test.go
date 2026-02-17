package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertCreateUpdateUnchanged(t *testing.T) {
	vaultPath := t.TempDir()
	writeUpsertTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := upsertFieldFlags
	prevContent := upsertContent
	prevPath := upsertPathFlag
	prevPathChanged := upsertCmd.Flags().Lookup("path").Changed
	prevContentChanged := upsertCmd.Flags().Lookup("content").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		upsertFieldFlags = prevFields
		upsertContent = prevContent
		upsertPathFlag = prevPath
		upsertCmd.Flags().Lookup("path").Changed = prevPathChanged
		upsertCmd.Flags().Lookup("content").Changed = prevContentChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	upsertFieldFlags = nil
	upsertPathFlag = ""
	upsertCmd.Flags().Lookup("path").Changed = false

	run := func(content string) (status string, file string) {
		upsertContent = content
		upsertCmd.Flags().Lookup("content").Changed = true
		out := captureStdout(t, func() {
			if err := upsertCmd.RunE(upsertCmd, []string{"brief", "Daily Brief 2026-02-14"}); err != nil {
				t.Fatalf("upsertCmd.RunE: %v", err)
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

func TestUpsertVsAddBoundary(t *testing.T) {
	vaultPath := t.TempDir()
	writeUpsertTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevUpsertFields := upsertFieldFlags
	prevUpsertContent := upsertContent
	prevUpsertPath := upsertPathFlag
	prevUpsertPathChanged := upsertCmd.Flags().Lookup("path").Changed
	prevUpsertContentChanged := upsertCmd.Flags().Lookup("content").Changed
	prevAddTo := addToFlag
	prevAddTimestamp := addTimestampFlag
	prevAddStdin := addStdin
	prevAddConfirm := addConfirm
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		upsertFieldFlags = prevUpsertFields
		upsertContent = prevUpsertContent
		upsertPathFlag = prevUpsertPath
		upsertCmd.Flags().Lookup("path").Changed = prevUpsertPathChanged
		upsertCmd.Flags().Lookup("content").Changed = prevUpsertContentChanged
		addToFlag = prevAddTo
		addTimestampFlag = prevAddTimestamp
		addStdin = prevAddStdin
		addConfirm = prevAddConfirm
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	upsertFieldFlags = nil
	upsertPathFlag = ""
	upsertCmd.Flags().Lookup("path").Changed = false
	upsertContent = "Canonical body"
	upsertCmd.Flags().Lookup("content").Changed = true

	var objectID string
	var relFile string
	out := captureStdout(t, func() {
		if err := upsertCmd.RunE(upsertCmd, []string{"brief", "Daily Brief Boundary"}); err != nil {
			t.Fatalf("upsert create failed: %v", err)
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

	addToFlag = objectID
	addTimestampFlag = false
	addStdin = false
	addConfirm = false
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

	upsertContent = "Canonical replacement"
	upsertCmd.Flags().Lookup("content").Changed = true
	_ = captureStdout(t, func() {
		if err := upsertCmd.RunE(upsertCmd, []string{"brief", "Daily Brief Boundary"}); err != nil {
			t.Fatalf("upsert update failed: %v", err)
		}
	})

	finalBytes, err := os.ReadFile(filepath.Join(vaultPath, relFile))
	if err != nil {
		t.Fatalf("read file after upsert replace: %v", err)
	}
	final := string(finalBytes)
	if !strings.Contains(final, "Canonical replacement") {
		t.Fatalf("expected replacement body, got:\n%s", final)
	}
	if strings.Contains(final, "appended line") {
		t.Fatalf("expected upsert to replace body (remove appended line), got:\n%s", final)
	}
}

func TestUpsertRejectsTitleWithPathSeparator(t *testing.T) {
	vaultPath := t.TempDir()
	writeUpsertTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := upsertFieldFlags
	prevContent := upsertContent
	prevPath := upsertPathFlag
	prevPathChanged := upsertCmd.Flags().Lookup("path").Changed
	prevContentChanged := upsertCmd.Flags().Lookup("content").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		upsertFieldFlags = prevFields
		upsertContent = prevContent
		upsertPathFlag = prevPath
		upsertCmd.Flags().Lookup("path").Changed = prevPathChanged
		upsertCmd.Flags().Lookup("content").Changed = prevContentChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	upsertFieldFlags = nil
	upsertPathFlag = ""
	upsertCmd.Flags().Lookup("path").Changed = false
	upsertContent = ""
	upsertCmd.Flags().Lookup("content").Changed = false

	out := captureStdout(t, func() {
		if err := upsertCmd.RunE(upsertCmd, []string{"brief", "folder/name"}); err != nil {
			t.Fatalf("upsertCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false, got true; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidInput {
		t.Fatalf("expected error.code=%s, got %#v; out=%s", ErrInvalidInput, resp.Error, out)
	}
	if !strings.Contains(resp.Error.Message, "title cannot contain path separators") {
		t.Fatalf("expected path separator validation message, got: %q", resp.Error.Message)
	}
}

func TestUpsertUsesExplicitPathWhenProvided(t *testing.T) {
	vaultPath := t.TempDir()
	writeUpsertTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := upsertFieldFlags
	prevContent := upsertContent
	prevPath := upsertPathFlag
	prevPathChanged := upsertCmd.Flags().Lookup("path").Changed
	prevContentChanged := upsertCmd.Flags().Lookup("content").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		upsertFieldFlags = prevFields
		upsertContent = prevContent
		upsertPathFlag = prevPath
		upsertCmd.Flags().Lookup("path").Changed = prevPathChanged
		upsertCmd.Flags().Lookup("content").Changed = prevContentChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	upsertFieldFlags = nil
	upsertPathFlag = "custom/brief-daily"
	upsertCmd.Flags().Lookup("path").Changed = true
	upsertContent = "Body V1"
	upsertCmd.Flags().Lookup("content").Changed = true

	out := captureStdout(t, func() {
		if err := upsertCmd.RunE(upsertCmd, []string{"brief", "Daily Brief"}); err != nil {
			t.Fatalf("upsertCmd.RunE: %v", err)
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

func TestUpsertRejectsDirectoryOnlyPath(t *testing.T) {
	vaultPath := t.TempDir()
	writeUpsertTestSchema(t, vaultPath)

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := upsertFieldFlags
	prevContent := upsertContent
	prevPath := upsertPathFlag
	prevPathChanged := upsertCmd.Flags().Lookup("path").Changed
	prevContentChanged := upsertCmd.Flags().Lookup("content").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		upsertFieldFlags = prevFields
		upsertContent = prevContent
		upsertPathFlag = prevPath
		upsertCmd.Flags().Lookup("path").Changed = prevPathChanged
		upsertCmd.Flags().Lookup("content").Changed = prevContentChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	upsertFieldFlags = nil
	upsertPathFlag = "brief/"
	upsertCmd.Flags().Lookup("path").Changed = true
	upsertContent = ""
	upsertCmd.Flags().Lookup("content").Changed = false

	out := captureStdout(t, func() {
		if err := upsertCmd.RunE(upsertCmd, []string{"brief", "Daily Brief"}); err != nil {
			t.Fatalf("upsertCmd.RunE: %v", err)
		}
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
	if resp.Error == nil || resp.Error.Code != ErrInvalidInput {
		t.Fatalf("expected error.code=%s, got %#v; out=%s", ErrInvalidInput, resp.Error, out)
	}
}

func writeUpsertTestSchema(t *testing.T, vaultPath string) {
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
