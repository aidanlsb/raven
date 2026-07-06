package fieldmutation

import (
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestCoerceFieldValueForDefinitionAtDateInputs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	def := &schema.FieldDefinition{Type: schema.FieldTypeDate}

	got := coerceFieldValueForDefinitionAt(schema.String("today"), def, now)
	if raw, _ := got.AsString(); raw != "2026-04-05" || !got.IsDate() {
		t.Fatalf("coerced date = %q IsDate=%v, want 2026-04-05 date value", raw, got.IsDate())
	}
}

func TestCoerceFieldValueForDefinitionAtDateArrayInputs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	def := &schema.FieldDefinition{Type: schema.FieldTypeDateArray}

	got := coerceFieldValueForDefinitionAt(schema.Array([]schema.FieldValue{
		schema.String("yesterday"),
		schema.String("2026-04-05"),
		schema.String("not-a-date"),
	}), def, now)

	arr, ok := got.AsArray()
	if !ok || len(arr) != 3 {
		t.Fatalf("coerced array = %#v, ok=%v", got.Raw(), ok)
	}
	if raw, _ := arr[0].AsString(); raw != "2026-04-04" || !arr[0].IsDate() {
		t.Fatalf("first item = %q IsDate=%v, want 2026-04-04 date value", raw, arr[0].IsDate())
	}
	if raw, _ := arr[1].AsString(); raw != "2026-04-05" || !arr[1].IsDate() {
		t.Fatalf("second item = %q IsDate=%v, want 2026-04-05 date value", raw, arr[1].IsDate())
	}
	if raw, _ := arr[2].AsString(); raw != "not-a-date" || arr[2].IsDate() {
		t.Fatalf("third item = %q IsDate=%v, want unchanged string", raw, arr[2].IsDate())
	}
}

func TestCoerceFieldValueForDefinitionAtDateTargetRef(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	def := &schema.FieldDefinition{Type: schema.FieldTypeRef, Target: "date"}

	got := coerceFieldValueForDefinitionAt(schema.String("tomorrow"), def, now)
	if raw, _ := got.AsString(); raw != "2026-04-06" || got.IsDate() || got.IsRef() {
		t.Fatalf("coerced date ref = %q IsDate=%v IsRef=%v, want plain 2026-04-06 string", raw, got.IsDate(), got.IsRef())
	}
}

func TestCoerceFieldValueForDefinitionAtDateTargetRefArray(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	def := &schema.FieldDefinition{Type: schema.FieldTypeRefArray, Target: "date"}

	got := coerceFieldValueForDefinitionAt(schema.Array([]schema.FieldValue{
		schema.String("today"),
		schema.String("daily/2026-04-06"),
	}), def, now)

	arr, ok := got.AsArray()
	if !ok || len(arr) != 2 {
		t.Fatalf("coerced array = %#v, ok=%v", got.Raw(), ok)
	}
	if raw, _ := arr[0].AsString(); raw != "2026-04-05" || arr[0].IsDate() {
		t.Fatalf("first item = %q IsDate=%v, want plain 2026-04-05 string", raw, arr[0].IsDate())
	}
	if raw, _ := arr[1].AsString(); raw != "daily/2026-04-06" {
		t.Fatalf("second item = %q, want unchanged daily ID", raw)
	}
}
