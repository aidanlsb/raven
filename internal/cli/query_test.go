package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
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

func TestRenderCanonicalQueryResultJSONPreservesCanonicalErrorCodes(t *testing.T) {
	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() {
		jsonOutput = previousJSON
	})

	tests := []struct {
		name string
		code codes.ErrorCode
	}{
		{name: "invalid args", code: codes.ErrInvalidArgs},
		{name: "invalid query", code: codes.ErrQueryInvalid},
		{name: "query not found", code: codes.ErrQueryNotFound},
		{name: "schema invalid previously remapped to internal", code: codes.ErrSchemaInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := commandexec.Failure(
				tt.code,
				"canonical message",
				map[string]interface{}{"source": "canonical"},
				"canonical suggestion",
			)

			out := captureStdout(t, func() {
				requireJSONResponseFailure(t, renderCanonicalQueryResult("", nil, failure))
			})

			var response commandexec.Result
			if err := json.Unmarshal([]byte(out), &response); err != nil {
				t.Fatalf("unmarshal response: %v\noutput: %s", err, out)
			}
			if response.Error == nil {
				t.Fatalf("error = nil\noutput: %s", out)
			}
			if response.Error.Code != tt.code {
				t.Fatalf("error code = %q, want %q", response.Error.Code, tt.code)
			}
			if response.Error.Suggestion != "canonical suggestion" {
				t.Fatalf("suggestion = %q, want canonical suggestion", response.Error.Suggestion)
			}
			details, ok := response.Error.Details.(map[string]interface{})
			if !ok || details["source"] != "canonical" {
				t.Fatalf("details = %#v, want canonical source", response.Error.Details)
			}
		})
	}
}

func TestRenderCanonicalQueryLinkJSONHasNoHyperlinks(t *testing.T) {
	prevJSON := jsonOutput
	prevHyperlinksDisabled := hyperlinksDisabled
	prevHyperlinkEnabled := hyperlinkEnabled
	jsonOutput = true
	hyperlinksDisabled = false
	enabled := true
	hyperlinkEnabled = &enabled
	t.Cleanup(func() {
		jsonOutput = prevJSON
		hyperlinksDisabled = prevHyperlinksDisabled
		hyperlinkEnabled = prevHyperlinkEnabled
	})

	result := commandexec.Success(commandpayload.QueryLinkResult{
		QueryKind: "link",
		Items: []commandpayload.LinkItem{{
			SourceID:      "projects/raven",
			FilePath:      "projects/raven.md",
			Line:          12,
			RawTarget:     "../files/spec.pdf",
			Display:       "Spec",
			Scheme:        "file",
			NormalizedKey: "files/spec.pdf",
		}},
	}, nil)
	out := captureStdout(t, func() {
		if err := renderCanonicalQueryResult("link .ext==pdf", nil, result); err != nil {
			t.Fatalf("renderCanonicalQueryResult() error = %v", err)
		}
	})

	if strings.Contains(out, "\x1b]8;;") {
		t.Fatalf("JSON output unexpectedly contains OSC 8: %q", out)
	}
	if !strings.Contains(out, `"normalized_key":"files/spec.pdf"`) {
		t.Fatalf("expected unchanged link payload, got: %q", out)
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
