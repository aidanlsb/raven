package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
)

func TestSetCommandArgsAllowPositionalFields(t *testing.T) {
	if setCmd.Args == nil {
		t.Fatal("set command has no args validator")
	}
	for _, args := range [][]string{
		nil,
		{"project/perch"},
		{"project/perch", "name=Perch"},
		{"project/perch", "name=Perch", "status=active"},
	} {
		if err := setCmd.Args(setCmd, args); err != nil {
			t.Fatalf("set args %q: %v", args, err)
		}
	}
}

func TestUnsetCommandArgsAllowMultipleFields(t *testing.T) {
	if unsetCmd.Args == nil {
		t.Fatal("unset command has no args validator")
	}
	if err := unsetCmd.Args(unsetCmd, []string{"docs/cleanup", "date", "link"}); err != nil {
		t.Fatalf("unset args: %v", err)
	}
}

func TestEditCommandArgsAllowThreePositionals(t *testing.T) {
	if editCmd.Args == nil {
		t.Fatal("edit command has no args validator")
	}
	if err := editCmd.Args(editCmd, []string{"project/x", "old", "new"}); err != nil {
		t.Fatalf("edit args: %v", err)
	}
	// A fourth positional fails. Cobra removes a "--" terminator before this
	// validator runs, so flags placed after "--" (not "--" itself) are what
	// show up as the extra argument.
	err := editCmd.Args(editCmd, []string{"project/x", "old", "new", "--json"})
	if err == nil || !strings.Contains(err.Error(), "received 4") {
		t.Fatalf("edit fourth arg error = %v, want received 4", err)
	}
}

func TestBuildCanonicalArgsSetPositionalFields(t *testing.T) {
	prevJSON := jsonOutput
	t.Cleanup(func() { jsonOutput = prevJSON })
	jsonOutput = false

	meta, ok := commands.EffectiveMeta("set")
	if !ok {
		t.Fatal("set metadata missing")
	}

	t.Run("positional pairs", func(t *testing.T) {
		cmd, meta := newFlagCommand(t, meta)
		argsMap, err := buildCanonicalArgsForMeta(meta, cmd, []string{"project/perch", "name=Perch", "status=active"})
		if err != nil {
			t.Fatalf("build args: %v", err)
		}
		if argsMap["reference"] != "project/perch" {
			t.Fatalf("reference = %#v", argsMap["reference"])
		}
		fields := argsMap["field"].(map[string]interface{})
		if fields["name"] != "Perch" || fields["status"] != "active" {
			t.Fatalf("fields = %#v", fields)
		}
	})

	t.Run("flag and positional merge", func(t *testing.T) {
		cmd, meta := newFlagCommand(t, meta)
		if err := cmd.ParseFlags([]string{"--field", "status=active"}); err != nil {
			t.Fatalf("parse flags: %v", err)
		}
		argsMap, err := buildCanonicalArgsForMeta(meta, cmd, []string{"project/perch", "name=Perch"})
		if err != nil {
			t.Fatalf("build args: %v", err)
		}
		fields := argsMap["field"].(map[string]interface{})
		if fields["name"] != "Perch" || fields["status"] != "active" {
			t.Fatalf("fields = %#v", fields)
		}
	})

	t.Run("duplicate field", func(t *testing.T) {
		cmd, meta := newFlagCommand(t, meta)
		if err := cmd.ParseFlags([]string{"--field", "name=Flag"}); err != nil {
			t.Fatalf("parse flags: %v", err)
		}
		_, err := buildCanonicalArgsForMeta(meta, cmd, []string{"project/perch", "name=Pos"})
		if err == nil || !strings.Contains(err.Error(), `field "name"`) {
			t.Fatalf("error = %v, want duplicate field", err)
		}
	})

	t.Run("invalid positional", func(t *testing.T) {
		cmd, meta := newFlagCommand(t, meta)
		_, err := buildCanonicalArgsForMeta(meta, cmd, []string{"project/perch", "not-a-pair"})
		if err == nil || !strings.Contains(err.Error(), "invalid field format") {
			t.Fatalf("error = %v, want invalid field format", err)
		}
	})

	t.Run("fields-json only", func(t *testing.T) {
		cmd, meta := newFlagCommand(t, meta)
		if err := cmd.ParseFlags([]string{"--fields-json", `{"name":"Perch"}`}); err != nil {
			t.Fatalf("parse flags: %v", err)
		}
		argsMap, err := buildCanonicalArgsForMeta(meta, cmd, []string{"project/perch"})
		if err != nil {
			t.Fatalf("build args: %v", err)
		}
		if _, ok := argsMap["field"]; ok {
			t.Fatalf("did not expect field map, got %#v", argsMap["field"])
		}
		raw, ok := argsMap["fields-json"].(map[string]interface{})
		if !ok || raw["name"] != "Perch" {
			t.Fatalf("fields-json = %#v", argsMap["fields-json"])
		}
	})
}

func newFlagCommand(t *testing.T, meta commands.Meta) (*cobra.Command, commands.Meta) {
	t.Helper()
	cmd := &cobra.Command{Use: meta.Name}
	bindMetaFlags(cmd, meta.Flags)
	return cmd, meta
}
