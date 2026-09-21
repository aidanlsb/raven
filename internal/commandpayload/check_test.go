package commandpayload

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
)

func TestBuildJSON_MissingReferenceSummarySuggestsCreateMissing(t *testing.T) {
	t.Parallel()

	result := &checksvc.RunResult{
		Issues: []check.Issue{
			{
				Type:       check.IssueMissingReference,
				Level:      check.LevelError,
				FilePath:   "project/roadmap.md",
				Line:       4,
				Message:    "Reference [[meeting/all-hands]] not found",
				Value:      "meeting/all-hands",
				FixCommand: `rvn new meeting "meeting/all-hands"`,
				FixHint:    "Create the missing meeting",
			},
		},
		ErrorCount: 1,
	}

	jsonResult := BuildJSON("/vault", result)
	if len(jsonResult.Summary) != 1 {
		t.Fatalf("summary = %#v, want one item", jsonResult.Summary)
	}
	summary := jsonResult.Summary[0]
	if summary.IssueType != string(check.IssueMissingReference) {
		t.Fatalf("issue_type = %q, want missing_reference", summary.IssueType)
	}
	if summary.FixCommand != "rvn check create-missing --json" {
		t.Fatalf("fix_command = %q, want create-missing preview command", summary.FixCommand)
	}
	if !strings.Contains(summary.FixHint, "--confirm") {
		t.Fatalf("fix_hint = %q, want confirm guidance", summary.FixHint)
	}
}

func TestBuildJSON_FieldNamesStayStable(t *testing.T) {
	t.Parallel()

	result := &checksvc.RunResult{
		Scope:      checksvc.Scope{Type: "file", Value: "notes/a.md"},
		FileCount:  1,
		ErrorCount: 1,
		Issues: []check.Issue{{
			Type:     check.IssueParseError,
			Level:    check.LevelError,
			FilePath: "notes/a.md",
			Line:     1,
			Message:  "bad yaml",
		}},
		SchemaIssues: []check.SchemaIssue{{
			Type:    check.IssueUnusedType,
			Level:   check.LevelWarning,
			Message: "unused",
			Value:   "person",
		}},
	}

	payload := BuildJSON("/vault", result)
	keys := payloadJSONKeys(t, payload)
	want := []string{"error_count", "file_count", "issues", "scope", "summary", "vault_path", "warning_count"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("JSON keys = %v, want %v", keys, want)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	issues, _ := decoded["issues"].([]any)
	if len(issues) != 2 {
		t.Fatalf("issues len = %d, want 2 (file + schema)", len(issues))
	}
	schemaIssue, _ := issues[1].(map[string]any)
	if schemaIssue["file_path"] != "schema.yaml" {
		t.Fatalf("schema issue file_path = %#v, want schema.yaml", schemaIssue["file_path"])
	}
}

func TestCheckResultScope_ValidatePayload(t *testing.T) {
	t.Parallel()

	scope, ok := CheckResultScope(CheckResultJSON{})
	if !ok || scope.Type != "full" {
		t.Fatalf("empty validate payload scope = %#v ok=%v, want full", scope, ok)
	}

	scope, ok = CheckResultScope(CheckResultJSON{Scope: &CheckScopeJSON{Type: "directory", Value: "notes"}})
	if !ok || scope.Type != "directory" || scope.Value != "notes" {
		t.Fatalf("directory scope = %#v ok=%v", scope, ok)
	}
}

func TestBuildJSON_EmptyFilteredResult(t *testing.T) {
	t.Parallel()

	payload := BuildJSON("/vault", &checksvc.RunResult{})
	if payload.ErrorCount != 0 || payload.WarnCount != 0 {
		t.Fatalf("counts = %+v, want zeros", payload)
	}
	if len(payload.Issues) != 0 {
		t.Fatalf("issues = %#v, want none", payload.Issues)
	}
	if payload.Issues == nil {
		t.Fatal("issues must be an empty slice, not null")
	}
}
