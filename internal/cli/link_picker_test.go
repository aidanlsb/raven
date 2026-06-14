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
	prevStdinTTY := fzfStdinIsTerminal
	prevStdoutTTY := fzfStdoutIsTerminal
	prevRun := ravenRunPicker
	t.Cleanup(func() {
		jsonOutput = prevJSON
		resolvedVaultPath = prevVaultPath
		fzfStdinIsTerminal = prevStdinTTY
		fzfStdoutIsTerminal = prevStdoutTTY
		ravenRunPicker = prevRun
	})

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/alpha.md", "# Alpha\n").
		Build()
	if _, err := reindexsvc.Run(reindexsvc.RunRequest{VaultPath: v.Path, Full: true, Context: context.Background()}); err != nil {
		t.Fatalf("reindexsvc.Run() error = %v", err)
	}

	jsonOutput = false
	resolvedVaultPath = v.Path
	fzfStdinIsTerminal = func() bool { return true }
	fzfStdoutIsTerminal = func() bool { return true }

	tests := []struct {
		name     string
		prepare  func([]string) ([]string, bool, error)
		build    func([]string) (map[string]interface{}, error)
		prompt   string
		argKey   string
		wantArgs []string
	}{
		{
			name: "backlinks",
			prepare: func(args []string) ([]string, bool, error) {
				return prepareBacklinksArgs(backlinksCmd, args)
			},
			build: func(args []string) (map[string]interface{}, error) {
				return buildBacklinksArgs(backlinksCmd, args)
			},
			prompt:   "backlinks> ",
			argKey:   "target",
			wantArgs: []string{"notes/alpha.md"},
		},
		{
			name: "outlinks",
			prepare: func(args []string) ([]string, bool, error) {
				return prepareOutlinksArgs(outlinksCmd, args)
			},
			build: func(args []string) (map[string]interface{}, error) {
				return buildOutlinksArgs(outlinksCmd, args)
			},
			prompt:   "outlinks> ",
			argKey:   "source",
			wantArgs: []string{"notes/alpha.md"},
		},
		{
			name: "resolve",
			prepare: func(args []string) ([]string, bool, error) {
				return prepareResolveArgs(resolveCmd, args)
			},
			build: func(args []string) (map[string]interface{}, error) {
				return buildResolveArgs(resolveCmd, args)
			},
			prompt:   "resolve> ",
			argKey:   "reference",
			wantArgs: []string{"notes/alpha.md"},
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
				if !slices.Contains(ids, "notes/alpha.md") {
					t.Fatalf("expected indexed file in picker items, got %#v", ids)
				}
				return picker.Selection{Item: picker.Item{ID: "notes/alpha.md"}}, true, nil
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
			if got := argsMap[tt.argKey]; got != "notes/alpha.md" {
				t.Fatalf("%s arg = %#v, want notes/alpha.md", tt.argKey, got)
			}
		})
	}
}
