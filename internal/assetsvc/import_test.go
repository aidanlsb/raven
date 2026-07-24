package assetsvc

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func TestImportCopiesAndPlansDirectoryDestination(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	sourcePath := writeSource(t, "paper.pdf", "pdf-data")

	result, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      sourcePath,
		Destination: "assets/pdfs/",
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.DestinationRel != "assets/pdfs/paper.pdf" {
		t.Fatalf("destination = %q, want assets/pdfs/paper.pdf", result.DestinationRel)
	}
	if result.Mode != ModeCopy {
		t.Fatalf("mode = %q, want copy", result.Mode)
	}
	if result.Asset.ID != result.DestinationRel || result.Asset.SizeBytes != int64(len("pdf-data")) {
		t.Fatalf("asset = %#v", result.Asset)
	}
	if len(result.ChangeSet.Changed) != 1 || result.ChangeSet.Changed[0] != result.DestinationRel {
		t.Fatalf("change set = %#v", result.ChangeSet)
	}
	assertFileContent(t, filepath.Join(vaultPath, filepath.FromSlash(result.DestinationRel)), "pdf-data")
	assertFileContent(t, sourcePath, "pdf-data")
}

func TestImportRecognizesExistingDirectoryWithoutSlash(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultPath, "assets", "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := writeSource(t, "photo.png", "png-data")

	result, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      sourcePath,
		Destination: "assets/uploads",
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.DestinationRel != "assets/uploads/photo.png" {
		t.Fatalf("destination = %q, want assets/uploads/photo.png", result.DestinationRel)
	}
}

func TestImportDryRunDoesNotWrite(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	sourcePath := writeSource(t, "data.csv", "a,b\n1,2\n")

	result, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      sourcePath,
		Destination: "assets/data/data.csv",
		Move:        true,
		Preview:     true,
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.Mode != ModeMove {
		t.Fatalf("mode = %q, want move", result.Mode)
	}
	if !result.ChangeSet.Empty() {
		t.Fatalf("preview change set = %#v, want empty", result.ChangeSet)
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "assets", "data", "data.csv")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote destination: %v", err)
	}
	assertFileContent(t, sourcePath, "a,b\n1,2\n")
}

func TestImportMoveRemovesSourceOnlyWhenFinalized(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	sourcePath := writeSource(t, "recording.wav", "audio")

	result, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      sourcePath,
		Destination: "assets/audio/recording.wav",
		Move:        true,
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	assertFileContent(t, sourcePath, "audio")
	if err := FinalizeMove(result); err != nil {
		t.Fatalf("FinalizeMove() error = %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source still exists after finalize: %v", err)
	}
	assertFileContent(t, filepath.Join(vaultPath, "assets", "audio", "recording.wav"), "audio")
}

func TestFinalizeMoveDoesNotRemoveChangedSource(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	sourcePath := writeSource(t, "recording.wav", "original")
	result, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      sourcePath,
		Destination: "assets/audio/recording.wav",
		Move:        true,
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("changed after copy"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = FinalizeMove(result)
	requireServiceCode(t, err, codes.ErrFileWrite)
	assertFileContent(t, sourcePath, "changed after copy")
	assertFileContent(t, filepath.Join(vaultPath, "assets", "audio", "recording.wav"), "original")
}

func TestImportCollisionRequiresForce(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	destination := filepath.Join(vaultPath, "assets", "data", "values.csv")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := writeSource(t, "values.csv", "new")

	_, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      sourcePath,
		Destination: "assets/data/values.csv",
	})
	requireServiceCode(t, err, codes.ErrFileExists)
	assertFileContent(t, destination, "old")

	result, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      sourcePath,
		Destination: "assets/data/values.csv",
		Force:       true,
	})
	if err != nil {
		t.Fatalf("forced Import() error = %v", err)
	}
	assertFileContent(t, destination, "new")
	if result.Asset.SizeBytes != 3 {
		t.Fatalf("size = %d, want 3", result.Asset.SizeBytes)
	}
}

