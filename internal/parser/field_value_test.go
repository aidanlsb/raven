package parser

import (
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestParseFieldValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, value schema.FieldValue)
	}{
		{
			name:  "string",
			input: "hello",
			check: func(t *testing.T, value schema.FieldValue) {
				if got, ok := value.AsString(); !ok || got != "hello" {
					t.Fatalf("value = %v, want string hello", value)
				}
			},
		},
		{
			name:  "quoted string",
			input: `"hello world"`,
			check: func(t *testing.T, value schema.FieldValue) {
				if got, ok := value.AsString(); !ok || got != "hello world" {
					t.Fatalf("value = %v, want string hello world", value)
				}
			},
		},
		{
			name:  "number",
			input: "42",
			check: func(t *testing.T, value schema.FieldValue) {
				if got, ok := value.AsNumber(); !ok || got != 42 {
					t.Fatalf("value = %v, want number 42", value)
				}
			},
		},
		{
			name:  "boolean",
			input: "true",
			check: func(t *testing.T, value schema.FieldValue) {
				if got, ok := value.AsBool(); !ok || !got {
					t.Fatalf("value = %v, want bool true", value)
				}
			},
		},
		{
			name:  "date",
			input: "2026-06-07",
			check: func(t *testing.T, value schema.FieldValue) {
				if got, ok := value.AsString(); !ok || got != "2026-06-07" {
					t.Fatalf("value = %v, want date string 2026-06-07", value)
				}
			},
		},
		{
			name:  "reference",
			input: "[[people/freya]]",
			check: func(t *testing.T, value schema.FieldValue) {
				if got, ok := value.AsRef(); !ok || got != "people/freya" {
					t.Fatalf("value = %v, want ref people/freya", value)
				}
			},
		},
		{
			name:  "array",
			input: `[alpha, "beta", [[people/freya]]]`,
			check: func(t *testing.T, value schema.FieldValue) {
				got, ok := value.AsArray()
				if !ok || len(got) != 3 {
					t.Fatalf("value = %v, want array of 3", value)
				}
				if s, ok := got[0].AsString(); !ok || s != "alpha" {
					t.Fatalf("first array item = %v, want alpha", got[0])
				}
				if s, ok := got[1].AsString(); !ok || s != "beta" {
					t.Fatalf("second array item = %v, want beta", got[1])
				}
				if ref, ok := got[2].AsRef(); !ok || ref != "people/freya" {
					t.Fatalf("third array item = %v, want ref people/freya", got[2])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, ParseFieldValue(tt.input))
		})
	}
}
