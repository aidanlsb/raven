package commandimpl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
)

func TestMapExecuteQueryFailureAddsParseSuggestion(t *testing.T) {
	result := mapExecuteQueryFailure(`type:issue .status=='open'`, errors.New("query execution failed"))

	if result.Error == nil {
		t.Fatal("expected query error")
	}
	if !strings.Contains(result.Error.Suggestion, "double quotes") {
		t.Fatalf("expected double-quote suggestion, got %q", result.Error.Suggestion)
	}
}

func TestQueryParseSuggestion(t *testing.T) {
	tests := []struct {
		name        string
		queryString string
		want        string
	}{
		{
			name:        "single quoted value",
			queryString: `type:issue .status=='open'`,
			want:        "double quotes",
		},
		{
			name:        "sql where clause",
			queryString: "type:issue where status = open",
			want:        "does not use 'where'",
		},
		{
			name:        "generic syntax failure",
			queryString: "type:issue (",
			want:        "rvn docs querying query-language",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryParseSuggestion(tt.queryString)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("queryParseSuggestion() = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestSavedQueryArgumentValidationPrecedesConfigLoad(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte("queries: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write invalid raven.yaml: %v", err)
	}

	result := HandleQuerySavedGet(context.Background(), commandexec.Request{VaultPath: vaultPath})
	if result.Error == nil || result.Error.Code != codes.ErrInvalidInput {
		t.Fatalf("error = %#v, want INVALID_INPUT", result.Error)
	}
}
