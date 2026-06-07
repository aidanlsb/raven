package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
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

func TestMaybeSplitInlineSavedQueryArgs(t *testing.T) {
	queries := map[string]*config.SavedQuery{
		"proj-todos": {
			Query: "trait:todo refs([[{{args.project}}]])",
			Args:  []string{"project"},
		},
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "split saved query with positional input",
			args: []string{"proj-todos raven"},
			want: []string{"proj-todos", "raven"},
		},
		{
			name: "split saved query with key value input",
			args: []string{"proj-todos project=raven"},
			want: []string{"proj-todos", "project=raven"},
		},
		{
			name: "split saved query with quoted positional input",
			args: []string{`proj-todos "raven app"`},
			want: []string{"proj-todos", "raven app"},
		},
		{
			name: "split saved query with quoted key value input",
			args: []string{`proj-todos project="raven app"`},
			want: []string{"proj-todos", "project=raven app"},
		},
		{
			name: "full query remains unchanged",
			args: []string{`trait:todo content("my task")`},
			want: []string{`trait:todo content("my task")`},
		},
		{
			name: "unknown saved query remains unchanged",
			args: []string{"unknown raven"},
			want: []string{"unknown raven"},
		},
		{
			name: "invalid quoting remains unchanged",
			args: []string{`proj-todos "raven`},
			want: []string{`proj-todos "raven`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maybeSplitInlineSavedQueryArgs(tt.args, queries)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("maybeSplitInlineSavedQueryArgs(%v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestBuildUnknownQuerySuggestion_IncludesReadOpenForResolvableRefs(t *testing.T) {
	// Use the real vault via local DB open; this test should stay stable because it uses
	// an in-memory index rather than relying on a specific vault.
	//
	// Create an in-memory DB and insert a known object ID so the resolver can resolve the short name.
	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.DB().Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES (?, ?, ?, ?, '{}')`,
		"project/growth-experiments",
		"objects/project/growth-experiments.md",
		"project",
		1,
	)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	s := buildUnknownQuerySuggestion(db, "growth-experiments", "daily", nil)
	if s == "" {
		t.Fatalf("expected suggestion")
	}
	if !strings.Contains(s, "rvn read") || !strings.Contains(s, "rvn open") {
		t.Fatalf("expected read/open hint, got: %q", s)
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

func TestQueryFlagValuesPreferExplicitFlags(t *testing.T) {
	cmd := newQueryOptionsTestCommand()
	if err := cmd.Flags().Set("browse", "false"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("limit", "50"); err != nil {
		t.Fatal(err)
	}

	savedBrowse := true
	savedLimit := 100
	if got := queryBoolFlagValue(cmd, "browse", &savedBrowse); got {
		t.Fatalf("queryBoolFlagValue() = true, want explicit false")
	}
	if got := queryIntFlagValue(cmd, "limit", &savedLimit); got != 50 {
		t.Fatalf("queryIntFlagValue() = %d, want 50", got)
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
