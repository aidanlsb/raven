//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_DocsListOpenSearch(t *testing.T) {
	v := testutil.NewTestVault(t).Build()

	list := v.RunCLI("docs")
	list.MustSucceed(t)
	categories := list.DataList("categories")
	if len(categories) == 0 {
		t.Fatalf("expected docs categories, got none")
	}

	requireCategory(t, categories, "guide")
	requireCategory(t, categories, "reference")

	reference := v.RunCLI("docs", "reference")
	reference.MustSucceed(t)
	topics := reference.DataList("topics")
	if len(topics) == 0 {
		t.Fatalf("expected reference topics, got none")
	}
	requireTopic(t, topics, "query-language")

	open := v.RunCLI("docs", "reference", "query-language")
	open.MustSucceed(t)
	if title := open.DataString("title"); title == "" {
		t.Fatalf("expected non-empty title in docs open response")
	}
	content := open.DataString("content")
	if content == "" {
		t.Fatalf("expected non-empty content in docs open response")
	}

	search := v.RunCLI("docs", "search", "query", "--category", "reference", "--limit", "5")
	search.MustSucceed(t)
	if count, ok := search.Data["count"].(float64); !ok || count < 1 {
		t.Fatalf("expected search count >= 1, got %v", search.Data["count"])
	}
}

func TestIntegration_DocsCommandRedirectToHelp(t *testing.T) {
	v := testutil.NewTestVault(t).Build()

	res := v.RunCLI("docs", "query")
	res.MustFail(t, "INVALID_INPUT")
	res.MustFailWithMessage(t, "rvn help query")
}

func requireCategory(t *testing.T, categories []interface{}, id string) {
	t.Helper()
	for _, raw := range categories {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if got, _ := item["id"].(string); got == id {
			return
		}
	}
	t.Fatalf("expected category %q in %+v", id, categories)
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
