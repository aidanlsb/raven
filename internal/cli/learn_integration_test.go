//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_LearnListOpenDoneNext(t *testing.T) {
	v := testutil.NewTestVault(t).Build()

	list := v.RunCLI("learn")
	list.MustSucceed(t)

	sections := list.DataList("sections")
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	section0, ok := sections[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected section map, got %T", sections[0])
	}
	if got := section0["id"]; got != "foundations" {
		t.Fatalf("expected section id foundations, got %v", got)
	}

	nextMap, ok := list.Data["next"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected next object in list output")
	}
	if got := nextMap["id"]; got != "objects" {
		t.Fatalf("expected next lesson objects, got %v", got)
	}

	openRefs := v.RunCLI("learn", "open", "refs")
	openRefs.MustSucceed(t)
	prereqs := openRefs.DataList("prereqs")
	if len(prereqs) != 1 {
		t.Fatalf("expected 1 prereq for refs, got %d", len(prereqs))
	}
	prereq0, ok := prereqs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected prereq map, got %T", prereqs[0])
	}
	if got := prereq0["id"]; got != "objects" {
		t.Fatalf("expected prereq objects, got %v", got)
	}
	if got := prereq0["completed"]; got != false {
		t.Fatalf("expected objects prereq incomplete, got %v", got)
	}

	doneObjects := v.RunCLI("learn", "done", "objects", "--date", "2026-02-15")
	doneObjects.MustSucceed(t)
	if got := doneObjects.Data["already_completed"]; got != false {
		t.Fatalf("expected first completion to be new, got %v", got)
	}

	next := v.RunCLI("learn", "next")
	next.MustSucceed(t)
	nextMap, ok = next.Data["next"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected next object")
	}
	if got := nextMap["id"]; got != "refs" {
		t.Fatalf("expected next lesson refs after objects completion, got %v", got)
	}

	doneObjectsAgain := v.RunCLI("learn", "done", "objects", "--date", "2026-02-20")
	doneObjectsAgain.MustSucceed(t)
	if got := doneObjectsAgain.Data["already_completed"]; got != true {
		t.Fatalf("expected idempotent done for objects, got %v", got)
	}

	doneRefs := v.RunCLI("learn", "done", "refs", "--date", "2026-02-16")
	doneRefs.MustSucceed(t)

	nextDone := v.RunCLI("learn", "next")
	nextDone.MustSucceed(t)
	if got := nextDone.Data["all_completed"]; got != true {
		t.Fatalf("expected all_completed=true after completing all lessons, got %v", got)
	}
	if _, exists := nextDone.Data["next"]; exists {
		t.Fatalf("expected no next lesson when all are complete")
	}

	v.AssertFileExists(".raven/learn/progress.yaml")
	v.AssertFileContains(".raven/learn/progress.yaml", "objects: \"2026-02-15\"")
	v.AssertFileContains(".raven/learn/progress.yaml", "refs: \"2026-02-16\"")
}

func TestIntegration_LearnUnknownLesson(t *testing.T) {
	v := testutil.NewTestVault(t).Build()

	res := v.RunCLI("learn", "open", "does-not-exist")
	res.MustFail(t, "INVALID_INPUT")
}

func TestIntegration_LearnValidate(t *testing.T) {
	v := testutil.NewTestVault(t).Build()

	res := v.RunCLI("learn", "validate")
	res.MustSucceed(t)
	if got := res.Data["valid"]; got != true {
		t.Fatalf("expected valid=true, got %v", got)
	}
}
