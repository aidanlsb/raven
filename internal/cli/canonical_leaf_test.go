package cli

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func TestCanonicalLeafSuccessSkipsFailureHandlerBeforeHumanRender(t *testing.T) {
	prevJSON := jsonOutput
	t.Cleanup(func() {
		jsonOutput = prevJSON
	})
	jsonOutput = false

	var rendered bool
	cmd := newCanonicalLeafCommand("docs_fetch", canonicalLeafOptions{
		Invoke: func(_ *cobra.Command, _ string, _ string, _ map[string]interface{}) commandexec.Result {
			return commandexec.Result{OK: true}
		},
		HandleError: func(_ commandexec.Result) error {
			return errors.New("failure handler called on successful result")
		},
		RenderHuman: func(_ *cobra.Command, _ commandexec.Result) error {
			rendered = true
			return nil
		},
	})

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !rendered {
		t.Fatal("expected human renderer to run")
	}
}

func TestCanonicalLeafBindsRegistryFlagsToCustomTargets(t *testing.T) {
	var ref, source string
	cmd := newCanonicalLeafCommand("docs_fetch", canonicalLeafOptions{
		FlagBindings: map[string]interface{}{
			"ref":    &ref,
			"source": &source,
		},
	})

	if ref != "main" {
		t.Fatalf("bound ref default = %q, want %q", ref, "main")
	}
	if source == "" {
		t.Fatal("bound source default must come from registry metadata")
	}
	if err := cmd.ParseFlags([]string{"--ref", "release", "--source", "https://example.com/docs"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if ref != "release" {
		t.Fatalf("bound ref = %q, want %q", ref, "release")
	}
	if source != "https://example.com/docs" {
		t.Fatalf("bound source = %q, want example URL", source)
	}
}
