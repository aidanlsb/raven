package workflow

import (
	"reflect"
	"testing"
)

func TestInterpolateObject_ExactExpressionPreservesType(t *testing.T) {
	inputs := map[string]string{
		"name": "Freya",
	}
	steps := map[string]interface{}{
		"fetch": map[string]interface{}{
			"data": map[string]interface{}{
				"items": []interface{}{"a", "b"},
				"meta": map[string]interface{}{
					"count": 2,
				},
				"active": true,
			},
		},
	}

	args := map[string]interface{}{
		"name":   "{{inputs.name}}",
		"items":  "{{steps.fetch.data.items}}",
		"count":  "{{steps.fetch.data.meta.count}}",
		"active": "{{steps.fetch.data.active}}",
	}

	got, err := InterpolateObject(args, inputs, steps)
	if err != nil {
		t.Fatalf("InterpolateObject returned error: %v", err)
	}

	if got["name"] != "Freya" {
		t.Fatalf("name mismatch: got %#v", got["name"])
	}
	if _, ok := got["items"].([]interface{}); !ok {
		t.Fatalf("items should remain []interface{}, got %T", got["items"])
	}
	if _, ok := got["count"].(int); !ok {
		t.Fatalf("count should remain int, got %T", got["count"])
	}
	if _, ok := got["active"].(bool); !ok {
		t.Fatalf("active should remain bool, got %T", got["active"])
	}
}

func TestInterpolateObject_RecursiveAndStringInterpolation(t *testing.T) {
	inputs := map[string]string{"name": "Loki"}
	steps := map[string]interface{}{
		"a": map[string]interface{}{
			"value": "done",
		},
	}

	args := map[string]interface{}{
		"message": "hello {{inputs.name}}",
		"nested": map[string]interface{}{
			"arr": []interface{}{
				"{{steps.a.value}}",
				"status={{steps.a.value}}",
			},
		},
	}

	got, err := InterpolateObject(args, inputs, steps)
	if err != nil {
		t.Fatalf("InterpolateObject returned error: %v", err)
	}

	if got["message"] != "hello Loki" {
		t.Fatalf("message mismatch: got %#v", got["message"])
	}

	nested, ok := got["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested should be a map, got %T", got["nested"])
	}
	arr, ok := nested["arr"].([]interface{})
	if !ok {
		t.Fatalf("arr should be []interface{}, got %T", nested["arr"])
	}
	want := []interface{}{"done", "status=done"}
	if !reflect.DeepEqual(arr, want) {
		t.Fatalf("arr mismatch: got %#v, want %#v", arr, want)
	}
}

func TestInterpolateObject_UnknownVariableReturnsError(t *testing.T) {
	args := map[string]interface{}{
		"x": "{{steps.missing.value}}",
	}

	_, err := InterpolateObject(args, map[string]string{}, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected unknown variable error")
	}
}
