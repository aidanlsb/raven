package query

import (
	"strings"
	"testing"
)

func TestParseObjectQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantType QueryType
		wantName string
		wantErr  bool
	}{
		{
			name:     "simple type query",
			input:    "type:project",
			wantType: QueryTypeObject,
			wantName: "project",
		},
		{
			name:     "simple trait query",
			input:    "trait:due",
			wantType: QueryTypeTrait,
			wantName: "due",
		},
		{
			name:     "simple asset query",
			input:    "asset",
			wantType: QueryTypeAsset,
		},
		{
			name:     "asset query with predicate",
			input:    "asset .extension==pdf",
			wantType: QueryTypeAsset,
		},
		{
			name:     "simple link query",
			input:    "link",
			wantType: QueryTypeLink,
		},
		{
			name:     "link query with predicate and scope",
			input:    "link .ext==pdf within(type:project)",
			wantType: QueryTypeLink,
		},
		{
			name:    "invalid query type",
			input:   "foo:bar",
			wantErr: true,
		},
		{
			name:    "missing type name",
			input:   "type:",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", q.Type, tt.wantType)
			}
			if q.TypeName != tt.wantName {
				t.Errorf("TypeName = %v, want %v", q.TypeName, tt.wantName)
			}
		})
	}
}
func TestParseRejectsLinkKind(t *testing.T) {
	t.Parallel()

	_, err := Parse("link:pdf")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "link query root is bare 'link'") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestParseRejectsAssetKind(t *testing.T) {
	t.Parallel()

	_, err := Parse("asset:pdf")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "asset query root is bare 'asset'") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestParseRejectsLegacyObjectRoot(t *testing.T) {
	t.Parallel()

	_, err := Parse("object:project")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "legacy 'object:' queries are no longer supported; use 'type:'") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestParseRejectsUnterminatedLiterals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unterminated string literal",
			input: `type:project .title=="Unclosed`,
		},
		{
			name:  "unterminated raw string literal",
			input: `type:project content(r"Unclosed)`,
		},
		{
			name:  "unterminated regex literal",
			input: `type:project matches(.title, /unclosed)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.input); err == nil {
				t.Fatal("expected parse error, got nil")
			}
		})
	}
}
func TestParseRejectsUnexpectedTrailingTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unmatched closing parenthesis after type",
			input: `type:project)`,
		},
		{
			name:  "unmatched closing parenthesis after predicate",
			input: `type:project .status==active)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.input); err == nil {
				t.Fatal("expected parse error, got nil")
			}
		})
	}
}
func TestParseReportsShellPipeGuidance(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:experiment_review | sort -field:created_at | head 1`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'|' (pipe) is not a shell pipe") {
		t.Fatalf("expected pipe guidance, got %q", msg)
	}
	if !strings.Contains(msg, "--pipe | jq") {
		t.Fatalf("expected shell pipeline example, got %q", msg)
	}
}
func TestParseReportsShellPipeGuidanceAfterPredicate(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:experiment_review .status==open | jq`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'|' (pipe) is not a shell pipe") {
		t.Fatalf("expected pipe guidance, got %q", msg)
	}
}
func TestParseReportsShellPipeGuidanceInArrayQuantifier(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:book any(.tags, _ == "raven" | jq)`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'|' (pipe) is not a shell pipe") {
		t.Fatalf("expected pipe guidance, got %q", msg)
	}
}
func TestParseUnexpectedTokenUsesReadableTokenName(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:project =active`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unexpected token '='") {
		t.Fatalf("expected readable token name, got %q", msg)
	}
	if strings.Contains(msg, "unexpected token 22") {
		t.Fatalf("expected symbolic token name instead of enum number, got %q", msg)
	}
}
