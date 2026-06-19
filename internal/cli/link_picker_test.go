package cli

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestPrepareLinkArgsUsesRavenPickerWhenBare(t *testing.T) {
	prevJSON := jsonOutput
	prevVaultPath := resolvedVaultPath
	prevStdinTTY := interactiveStdinIsTerminal
	prevStdoutTTY := interactiveStdoutIsTerminal
	prevRun := ravenRunPicker
	t.Cleanup(func() {
		jsonOutput = prevJSON
		resolvedVaultPath = prevVaultPath
		interactiveStdinIsTerminal = prevStdinTTY
		interactiveStdoutIsTerminal = prevStdoutTTY
		ravenRunPicker = prevRun
	})

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/alpha.md", "# Alpha\n\n## Details\n").
		WithFile("assets/paper.pdf", "%PDF-1.4\n").
		Build()
	if _, err := reindexsvc.Run(reindexsvc.RunRequest{VaultPath: v.Path, Full: true, Context: context.Background()}); err != nil {
		t.Fatalf("reindexsvc.Run() error = %v", err)
	}

	jsonOutput = false
	resolvedVaultPath = v.Path
	interactiveStdinIsTerminal = func() bool { return true }
	interactiveStdoutIsTerminal = func() bool { return true }

	tests := []struct {
		name      string
		prepare   func([]string) ([]string, bool, error)
		build     func([]string) (map[string]interface{}, error)
		prompt    string
		argKey    string
		wantArgs  []string
		wantAsset bool
	}{
		{
			name: "backlinks",
			prepare: func(args []string) ([]string, bool, error) {
				return prepareBacklinksArgs(backlinksCmd, args)
			},
			build: func(args []string) (map[string]interface{}, error) {
				return buildBacklinksArgs(backlinksCmd, args)
			},
			prompt:    "backlinks> ",
			argKey:    "target",
			wantArgs:  []string{"notes/alpha#details"},
			wantAsset: true,
		},
		{
			name: "outlinks",
			prepare: func(args []string) ([]string, bool, error) {
				return prepareOutlinksArgs(outlinksCmd, args)
			},
			build: func(args []string) (map[string]interface{}, error) {
				return buildOutlinksArgs(outlinksCmd, args)
			},
			prompt:    "outlinks> ",
			argKey:    "source",
			wantArgs:  []string{"notes/alpha#details"},
			wantAsset: false,
		},
		{
			name: "resolve",
			prepare: func(args []string) ([]string, bool, error) {
				return prepareResolveArgs(resolveCmd, args)
			},
			build: func(args []string) (map[string]interface{}, error) {
				return buildResolveArgs(resolveCmd, args)
			},
			prompt:    "resolve> ",
			argKey:    "reference",
			wantArgs:  []string{"notes/alpha#details"},
			wantAsset: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ravenRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
				if opts.Prompt != strings.TrimSuffix(tt.prompt, "> ") {
					t.Fatalf("prompt = %q, want %q", opts.Prompt, strings.TrimSuffix(tt.prompt, "> "))
				}
				ids := make([]string, 0, len(items))
				for _, item := range items {
					ids = append(ids, item.ID)
				}
				if !slices.Contains(ids, "notes/alpha") {
					t.Fatalf("expected object reference in picker items, got %#v", ids)
				}
				if !slices.Contains(ids, "notes/alpha#details") {
					t.Fatalf("expected section reference in picker items, got %#v", ids)
				}
				if gotAsset := slices.Contains(ids, "assets/paper.pdf"); gotAsset != tt.wantAsset {
					t.Fatalf("asset presence = %v, want %v; ids=%#v", gotAsset, tt.wantAsset, ids)
				}
				if opts.Headers[1] != "reference" {
					t.Fatalf("expected reference picker headers, got %#v", opts.Headers)
				}
				return picker.Selection{Item: picker.Item{ID: "notes/alpha#details"}}, true, nil
			}

			args, handled, err := tt.prepare(nil)
			if err != nil {
				t.Fatalf("prepare() error = %v", err)
			}
			if handled {
				t.Fatalf("prepare() handled command, want canonical execution")
			}
			if !slices.Equal(args, tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, tt.wantArgs)
			}

			argsMap, err := tt.build(args)
			if err != nil {
				t.Fatalf("build() error = %v", err)
			}
			if got := argsMap[tt.argKey]; got != "notes/alpha#details" {
				t.Fatalf("%s arg = %#v, want notes/alpha#details", tt.argKey, got)
			}
		})
	}
}
