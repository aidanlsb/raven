package linktarget

import (
	"path/filepath"
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
	tests := []struct {
		name       string
		sourceFile string
		target     string
	}{
		{name: "source relative", sourceFile: "source.md", target: "assets/x.pdf"},
		{name: "explicit source relative", sourceFile: "source.md", target: "./assets/x.pdf"},
		{name: "parent relative", sourceFile: "notes/source.md", target: "../assets/x.pdf"},
		{name: "absolute inside vault", sourceFile: "notes/source.md", target: absoluteTarget},
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
