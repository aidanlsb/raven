package commandimpl

import (
	"errors"
	"strings"
	"testing"
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
