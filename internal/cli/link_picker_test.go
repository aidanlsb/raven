package cli

import (
	"context"
	"slices"
	"testing"

	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestPrepareLinkArgsUsesFZFWhenBare(t *testing.T) {
	prevJSON := jsonOutput
	prevVaultPath := resolvedVaultPath
	prevLookPath := fzfLookPath
	prevStdinTTY := fzfStdinIsTerminal
	prevStdoutTTY := fzfStdoutIsTerminal
	prevRun := fzfRunPicker
	t.Cleanup(func() {
		jsonOutput = prevJSON
		resolvedVaultPath = prevVaultPath
		fzfLookPath = prevLookPath
		fzfStdinIsTerminal = prevStdinTTY
		fzfStdoutIsTerminal = prevStdoutTTY
		fzfRunPicker = prevRun
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
	fzfLookPath = func(string) (string, error) { return "/usr/local/bin/fzf", nil }
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
			fzfRunPicker = func(lines []string, opts fzfPickerOptions) (string, bool, error) {
				if opts.Prompt != tt.prompt {
					t.Fatalf("prompt = %q, want %q", opts.Prompt, tt.prompt)
				}
				if !slices.Contains(lines, "notes/alpha.md") {
					t.Fatalf("expected indexed file in fzf lines, got %#v", lines)
				}
				return "notes/alpha.md", true, nil
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
