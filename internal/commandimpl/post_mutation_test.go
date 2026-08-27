package commandimpl

import (
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/codes"
)

func TestMissingRefWarningInferredType(t *testing.T) {
	t.Parallel()
	warning := missingRefWarning(&check.MissingRef{TargetPath: "people/ghost", InferredType: "person"})
	if warning.Code != codes.WarnRefTargetMissing || warning.SuggestedType != "person" {
		t.Fatalf("warning = %#v", warning)
	}
	if warning.CreateInvoke == nil || warning.CreateInvoke.Command != "new" {
		t.Fatalf("create invoke = %#v, want new", warning.CreateInvoke)
	}
	if warning.CreateInvoke.Args["path"] != "people/ghost" || warning.CreateInvoke.Args["title"] != "ghost" {
		t.Fatalf("create args = %#v", warning.CreateInvoke.Args)
	}
}

func TestMissingRefWarningUnknownTypeFallsBackToCheck(t *testing.T) {
	t.Parallel()
	warning := missingRefWarning(&check.MissingRef{TargetPath: "misc/thing"})
	if warning.Code != codes.WarnRefTargetMissing || warning.SuggestedType != "" {
		t.Fatalf("warning = %#v", warning)
	}
	if warning.CreateInvoke == nil || warning.CreateInvoke.Command != "check create-missing" {
		t.Fatalf("create invoke = %#v, want check create-missing", warning.CreateInvoke)
	}
}
