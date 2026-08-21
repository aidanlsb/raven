package linktarget

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeClassifiesTargetsAndExtractsExtensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		wantScheme Scheme
		wantExt    string
	}{
		{name: "relative file", target: "../assets/Report.PDF", wantScheme: SchemeFile, wantExt: "pdf"},
		{name: "extensionless file", target: "../assets/LICENSE", wantScheme: SchemeFile},
		{name: "dotfile is extensionless", target: "../assets/.gitignore", wantScheme: SchemeFile},
		{name: "url", target: "https://example.com/image.PNG", wantScheme: SchemeURL},
		{name: "protocol relative url", target: "//example.com/image.png", wantScheme: SchemeURL},
		{name: "other scheme", target: "mailto:team@example.com", wantScheme: SchemeOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Analyze(tt.target, "notes/source.md", "/vault")
			if got.Scheme != tt.wantScheme {
				t.Fatalf("scheme = %q, want %q", got.Scheme, tt.wantScheme)
			}
			if got.Ext != tt.wantExt {
				t.Fatalf("ext = %q, want %q", got.Ext, tt.wantExt)
			}
		})
	}
}

func TestAnalyzeNormalizesFilePathVariantsToVaultRelativeKey(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	absoluteTarget := filepath.Join(vaultPath, "assets", "x.pdf")
	fileURI := "file:///" + strings.TrimPrefix(filepath.ToSlash(absoluteTarget), "/")
	tests := []struct {
		name       string
		sourceFile string
		target     string
	}{
		{name: "source relative", sourceFile: "source.md", target: "assets/x.pdf"},
		{name: "explicit source relative", sourceFile: "source.md", target: "./assets/x.pdf"},
		{name: "parent relative", sourceFile: "notes/source.md", target: "../assets/x.pdf"},
		{name: "absolute inside vault", sourceFile: "notes/source.md", target: absoluteTarget},
		{name: "file URI inside vault", sourceFile: "notes/source.md", target: fileURI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Analyze(tt.target, tt.sourceFile, vaultPath)
			if got.NormalizedKey != "assets/x.pdf" {
				t.Fatalf("normalized key = %q, want %q", got.NormalizedKey, "assets/x.pdf")
			}
		})
	}
}

func TestAnalyzeKeepsAbsolutePathsOutsideVaultAbsolute(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	target := filepath.Join(filepath.Dir(vaultPath), "outside", "..", "outside", "x.pdf")
	got := Analyze(target, "source.md", vaultPath)
	if got.NormalizedKey != filepath.ToSlash(filepath.Clean(target)) {
		t.Fatalf("normalized key = %q, want %q", got.NormalizedKey, filepath.ToSlash(filepath.Clean(target)))
	}
}

func TestIsVaultRelativeFileKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "vault file", key: "assets/report.pdf", want: true},
		{name: "parent traversal", key: "../report.pdf", want: false},
		{name: "unix absolute", key: "/tmp/report.pdf", want: false},
		{name: "windows drive absolute", key: `C:\files\report.pdf`, want: false},
		{name: "windows UNC absolute", key: `\\server\share\report.pdf`, want: false},
		{name: "empty", key: "", want: false},
		{name: "current directory", key: ".", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsVaultRelativeFileKey(tt.key); got != tt.want {
				t.Errorf("IsVaultRelativeFileKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestAnalyzeAuthoredPreservesEscapedFilenameDelimiters(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	tests := []struct {
		raw      string
		semantic string
		want     string
	}{
		{raw: `../assets/a\#1.pdf`, semantic: "../assets/a#1.pdf", want: "assets/a#1.pdf"},
		{raw: `../assets/a\?1.pdf`, semantic: "../assets/a?1.pdf", want: "assets/a?1.pdf"},
		{raw: `../assets/a.pdf#page=2`, semantic: "../assets/a.pdf#page=2", want: "assets/a.pdf"},
	}
	for _, tt := range tests {
		got := AnalyzeAuthored(tt.raw, tt.semantic, "notes/source.md", vaultPath)
		if got.NormalizedKey != tt.want {
			t.Errorf("AnalyzeAuthored(%q) key = %q, want %q", tt.raw, got.NormalizedKey, tt.want)
		}
	}
}

func TestAnalyzeNormalizesURLsConservatively(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "https default port",
			target: "https://EXAMPLE.COM:443/Case/Sensitive?Key=Value#Fragment",
			want:   "https://example.com/Case/Sensitive?Key=Value#Fragment",
		},
		{
			name:   "http default port with userinfo",
			target: "http://User@EXAMPLE.COM:80/Path/?Q=A#Frag",
			want:   "http://User@example.com/Path/?Q=A#Frag",
		},
		{
			name:   "non-default port retained",
			target: "https://EXAMPLE.COM:8443/Path",
			want:   "https://example.com:8443/Path",
		},
		{
			name:   "empty explicit port retained",
			target: "https://EXAMPLE.COM:/Path",
			want:   "https://example.com:/Path",
		},
		{
			name:   "protocol relative host only",
			target: "//EXAMPLE.COM:443/Path?Q=A#Frag",
			want:   "//example.com:443/Path?Q=A#Frag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Analyze(tt.target, "source.md", "/vault")
			if got.NormalizedKey != tt.want {
				t.Fatalf("normalized key = %q, want %q", got.NormalizedKey, tt.want)
			}
		})
	}
}

