//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_DocsListOpenSearch(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithFile(".raven/docs/index.yaml", `sections:
  getting-started:
    topics:
      getting-started:
        path: getting-started.md
  querying:
    topics:
      query-language:
        path: query-language.md
`).
		WithFile(".raven/docs/getting-started/getting-started.md", "# Getting Started\n\nWelcome.\n").
		WithFile(".raven/docs/querying/query-language.md", "# Query Language\n\nquery predicate examples.\n").
		Build()

	list := v.RunCLI("docs")
	list.MustSucceed(t)
	sections := list.DataList("sections")
	if len(sections) == 0 {
		t.Fatalf("expected docs sections, got none")
	}

	listAlias := v.RunCLI("docs", "list")
	listAlias.MustSucceed(t)
	aliasSections := listAlias.DataList("sections")
	if len(aliasSections) != len(sections) {
		t.Fatalf("expected docs list alias to return %d sections, got %d", len(sections), len(aliasSections))
	}

	requireSection(t, sections, "getting-started")
	requireSection(t, sections, "querying")
	requireSection(t, aliasSections, "getting-started")
	requireSection(t, aliasSections, "querying")

	querying := v.RunCLI("docs", "querying")
	querying.MustSucceed(t)
	topics := querying.DataList("topics")
	if len(topics) == 0 {
		t.Fatalf("expected querying topics, got none")
	}
	requireTopic(t, topics, "query-language")

	open := v.RunCLI("docs", "querying", "query-language")
	open.MustSucceed(t)
	if title := open.DataString("title"); title == "" {
		t.Fatalf("expected non-empty title in docs open response")
	}
	content := open.DataString("content")
	if content == "" {
		t.Fatalf("expected non-empty content in docs open response")
	}

	search := v.RunCLI("docs", "search", "query", "--section", "querying", "--limit", "5")
	search.MustSucceed(t)
	if count, ok := search.Data["count"].(float64); !ok || count < 1 {
		t.Fatalf("expected search count >= 1, got %v", search.Data["count"])
	}
}

func TestIntegration_DocsCommandRedirectToHelp(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithFile(".raven/docs/index.yaml", `sections:
  getting-started:
    topics:
      getting-started:
        path: getting-started.md
`).
		WithFile(".raven/docs/getting-started/getting-started.md", "# Getting Started\n").
		Build()

	res := v.RunCLI("docs", "query")
	res.MustFail(t, "INVALID_INPUT")
	res.MustFailWithMessage(t, "rvn help query")
}

func requireSection(t *testing.T, sections []interface{}, id string) {
	t.Helper()
	for _, raw := range sections {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if got, _ := item["id"].(string); got == id {
			return
		}
	}
	t.Fatalf("expected section %q in %+v", id, sections)
}

func requireTopic(t *testing.T, topics []interface{}, id string) {
	t.Helper()
	for _, raw := range topics {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if got, _ := item["id"].(string); got == id {
			return
		}
	}
	t.Fatalf("expected topic %q in %+v", id, topics)
}
