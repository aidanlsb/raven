package parser

import (
	"testing"
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
			name:     "valid embedded type",
			line:     "::topic(id=website-discussion, project=[[projects/website]])",
			wantType: "topic",
			wantID:   "website-discussion",
		},
		{
			name:    "type without id",
			line:    "::meeting(time=09:00)",
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
