package querysvc

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestExecuteErrorAddsParseSuggestion(t *testing.T) {
	err := executeError(`type:issue .status=='open'`, errors.New("query execution failed"))
	svcErr, ok := svcerr.AsError(err)
	if !ok || !strings.Contains(svcErr.Suggestion, "double quotes") {
		t.Fatalf("error = %#v, want double-quote suggestion", err)
	}
}

func TestQueryParseSuggestion(t *testing.T) {
	tests := []struct {
		name        string
		queryString string
		want        string
	}{
		{name: "single quoted value", queryString: `type:issue .status=='open'`, want: "double quotes"},
		{name: "sql where clause", queryString: "type:issue where status = open", want: "does not use 'where'"},
		{name: "generic syntax failure", queryString: "type:issue (", want: "rvn docs querying query-language"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queryParseSuggestion(tt.queryString); !strings.Contains(got, tt.want) {
				t.Fatalf("queryParseSuggestion() = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestUnknownQuerySuggestionIncludesReadOpenForResolvableRefs(t *testing.T) {
	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(
		`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES (?, ?, ?, ?, '{}')`,
		"project/growth-experiments", "objects/project/growth-experiments.md", "project", 1,
	); err != nil {
		t.Fatalf("insert object: %v", err)
	}
	rt := &vaultruntime.Runtime{DB: db, Schema: schema.New(), VaultCfg: config.DefaultVaultConfig()}
	suggestion := unknownQuerySuggestion(rt, "growth-experiments")
	if !strings.Contains(suggestion, "rvn read") || !strings.Contains(suggestion, "rvn open") {
		t.Fatalf("suggestion = %q, want read/open hint", suggestion)
	}
}

func TestUnknownQuerySuggestionListsEveryQueryRoot(t *testing.T) {
	rt := &vaultruntime.Runtime{Schema: schema.New(), VaultCfg: config.DefaultVaultConfig()}
	suggestion := unknownQuerySuggestion(rt, "issue .status==open")
	for _, root := range []string{"type:", "trait:", "section", "link"} {
		if !strings.Contains(suggestion, root) {
			t.Fatalf("suggestion %q does not mention query root %q", suggestion, root)
		}
	}
}

func TestUnknownQuerySuggestionRecognizesSchemaType(t *testing.T) {
	sch := schema.New()
	sch.Types["issue"] = &schema.TypeDefinition{}
	rt := &vaultruntime.Runtime{Schema: sch, VaultCfg: config.DefaultVaultConfig()}
	if suggestion := unknownQuerySuggestion(rt, "issue"); !strings.Contains(suggestion, "rvn query type:issue") {
		t.Fatalf("suggestion = %q, want type query hint", suggestion)
	}
}
