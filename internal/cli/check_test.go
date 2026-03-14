package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
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
	prevStrict := checkStrict
	prevCreateMissing := checkCreateMissing
	prevByFile := checkByFile
	prevVerbose := checkVerbose
	prevType := checkType
	prevTrait := checkTrait
	prevIssues := checkIssues
	prevExclude := checkExclude
	prevErrorsOnly := checkErrorsOnly
	prevFix := checkFix
	prevConfirm := checkConfirm
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		checkStrict = prevStrict
		checkCreateMissing = prevCreateMissing
		checkByFile = prevByFile
		checkVerbose = prevVerbose
		checkType = prevType
		checkTrait = prevTrait
		checkIssues = prevIssues
		checkExclude = prevExclude
		checkErrorsOnly = prevErrorsOnly
		checkFix = prevFix
		checkConfirm = prevConfirm
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	checkStrict = false
	checkCreateMissing = false
	checkByFile = false
	checkVerbose = false
	checkType = ""
	checkTrait = ""
	checkIssues = ""
	checkExclude = ""
	checkErrorsOnly = false
	checkFix = false
	checkConfirm = false

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
