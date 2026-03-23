package bulkops

import (
	"testing"

	"github.com/aidanlsb/raven/internal/model"
)

func TestParseRawApply(t *testing.T) {
	t.Run("parses command and args", func(t *testing.T) {
		got, err := ParseRawApply([]string{"set", "status=done"})
		if err != nil {
			t.Fatalf("ParseRawApply returned error: %v", err)
		}
		if got.Command != "set" {
			t.Fatalf("Command = %q, want %q", got.Command, "set")
		}
		if len(got.Args) != 1 || got.Args[0] != "status=done" {
			t.Fatalf("Args = %#v, want [status=done]", got.Args)
		}
	})

	t.Run("rejects empty apply", func(t *testing.T) {
		_, err := ParseRawApply(nil)
		if err == nil {
			t.Fatal("ParseRawApply(nil) returned nil error")
		}
		bulkErr, ok := AsError(err)
		if !ok {
			t.Fatalf("error = %T, want *Error", err)
		}
		if bulkErr.Code != CodeInvalidInput {
			t.Fatalf("Code = %q, want %q", bulkErr.Code, CodeInvalidInput)
		}
	})
}

func TestPlanObjectApply(t *testing.T) {
	t.Run("builds set plan and dedupes ids", func(t *testing.T) {
		raw := &RawApplyCommand{Command: "set", Args: []string{"status=done"}}
		got, err := PlanObjectApply(raw, []string{"people/freya", "people/freya", "people/thor#tasks"})
		if err != nil {
			t.Fatalf("PlanObjectApply returned error: %v", err)
		}
		if got.Command != ObjectApplySet {
			t.Fatalf("Command = %q, want %q", got.Command, ObjectApplySet)
		}
		if len(got.IDs) != 2 {
			t.Fatalf("IDs len = %d, want 2", len(got.IDs))
		}
		if got.SetUpdates["status"] != "done" {
			t.Fatalf("SetUpdates[status] = %q, want done", got.SetUpdates["status"])
		}
		if len(got.FileIDs) != 1 || got.FileIDs[0] != "people/freya" {
			t.Fatalf("FileIDs = %#v, want [people/freya]", got.FileIDs)
		}
		if len(got.EmbeddedIDs) != 1 || got.EmbeddedIDs[0] != "people/thor#tasks" {
			t.Fatalf("EmbeddedIDs = %#v, want [people/thor#tasks]", got.EmbeddedIDs)
		}
	})

	t.Run("rejects add without text", func(t *testing.T) {
		_, err := PlanObjectApply(&RawApplyCommand{Command: "add"}, []string{"people/freya"})
		if err == nil {
			t.Fatal("PlanObjectApply returned nil error")
		}
		bulkErr, _ := AsError(err)
		if bulkErr.Code != CodeMissingArgument {
			t.Fatalf("Code = %q, want %q", bulkErr.Code, CodeMissingArgument)
		}
	})

	t.Run("rejects move without directory destination", func(t *testing.T) {
		_, err := PlanObjectApply(&RawApplyCommand{Command: "move", Args: []string{"archive"}}, []string{"people/freya"})
		if err == nil {
			t.Fatal("PlanObjectApply returned nil error")
		}
		bulkErr, _ := AsError(err)
		if bulkErr.Code != CodeInvalidInput {
			t.Fatalf("Code = %q, want %q", bulkErr.Code, CodeInvalidInput)
		}
	})
}

func TestPlanTraitApply(t *testing.T) {
	traits := []model.Trait{{ID: "daily/2026-01-01.md:trait:1"}}

	t.Run("builds update plan", func(t *testing.T) {
		got, err := PlanTraitApply(&RawApplyCommand{Command: "update", Args: []string{"done"}}, traits)
		if err != nil {
			t.Fatalf("PlanTraitApply returned error: %v", err)
		}
		if got.NewValue != "done" {
			t.Fatalf("NewValue = %q, want done", got.NewValue)
		}
		if len(got.Items) != 1 {
			t.Fatalf("Items len = %d, want 1", len(got.Items))
		}
	})

	t.Run("rejects unsupported command", func(t *testing.T) {
		_, err := PlanTraitApply(&RawApplyCommand{Command: "delete"}, traits)
		if err == nil {
			t.Fatal("PlanTraitApply returned nil error")
		}
		bulkErr, _ := AsError(err)
		if bulkErr.Code != CodeInvalidInput {
			t.Fatalf("Code = %q, want %q", bulkErr.Code, CodeInvalidInput)
		}
	})
}
