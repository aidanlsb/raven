package cli

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/fieldvalue"
)

func TestSchemaRenameDefaultPathPreviewUsesTypedPayload(t *testing.T) {
	t.Parallel()

	available := true
	oldPath := "events/"
	newPath := "meetings/"
	filesToMove := 3
	gotOld, gotNew, gotFiles, gotAvailable := schemaRenameDefaultPathPreview(commandpayload.SchemaRenameTypePreviewResult{
		DefaultPathRenameAvailable: &available,
		DefaultPathOld:             &oldPath,
		DefaultPathNew:             &newPath,
		FilesToMove:                &filesToMove,
	})
	if !gotAvailable || gotOld != oldPath || gotNew != newPath || gotFiles != filesToMove {
		t.Fatalf(
			"schemaRenameDefaultPathPreview() = (%q, %q, %d, %t), want (%q, %q, %d, true)",
			gotOld,
			gotNew,
			gotFiles,
			gotAvailable,
			oldPath,
			newPath,
			filesToMove,
		)
	}
}

func TestRenderSetResultFormatsPreviousFieldValues(t *testing.T) {
	result := commandexec.Success(commandpayload.SetResult{
		File:          "projects/raven.md",
		ObjectID:      "projects/raven",
		Type:          "project",
		UpdatedFields: map[string]string{"owner": "[[people/loki]]"},
		PreviousFields: map[string]fieldvalue.FieldValue{
			"owner": fieldvalue.Ref("people/freya"),
		},
	}, nil)

	output := captureStdout(t, func() {
		if err := renderCanonicalSetSingleResult(result); err != nil {
			t.Fatalf("renderCanonicalSetSingleResult() error = %v", err)
		}
	})
	for _, want := range []string{"[[people/freya]]", "[[people/loki]]"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "{people/freya}") {
		t.Fatalf("output leaked FieldValue representation: %q", output)
	}
}
