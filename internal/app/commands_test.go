package app

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func TestValidateRequestNormalizesConfirmArg(t *testing.T) {
	t.Parallel()

	req, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "edit",
		Args: map[string]any{
			"path":    "note/example",
			"old_str": "old",
			"new_str": "new",
			"confirm": true,
		},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if !req.Confirm {
		t.Fatal("Confirm = false, want true from normalized args")
	}
	if req.Preview {
		t.Fatal("Preview = true, want false when confirm is true")
	}
}

func TestValidateRequestDefaultsPreviewForPreviewCommands(t *testing.T) {
	t.Parallel()

	req, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "edit",
		Args: map[string]any{
			"path":    "note/example",
			"old_str": "old",
			"new_str": "new",
		},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if req.Confirm {
		t.Fatal("Confirm = true, want false")
	}
	if !req.Preview {
		t.Fatal("Preview = false, want true for preview-default command")
	}
}

func TestValidateRequestDefaultsPreviewForBulkInputsOnly(t *testing.T) {
	t.Parallel()

	single, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "add",
		Args: map[string]any{
			"text": "hello",
		},
	})
	if !ok {
		t.Fatalf("validateRequest single add failed: %#v", result)
	}
	if single.Preview {
		t.Fatal("single add Preview = true, want false")
	}

	bulk, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "add",
		Args: map[string]any{
			"text":       "hello",
			"stdin":      true,
			"object_ids": []interface{}{"note/one"},
		},
	})
	if !ok {
		t.Fatalf("validateRequest bulk add failed: %#v", result)
	}
	if !bulk.Preview {
		t.Fatal("bulk add Preview = false, want true")
	}
}
