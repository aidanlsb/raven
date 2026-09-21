package sectionsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestResolveTargetKinds(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", lifecycleOutline).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	tests := []struct {
		name    string
		kind    resolveKind
		ref     string
		wantID  string
		wantErr codes.ErrorCode
		wantMsg string
	}{
		{name: "file ok", kind: resolveFile, ref: "projects/site", wantID: "projects/site"},
		{name: "file rejects section", kind: resolveFile, ref: "projects/site#alpha", wantErr: codes.ErrInvalidInput, wantMsg: "must be a file"},
		{name: "file empty", kind: resolveFile, ref: "  ", wantErr: codes.ErrInvalidInput, wantMsg: "file reference is required"},
		{name: "section ok", kind: resolveSectionRef, ref: "projects/site#beta", wantID: "projects/site#beta"},
		{name: "section rejects file", kind: resolveSectionRef, ref: "projects/site", wantErr: codes.ErrInvalidInput, wantMsg: "section reference required"},
		{name: "section empty", kind: resolveSectionRef, ref: "", wantErr: codes.ErrInvalidInput, wantMsg: "section reference is required"},
		{name: "section missing", kind: resolveSectionRef, ref: "projects/site#missing", wantErr: codes.ErrRefNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolveTarget(rt, tt.ref, tt.kind)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				assertServiceCode(t, err, tt.wantErr)
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTarget() error = %v", err)
			}
			if resolved.ObjectID != tt.wantID {
				t.Fatalf("ObjectID = %q, want %q", resolved.ObjectID, tt.wantID)
			}
		})
	}
}

func TestLoadResolvedSectionAndFile(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", lifecycleOutline).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	state, err := loadResolvedFile(rt, "projects/site")
	if err != nil {
		t.Fatalf("loadResolvedFile() error = %v", err)
	}
	if state.fileID != "projects/site" || state.fileRelative != "projects/site.md" {
		t.Fatalf("file state = %+v", state)
	}
	if _, ok := state.sectionsByID["projects/site#alpha"]; !ok {
		t.Fatalf("sectionsByID missing alpha: %#v", state.sectionsByID)
	}

	state, section, err := loadResolvedSection(rt, "projects/site#alpha-child")
	if err != nil {
		t.Fatalf("loadResolvedSection() error = %v", err)
	}
	if section.ID != "projects/site#alpha-child" || section.Level != 3 {
		t.Fatalf("section = %#v", section)
	}
	if state.fileID != "projects/site" {
		t.Fatalf("section fileID = %q", state.fileID)
	}

	_, err = loadResolvedFile(rt, "projects/site#alpha")
	assertServiceCode(t, err, codes.ErrInvalidInput)
	_, _, err = loadResolvedSection(rt, "projects/site")
	assertServiceCode(t, err, codes.ErrInvalidInput)
}
