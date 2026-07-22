package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/commandexec"
)

type fakeCheckInteraction struct {
	inputs []string
	output strings.Builder
}

func (f *fakeCheckInteraction) Printf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(&f.output, format, args...)
}

func (f *fakeCheckInteraction) Println(args ...interface{}) {
	_, _ = fmt.Fprintln(&f.output, args...)
}

func (f *fakeCheckInteraction) ReadLine() (string, error) {
	if len(f.inputs) == 0 {
		return "", io.EOF
	}
	line := f.inputs[0]
	f.inputs = f.inputs[1:]
	return line, nil
}

func TestCheckJSONUsesStandardEnvelope(t *testing.T) {
	vaultPath := t.TempDir()

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true

	out := captureStdout(t, func() {
		if err := checkCmd.RunE(checkCmd, nil); err != nil {
			t.Fatalf("checkCmd.RunE: %v", err)
		}
	})

	var envelope struct {
		OK   bool            `json:"ok"`
		Data CheckResultJSON `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("expected standard JSON envelope, got parse error: %v; out=%s", err, out)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got false; out=%s", out)
	}
	if envelope.Data.VaultPath != vaultPath {
		t.Fatalf("vault_path = %q, want %q", envelope.Data.VaultPath, vaultPath)
	}
}

func TestPromptTraitTypeAcceptsNumber(t *testing.T) {
	trait := &check.UndefinedTrait{
		TraitName: "estimate",
		HasValue:  true,
	}
	interaction := &fakeCheckInteraction{inputs: []string{"number\n"}}

	selected := promptTraitType(trait, interaction)
	if selected != "number" {
		t.Fatalf("selected type = %q, want %q", selected, "number")
	}

	if strings.Contains(interaction.output.String(), "Invalid type") {
		t.Fatalf("prompt unexpectedly rejected number type: %s", interaction.output.String())
	}
}

func TestCollectTraitDecisionsKeepsInteractiveOptions(t *testing.T) {
	traits := []*check.UndefinedTrait{{
		TraitName:  "priority",
		HasValue:   true,
		UsageCount: 3,
	}}
	interaction := &fakeCheckInteraction{inputs: []string{
		"y\n",
		"enum\n",
		"low, medium, high\n",
		"medium\n",
	}}

	got := collectTraitDecisions(traits, interaction)
	want := checksvc.TraitResolution{
		TraitName:    "priority",
		TraitType:    "enum",
		EnumValues:   []string{"low", "medium", "high"},
		DefaultValue: "medium",
	}
	if len(got) != 1 {
		t.Fatalf("resolution count = %d, want 1", len(got))
	}
	if got[0].TraitName != want.TraitName ||
		got[0].TraitType != want.TraitType ||
		got[0].DefaultValue != want.DefaultValue ||
		strings.Join(got[0].EnumValues, ",") != strings.Join(want.EnumValues, ",") {
		t.Fatalf("resolution = %#v, want %#v", got[0], want)
	}
}

func TestCheckShouldExitPolicy(t *testing.T) {
	tests := []struct {
		name     string
		errors   int
		warnings int
		strict   bool
		want     bool
	}{
		{name: "clean", want: false},
		{name: "warning non-strict", warnings: 1, want: false},
		{name: "warning strict", warnings: 1, strict: true, want: true},
		{name: "error", errors: 1, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := commandexec.Success(map[string]interface{}{
				"error_count":   tt.errors,
				"warning_count": tt.warnings,
			}, nil)
			if got := checkShouldExit(result, tt.strict); got != tt.want {
				t.Fatalf("checkShouldExit() = %v, want %v", got, tt.want)
			}
		})
	}
}