func TestIsRavenTarget(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	externalMarkdown := filepath.Join(filepath.Dir(vaultPath), "external", "readme.md")
	tests := []struct {
		target string
		want   bool
	}{
		{target: "notes/next.md", want: true},
		{target: "../next.MD#details", want: true},
		{target: "#details", want: true},
		{target: "assets/report.pdf", want: false},
		{target: externalMarkdown, want: false},
		{target: "https://example.com/readme.md", want: false},
	}
	for _, tt := range tests {
		if got := IsRavenTarget(tt.target, "notes/source.md", vaultPath); got != tt.want {
			t.Errorf("IsRavenTarget(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestIsRavenTargetAuthoredPreservesEscapedDelimiters(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	tests := []struct {
		name     string
		raw      string
		semantic string
		want     bool
	}{
		{name: "escaped hash in markdown filename", raw: `../next\#draft.md`, semantic: "../next#draft.md", want: true},
		{name: "escaped hash in non-markdown filename", raw: `../report\#draft.pdf`, semantic: "../report#draft.pdf", want: false},
		{name: "ordinary fragment", raw: "../next.md#details", semantic: "../next.md#details", want: true},
		{name: "local fragment", raw: "#details", semantic: "#details", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRavenTargetAuthored(tt.raw, tt.semantic, "notes/source.md", vaultPath); got != tt.want {
				t.Errorf("IsRavenTargetAuthored(%q, %q) = %v, want %v", tt.raw, tt.semantic, got, tt.want)
			}
		})
	}
}

func TestRetargetFilePreservesAuthoredStyle(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	absoluteOld := filepath.ToSlash(filepath.Join(vaultPath, "assets", "a(1).pdf"))
	absoluteNew := filepath.ToSlash(filepath.Join(vaultPath, "assets", "archive", "a(1).pdf"))
	tests := []struct {
		name       string
		raw        string
		sourceFile string
		newKey     string
		want       string
	}{
		{
			name:       "parent relative",
			raw:        `../assets/a(1).pdf#page=2`,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a(1).pdf",
			want:       `../assets/archive/a(1).pdf#page=2`,
		},
		{
			name:       "escaped parentheses",
			raw:        `../assets/a\(1\).pdf`,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a(1).pdf",
			want:       `../assets/archive/a\(1\).pdf`,
		},
		{
			name:       "escaped fragment delimiter",
			raw:        `../assets/a\#1.pdf`,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a#1.pdf",
			want:       `../assets/archive/a\#1.pdf`,
		},
		{
			name:       "new fragment delimiter is escaped",
			raw:        `../assets/a.pdf`,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a#1.pdf",
			want:       `../assets/archive/a\#1.pdf`,
		},
		{
			name:       "angle brackets",
			raw:        `<../assets/a(1).pdf?download=1>`,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a(1).pdf",
			want:       `<../assets/archive/a(1).pdf?download=1>`,
		},
		{
			name:       "explicit current directory",
			raw:        `./a.pdf`,
			sourceFile: "assets/source.md",
			newKey:     "assets/archive/a.pdf",
			want:       `./archive/a.pdf`,
		},
		{
			name:       "absolute",
			raw:        absoluteOld,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a(1).pdf",
			want:       absoluteNew,
		},
		{
			name:       "file URI",
			raw:        "file://" + absoluteOld,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a(1).pdf",
			want:       "file://" + absoluteNew,
		},
		{
			name:       "unwrapped destination gains angle brackets for spaces",
			raw:        `../assets/a.pdf`,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a final.pdf",
			want:       `<../assets/archive/a final.pdf>`,
		},
		{
			name:       "unbalanced parenthesis is escaped",
			raw:        `../assets/a.pdf`,
			sourceFile: "notes/source.md",
			newKey:     "assets/archive/a(1.pdf",
			want:       `../assets/archive/a\(1.pdf`,
		},
		{
			name:       "standard Windows file URI",
			raw:        `file:///C:/vault/assets/a.pdf`,
			sourceFile: "notes/source.md",
			newKey:     "C:/vault/assets/archive/a.pdf",
			want:       `file:///C:/vault/assets/archive/a.pdf`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RetargetFile(tt.raw, tt.sourceFile, vaultPath, tt.newKey); got != tt.want {
				t.Fatalf("RetargetFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveFileKey(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if got, want := ResolveFileKey("assets/report.pdf", vaultPath), filepath.Join(vaultPath, "assets", "report.pdf"); got != want {
		t.Fatalf("ResolveFileKey() = %q, want %q", got, want)
	}

	absolute := filepath.Join(filepath.Dir(vaultPath), "external", "report.pdf")
	if got := ResolveFileKey(filepath.ToSlash(absolute), vaultPath); got != filepath.Clean(absolute) {
		t.Fatalf("ResolveFileKey(absolute) = %q, want %q", got, filepath.Clean(absolute))
	}
}
