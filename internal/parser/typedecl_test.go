package parser

import (
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestParseTypeDeclaration(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantType string
		wantID   string
		wantNil  bool
	}{
		{
			name:     "simple type decl",
			line:     "::meeting(id=standup, time=09:00)",
			wantType: "meeting",
			wantID:   "standup",
		},
		{
			name:    "not a type decl",
			line:    "Some regular text",
			wantNil: true,
		},
		{
			name:     "type with refs",
			line:     "::meeting(id=standup, attendees=[[[people/freya]], [[people/thor]]])",
			wantType: "meeting",
			wantID:   "standup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTypeDeclaration(tt.line, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if got != nil {
					t.Error("expected nil result")
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil result")
			}

			if got.TypeName != tt.wantType {
				t.Errorf("TypeName = %q, want %q", got.TypeName, tt.wantType)
			}

			if got.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestParseEmbeddedType(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantType string
		wantID   string
		wantNil  bool
	}{
		{
			name:     "valid embedded type with explicit id",
			line:     "::topic(id=website-discussion, project=[[projects/website]])",
			wantType: "topic",
			wantID:   "website-discussion",
		},
		{
			name:     "type without id - id derived from heading later",
			line:     "::meeting(time=09:00)",
			wantType: "meeting",
			wantID:   "", // ID will be derived from heading by document parser
		},
		{
			name:    "not a type declaration",
			line:    "Some regular text",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEmbeddedType(tt.line, 1)

			if tt.wantNil {
				if got != nil {
					t.Error("expected nil result")
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil result")
			}

			if got.TypeName != tt.wantType {
				t.Errorf("TypeName = %q, want %q", got.TypeName, tt.wantType)
			}

			if got.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestSerializeTypeDeclaration(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		fields   map[string]schema.FieldValue
		want     string
	}{
		{
			name:     "empty fields",
			typeName: "task",
			fields:   map[string]schema.FieldValue{},
			want:     "::task()",
		},
		{
			name:     "single string field",
			typeName: "task",
			fields: map[string]schema.FieldValue{
				"status": schema.String("active"),
			},
			want: "::task(status=active)",
		},
		{
			name:     "multiple fields sorted",
			typeName: "meeting",
			fields: map[string]schema.FieldValue{
				"time":   schema.String("09:00"),
				"id":     schema.String("standup"),
				"agenda": schema.String("review"),
			},
			want: "::meeting(agenda=review, id=standup, time=09:00)",
		},
		{
			name:     "boolean field",
			typeName: "task",
			fields: map[string]schema.FieldValue{
				"done": schema.Bool(true),
			},
			want: "::task(done=true)",
		},
		{
			name:     "number field",
			typeName: "task",
			fields: map[string]schema.FieldValue{
				"priority": schema.Number(5),
			},
			want: "::task(priority=5)",
		},
		{
			name:     "reference field",
			typeName: "task",
			fields: map[string]schema.FieldValue{
				"assignee": schema.Ref("people/freya"),
			},
			want: "::task(assignee=[[people/freya]])",
		},
		{
			name:     "array field",
			typeName: "meeting",
			fields: map[string]schema.FieldValue{
				"attendees": schema.Array([]schema.FieldValue{
					schema.Ref("people/freya"),
					schema.Ref("people/thor"),
				}),
			},
			want: "::meeting(attendees=[[[people/freya]], [[people/thor]]])",
		},
		{
			name:     "string with special chars needs quoting",
			typeName: "task",
			fields: map[string]schema.FieldValue{
				"note": schema.String("hello, world"),
			},
			want: `::task(note="hello, world")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SerializeTypeDeclaration(tt.typeName, tt.fields)
			if got != tt.want {
				t.Errorf("SerializeTypeDeclaration() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	// Test that parsing a declaration and serializing it back produces equivalent output
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple fields",
			input: "::meeting(id=standup, time=09:00)",
		},
		{
			name:  "with reference",
			input: "::task(assignee=[[people/freya]], status=active)",
		},
		{
			name:  "with array of refs",
			input: "::meeting(attendees=[[[people/freya]], [[people/thor]]], id=sync)",
		},
		{
			name:  "boolean value",
			input: "::task(done=true, id=cleanup)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the input
			decl, err := ParseTypeDeclaration(tt.input, 1)
			if err != nil {
				t.Fatalf("ParseTypeDeclaration() error: %v", err)
			}
			if decl == nil {
				t.Fatal("ParseTypeDeclaration() returned nil")
			}

			// Serialize it back
			serialized := SerializeTypeDeclaration(decl.TypeName, decl.Fields)

			// Parse the serialized version
			decl2, err := ParseTypeDeclaration(serialized, 1)
			if err != nil {
				t.Fatalf("ParseTypeDeclaration() on serialized error: %v", err)
			}
			if decl2 == nil {
				t.Fatal("ParseTypeDeclaration() on serialized returned nil")
			}

			// Compare the type names and IDs
			if decl.TypeName != decl2.TypeName {
				t.Errorf("TypeName mismatch: original=%q, round-trip=%q", decl.TypeName, decl2.TypeName)
			}
			if decl.ID != decl2.ID {
				t.Errorf("ID mismatch: original=%q, round-trip=%q", decl.ID, decl2.ID)
			}

			// Compare field counts
			if len(decl.Fields) != len(decl2.Fields) {
				t.Errorf("Field count mismatch: original=%d, round-trip=%d", len(decl.Fields), len(decl2.Fields))
			}
		})
	}
}
