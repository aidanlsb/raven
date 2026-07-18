package cli

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestJoinQueryArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "single arg unchanged",
			args: []string{`trait:due content("hello world")`},
			want: `trait:due content("hello world")`,
		},
		{
			name: "multiple args joined with space",
			args: []string{"trait:due", ".value==past"},
			want: "trait:due .value==past",
		},
		{
			name: "mixed predicates",
			args: []string{"trait:due", `content("my task")`, ".value==past"},
			want: `trait:due content("my task") .value==past`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinQueryArgs(tt.args)
			if got != tt.want {
				t.Errorf("joinQueryArgs(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestSavedQueryOptionsFromFlags(t *testing.T) {
	cmd := newQueryOptionsTestCommand()
	if err := cmd.Flags().Set("browse", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("limit", "100"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("apply", "set status=done"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("confirm", "true"); err != nil {
		t.Fatal(err)
	}

	options := savedQueryOptionsFromFlags(cmd)
	if options == nil {
		t.Fatalf("savedQueryOptionsFromFlags() = nil, want options")
	}
	if options.Browse == nil || !*options.Browse {
		t.Fatalf("Browse = %#v, want true", options.Browse)
	}
	if options.Limit == nil || *options.Limit != 100 {
		t.Fatalf("Limit = %#v, want 100", options.Limit)
	}
	if !reflect.DeepEqual(options.Apply, []string{"set status=done"}) {
		t.Fatalf("Apply = %#v, want set status=done", options.Apply)
	}
	if options.Confirm == nil || !*options.Confirm {
		t.Fatalf("Confirm = %#v, want true", options.Confirm)
	}
}

func TestSavedQueryOptionsFromFlagsNoPipe(t *testing.T) {
	cmd := newQueryOptionsTestCommand()
	if err := cmd.Flags().Set("no-pipe", "true"); err != nil {
		t.Fatal(err)
	}

	options := savedQueryOptionsFromFlags(cmd)
	if options == nil || options.Pipe == nil {
		t.Fatalf("Pipe = %#v, want explicit false", options)
	}
	if *options.Pipe {
		t.Fatalf("Pipe = true, want false")
	}
}

func TestEffectiveQueryBrowseSavedDefaultDegradesOffTerminal(t *testing.T) {
	prevJSON := jsonOutput
	prevStdinTTY := interactiveStdinIsTerminal
	prevStdoutTTY := interactiveStdoutIsTerminal
	t.Cleanup(func() {
		jsonOutput = prevJSON
		interactiveStdinIsTerminal = prevStdinTTY
		interactiveStdoutIsTerminal = prevStdoutTTY
	})

	jsonOutput = false
	interactiveStdinIsTerminal = func() bool { return true }
	interactiveStdoutIsTerminal = func() bool { return true }
	if !effectiveQueryBrowse(true, false) {
		t.Fatalf("saved browse default should apply on an interactive terminal")
	}

	interactiveStdoutIsTerminal = func() bool { return false }
	if effectiveQueryBrowse(true, false) {
		t.Fatalf("saved browse default should degrade when stdout is not a terminal")
	}
	if !effectiveQueryBrowse(true, true) {
		t.Fatalf("explicit --browse must be preserved (validated later) even off-terminal")
	}

	interactiveStdoutIsTerminal = func() bool { return true }
	jsonOutput = true
	if effectiveQueryBrowse(true, false) {
		t.Fatalf("saved browse default should degrade in JSON mode")
	}

	jsonOutput = false
	if effectiveQueryBrowse(false, false) || effectiveQueryBrowse(false, true) {
		t.Fatalf("browse=false should never be enabled")
	}
}

func newQueryOptionsTestCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("refresh", false, "")
	cmd.Flags().Bool("ids", false, "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().Int("offset", 0, "")
	cmd.Flags().Bool("count-only", false, "")
	cmd.Flags().StringArray("apply", nil, "")
	cmd.Flags().Bool("confirm", false, "")
	cmd.Flags().Bool("pipe", false, "")
	cmd.Flags().Bool("no-pipe", false, "")
	cmd.Flags().Bool("browse", false, "")
	return cmd
}
