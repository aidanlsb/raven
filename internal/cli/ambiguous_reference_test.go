package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestAmbiguousReferenceRetryForReadBacklinksAndOutlinks(t *testing.T) {
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
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", `---
type: person
name: Freya
---
# Freya
`).
		WithFile("projects/source.md", `---
type: project
title: Source
---
See [[people/freya]].
`).
		Build()
	if _, err := reindexsvc.Run(reindexsvc.RunRequest{VaultPath: v.Path, Full: true, Context: context.Background()}); err != nil {
		t.Fatalf("reindexsvc.Run() error = %v", err)
	}

	jsonOutput = false
	resolvedVaultPath = v.Path
	interactiveStdinIsTerminal = func() bool { return true }
	interactiveStdoutIsTerminal = func() bool { return true }

	tests := []struct {
		name       string
		prompt     string
		selected   string
		handle     func(*cobra.Command, commandexec.Result) error
		cmd        *cobra.Command
		wantOutput string
	}{
		{
			name:       "read",
			prompt:     "read/ref",
			selected:   "people/freya",
			handle:     handleCanonicalReadFailureCmd,
			cmd:        newReadRetryTestCmd(t),
			wantOutput: "Freya",
		},
		{
			name:       "backlinks",
			prompt:     "backlinks/ref",
			selected:   "people/freya",
			handle:     handleBacklinksFailure,
			cmd:        newBrowseFlagTestCmd(t),
			wantOutput: "projects/source.md",
		},
		{
			name:       "outlinks",
			prompt:     "outlinks/ref",
			selected:   "projects/source",
			handle:     handleOutlinksFailure,
			cmd:        newBrowseFlagTestCmd(t),
			wantOutput: "people/freya",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ravenRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
				if opts.Prompt != tt.prompt {
					t.Fatalf("prompt = %q, want %q", opts.Prompt, tt.prompt)
				}
				if len(items) != 2 {
					t.Fatalf("expected 2 ambiguous choices, got %d", len(items))
				}
				return picker.Selection{Item: picker.Item{ID: tt.selected}}, true, nil
			}

			out := captureStdout(t, func() {
				if err := tt.handle(tt.cmd, ambiguousReferenceResult(tt.selected)); err != nil {
					t.Fatalf("handle ambiguous reference: %v", err)
				}
			})
			if !strings.Contains(out, tt.wantOutput) {
				t.Fatalf("output missing %q:\n%s", tt.wantOutput, out)
			}
		})
	}
}

func ambiguousReferenceResult(selected string) commandexec.Result {
	return commandexec.Failure(ErrRefAmbiguous, "reference is ambiguous", map[string]interface{}{
		"reference":     "freya",
		"matches":       []string{selected, "people/freya-alt"},
		"match_sources": map[string]string{selected: "name_field", "people/freya-alt": "name_field"},
	}, "")
}

func newReadRetryTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().Bool("raw", false, "")
	cmd.Flags().Bool("lines", false, "")
	cmd.Flags().Int("start-line", 0, "")
	cmd.Flags().Int("end-line", 0, "")
	return cmd
}

func newBrowseFlagTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().Bool("browse", false, "")
	return cmd
}
