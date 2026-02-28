package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAgentOutputEnvelope_SourcePrecedence(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "agent-output.json")
	if err := os.WriteFile(outputFile, []byte(`{"outputs":{"markdown":"from-file"}}`), 0o600); err != nil {
		t.Fatalf("write temp output file: %v", err)
	}

	tests := []struct {
		name         string
		file         string
		jsonFlag     string
		inlineString string
		wantMarkdown string
	}{
		{
			name:         "agent-output-json wins when provided",
			file:         outputFile,
			jsonFlag:     `{"outputs":{"markdown":"from-json"}}`,
			inlineString: `{"outputs":{"markdown":"from-inline"}}`,
			wantMarkdown: "from-json",
		},
		{
			name:         "agent-output string wins over file",
			file:         outputFile,
			inlineString: `{"outputs":{"markdown":"from-inline"}}`,
			wantMarkdown: "from-inline",
		},
		{
			name:         "agent-output-file is used as fallback",
			file:         outputFile,
			wantMarkdown: "from-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env, err := parseAgentOutputEnvelope(tt.file, tt.jsonFlag, tt.inlineString)
			if err != nil {
				t.Fatalf("parseAgentOutputEnvelope() error = %v", err)
			}
			got, ok := env.Outputs["markdown"].(string)
			if !ok {
				t.Fatalf("expected markdown output to be string, got %T", env.Outputs["markdown"])
			}
			if got != tt.wantMarkdown {
				t.Fatalf("markdown output = %q, want %q", got, tt.wantMarkdown)
			}
		})
	}
}

func TestParseAgentOutputEnvelope_RequiresInput(t *testing.T) {
	t.Parallel()

	_, err := parseAgentOutputEnvelope("", "", "")
	if err == nil {
		t.Fatal("expected error when no agent output source is provided")
	}
	want := "provide --agent-output-json, --agent-output, or --agent-output-file"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want to contain %q", err.Error(), want)
	}
}
