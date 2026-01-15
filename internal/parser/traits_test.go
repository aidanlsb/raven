package parser

import (
	"testing"
)

func TestParseTraitAnnotations(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantCount  int
		wantTraits []string
	}{
		{
			name:       "simple trait",
			line:       "- @task(due=2025-02-01) Send the email",
			wantCount:  1,
			wantTraits: []string{"task"},
		},
		{
			name:       "trait without args",
			line:       "- @highlight This is important",
			wantCount:  1,
			wantTraits: []string{"highlight"},
		},
		{
			name:       "multiple traits",
			line:       "- @task(due=2025-02-01) @highlight Fix this bug",
			wantCount:  2,
			wantTraits: []string{"task", "highlight"},
		},
		{
			name:      "no traits",
			line:      "Just a regular line of text",
			wantCount: 0,
		},
		{
			name:       "trait at start of line",
			line:       "@remind(2025-02-05T09:00) Call someone",
			wantCount:  1,
			wantTraits: []string{"remind"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTraitAnnotations(tt.line, 1)

			if len(got) != tt.wantCount {
				t.Errorf("got %d traits, want %d", len(got), tt.wantCount)
				return
			}

			for i, name := range tt.wantTraits {
				if i < len(got) && got[i].TraitName != name {
					t.Errorf("trait[%d].TraitName = %q, want %q", i, got[i].TraitName, name)
				}
			}
		})
	}
}

func TestParseTrait(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    string
		wantNil bool
	}{
		{
			name: "first trait returned",
			line: "- @task(due=2025-02-01) @highlight Fix this",
			want: "task",
		},
		{
			name:    "no traits",
			line:    "Regular text",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTrait(tt.line, 1)

			if tt.wantNil {
				if got != nil {
					t.Error("expected nil")
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil")
			}

			if got.TraitName != tt.want {
				t.Errorf("TraitName = %q, want %q", got.TraitName, tt.want)
			}
		})
	}
}

func TestParseTraitAnnotations_InlineCode(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantCount int
	}{
		{
			name:      "trait inside inline code ignored",
			line:      "use `@decorator` for Python",
			wantCount: 0,
		},
		{
			name:      "trait outside inline code found",
			line:      "@todo check `some code` here",
			wantCount: 1,
		},
		{
			name:      "mixed: one in code, one outside",
			line:      "@todo `@ignored` task",
			wantCount: 1,
		},
		{
			name:      "double backticks",
			line:      "``@inside`` @outside",
			wantCount: 1,
		},
		{
			name:      "python decorator example",
			line:      "`@property` is a Python decorator",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTraitAnnotations(tt.line, 1)
			if len(got) != tt.wantCount {
				t.Errorf("got %d traits, want %d (line: %q)", len(got), tt.wantCount, tt.line)
			}
		})
	}
}

func TestParseTraitValue_Kinds(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantDate     bool
		wantDatetime bool
		wantRef      bool
		wantString   string
	}{
		{
			name:       "valid date",
			input:      "2025-02-01",
			wantDate:   true,
			wantString: "2025-02-01",
		},
		{
			name:         "valid datetime",
			input:        "2025-02-05T09:00",
			wantDatetime: true,
			wantString:   "2025-02-05T09:00",
		},
		{
			name:       "invalid date-looking string stays string",
			input:      "2025-13-45",
			wantString: "2025-13-45",
		},
		{
			name:       "random T string stays string",
			input:      "invalidTstring",
			wantString: "invalidTstring",
		},
		{
			name:       "ref",
			input:      "[[people/freya]]",
			wantRef:    true,
			wantString: "people/freya",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fv := parseTraitValue(tt.input)

			if got := fv.IsDate(); got != tt.wantDate {
				t.Fatalf("IsDate() = %v, want %v", got, tt.wantDate)
			}
			if got := fv.IsDatetime(); got != tt.wantDatetime {
				t.Fatalf("IsDatetime() = %v, want %v", got, tt.wantDatetime)
			}
			if got := fv.IsRef(); got != tt.wantRef {
				t.Fatalf("IsRef() = %v, want %v", got, tt.wantRef)
			}

			s, ok := fv.AsString()
			if !ok {
				t.Fatalf("AsString() ok=false, want true")
			}
			if s != tt.wantString {
				t.Fatalf("AsString() = %q, want %q", s, tt.wantString)
			}
		})
	}
}

func TestIsRefOnTraitLine(t *testing.T) {
	// This test documents the CONTENT SCOPE RULE:
	// A reference is associated with a trait if and only if they are on the same line.
	tests := []struct {
		name      string
		traitLine int
		refLine   int
		want      bool
	}{
		{
			name:      "same line - associated",
			traitLine: 10,
			refLine:   10,
			want:      true,
		},
		{
			name:      "different lines - not associated",
			traitLine: 10,
			refLine:   11,
			want:      false,
		},
		{
			name:      "ref before trait - not associated",
			traitLine: 10,
			refLine:   9,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRefOnTraitLine(tt.traitLine, tt.refLine)
			if got != tt.want {
				t.Errorf("IsRefOnTraitLine(%d, %d) = %v, want %v",
					tt.traitLine, tt.refLine, got, tt.want)
			}
		})
	}
}