func TestImportUsesCustomAssetRootAndHomeExpansion(t *testing.T) {
	home := t.TempDir()
	sourcePath := filepath.Join(home, "Downloads", "chart.svg")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	vaultPath := t.TempDir()
	result, err := Import(ImportRequest{
		VaultPath: vaultPath,
		VaultConfig: &config.VaultConfig{Directories: &config.DirectoriesConfig{
			Assets: "files/",
		}},
		Source:      "~/Downloads/chart.svg",
		Destination: "files/diagrams/",
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.DestinationRel != "files/diagrams/chart.svg" {
		t.Fatalf("destination = %q", result.DestinationRel)
	}
}

func TestImportRejectsInvalidSourcesAndDestinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sourceName  string
		destination string
		setup       func(t *testing.T, vaultPath string) string
		code        codes.ErrorCode
	}{
		{
			name:        "relative source",
			destination: "assets/file.pdf",
			setup:       func(_ *testing.T, _ string) string { return "file.pdf" },
			code:        codes.ErrInvalidInput,
		},
		{
			name:        "missing source",
			destination: "assets/file.pdf",
			setup:       func(t *testing.T, _ string) string { return filepath.Join(t.TempDir(), "missing.pdf") },
			code:        codes.ErrFileNotFound,
		},
		{
			name:        "directory source",
			destination: "assets/file.pdf",
			setup:       func(t *testing.T, _ string) string { return t.TempDir() },
			code:        codes.ErrInvalidInput,
		},
		{
			name:        "markdown source",
			destination: "assets/file.bin",
			setup:       func(t *testing.T, _ string) string { return writeSource(t, "note.MD", "markdown") },
			code:        codes.ErrInvalidInput,
		},
		{
			name:        "source inside vault",
			destination: "assets/copy.pdf",
			setup: func(t *testing.T, vaultPath string) string {
				source := filepath.Join(vaultPath, "assets", "source.pdf")
				if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(source, []byte("pdf"), 0o644); err != nil {
					t.Fatal(err)
				}
				return source
			},
			code: codes.ErrFileOutsideVault,
		},
		{
			name:        "destination outside asset root",
			destination: "downloads/file.pdf",
			setup:       func(t *testing.T, _ string) string { return writeSource(t, "file.pdf", "pdf") },
			code:        codes.ErrFileOutsideVault,
		},
		{
			name:        "destination escapes vault",
			destination: "../assets/file.pdf",
			setup:       func(t *testing.T, _ string) string { return writeSource(t, "file.pdf", "pdf") },
			code:        codes.ErrFileOutsideVault,
		},
		{
			name:        "backslash destination escapes asset root",
			destination: `assets\..\outside.pdf`,
			setup:       func(t *testing.T, _ string) string { return writeSource(t, "file.pdf", "pdf") },
			code:        codes.ErrFileOutsideVault,
		},
		{
			name:        "destination has no extension",
			destination: "assets/file",
			setup:       func(t *testing.T, _ string) string { return writeSource(t, "file.pdf", "pdf") },
			code:        codes.ErrInvalidInput,
		},
		{
			name:        "destination has empty extension",
			destination: "assets/file.",
			setup:       func(t *testing.T, _ string) string { return writeSource(t, "file.pdf", "pdf") },
			code:        codes.ErrInvalidInput,
		},
		{
			name:        "markdown destination",
			destination: "assets/file.md",
			setup:       func(t *testing.T, _ string) string { return writeSource(t, "file.pdf", "pdf") },
			code:        codes.ErrInvalidInput,
		},
		{
			name:        "absolute destination",
			destination: "/assets/file.pdf",
			setup:       func(t *testing.T, _ string) string { return writeSource(t, "file.pdf", "pdf") },
			code:        codes.ErrInvalidInput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := t.TempDir()
			source := tc.setup(t, vaultPath)
			_, err := Import(ImportRequest{
				VaultPath:   vaultPath,
				VaultConfig: config.DefaultVaultConfig(),
				Source:      source,
				Destination: tc.destination,
			})
			requireServiceCode(t, err, tc.code)
		})
	}
}

func TestImportRejectsDestinationSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires privileges on Windows")
	}
	t.Parallel()

	vaultPath := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultPath, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(vaultPath, "assets", "escape")); err != nil {
		t.Fatal(err)
	}
	source := writeSource(t, "file.pdf", "pdf")

	_, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      source,
		Destination: "assets/escape/file.pdf",
	})
	requireServiceCode(t, err, codes.ErrFileOutsideVault)
}

func TestImportRejectsDestinationSymlinkInsideVault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires privileges on Windows")
	}
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultPath, "assets", "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(vaultPath, "assets", "real"), filepath.Join(vaultPath, "assets", "linked")); err != nil {
		t.Fatal(err)
	}
	source := writeSource(t, "file.pdf", "pdf")

	_, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      source,
		Destination: "assets/linked/file.pdf",
	})
	requireServiceCode(t, err, codes.ErrFileOutsideVault)
}

func TestImportRejectsBackslashTraversalInPreservedBasename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backslash is a path separator on Windows")
	}
	t.Parallel()

	vaultPath := t.TempDir()
	source := writeSource(t, `..\outside.pdf`, "pdf")
	_, err := Import(ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Source:      source,
		Destination: "assets/",
	})
	requireServiceCode(t, err, codes.ErrInvalidInput)
}

func writeSource(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s content = %q, want %q", path, data, want)
	}
}

func requireServiceCode(t *testing.T, err error, want codes.ErrorCode) {
	t.Helper()
	serviceErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("error = %v, want service code %s", err, want)
	}
	if serviceErr.Code != want {
		t.Fatalf("code = %s, want %s (error: %v)", serviceErr.Code, want, err)
	}
}
