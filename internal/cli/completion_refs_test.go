package cli

import (
	"reflect"
	"testing"
)

func TestReferenceCompletionCandidates(t *testing.T) {
	objectIDs := []string{
		"projects/project-one",
		"projects/project-one#notes",
		"people/freya",
	}

	t.Run("short-name completion", func(t *testing.T) {
		got := referenceCompletionCandidates(objectIDs, "proj-o", false)
		want := []string{"project-one", "projects/project-one"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("referenceCompletionCandidates(...) = %v, want %v", got, want)
		}
	})

	t.Run("path completion", func(t *testing.T) {
		got := referenceCompletionCandidates(objectIDs, "projects/proj", false)
		want := []string{"projects/project-one"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("referenceCompletionCandidates(...) = %v, want %v", got, want)
		}
	})

	t.Run("sections excluded without hash", func(t *testing.T) {
		got := referenceCompletionCandidates(objectIDs, "notes", false)
		if len(got) != 0 {
			t.Fatalf("expected no section suggestions without '#', got %v", got)
		}
	})

	t.Run("sections included with hash", func(t *testing.T) {
		got := referenceCompletionCandidates(objectIDs, "projects/project-one#", false)
		want := []string{"projects/project-one#notes"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("referenceCompletionCandidates(...) = %v, want %v", got, want)
		}
	})

	t.Run("dynamic-date keywords", func(t *testing.T) {
		got := referenceCompletionCandidates(objectIDs, "to", true)
		want := []string{"today", "tomorrow"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("referenceCompletionCandidates(...) = %v, want %v", got, want)
		}
	})
}
