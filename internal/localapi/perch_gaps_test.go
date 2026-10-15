package localapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vault"
)

func reindexVault(t *testing.T, vaultPath string) {
	t.Helper()
	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "reindex",
		VaultPath: vaultPath,
		Args:      map[string]any{"full": true},
	})
	if !result.OK {
		t.Fatalf("reindex failed: %#v", result)
	}
}

func TestSessionDailyIdentityWithoutCreatingFile(t *testing.T) {
	t.Parallel()
	vaultDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultDir, "raven.yaml"), []byte("directories:\n  daily: journal/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, Options{
		VaultPath:  vaultDir,
		ConfigPath: isolatedConfigPath(t),
	})
	session := getSession(t, server)
	today := vault.FormatDateISO(mustToday(t))
	if session.Daily.Directory != "journal" {
		t.Fatalf("daily.directory = %q, want journal", session.Daily.Directory)
	}
	if session.Daily.TodayID != today {
		t.Fatalf("daily.today_id = %q, want %q", session.Daily.TodayID, today)
	}
	if session.Daily.TodayPath != "journal/"+today+".md" {
		t.Fatalf("daily.today_path = %q", session.Daily.TodayPath)
	}
	if session.Daily.TodayExists {
		t.Fatal("daily.today_exists = true, want false")
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "journal", today+".md")); !os.IsNotExist(err) {
		t.Fatalf("session created today's daily note: %v", err)
	}
}

func TestSessionDailyTodayExistsWhenFilePresent(t *testing.T) {
	t.Parallel()
	vaultDir := t.TempDir()
	today := vault.FormatDateISO(mustToday(t))
	dir := filepath.Join(vaultDir, "daily")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, today+".md"), []byte("---\ntype: date\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, Options{
		VaultPath:  vaultDir,
		ConfigPath: isolatedConfigPath(t),
	})
	session := getSession(t, server)
	if !session.Daily.TodayExists {
		t.Fatal("daily.today_exists = false, want true")
	}
	if session.Daily.TodayID != today {
		t.Fatalf("daily.today_id = %q, want %q", session.Daily.TodayID, today)
	}
	if session.Daily.TodayPath != "daily/"+today+".md" {
		t.Fatalf("daily.today_path = %q", session.Daily.TodayPath)
	}
}

func TestInvokeSavedQueryBindsInputsMap(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`queries:
  project-by-status:
    query: "type:project .status=={{args.status}}"
    args: [status]
`).
		WithFile("projects/alpha.md", "---\ntype: project\ntitle: Alpha\nstatus: active\n---\n").
		WithFile("projects/beta.md", "---\ntype: project\ntitle: Beta\nstatus: done\n---\n").
		Build()
	reindexVault(t, vault.Path)

	server := newTestServer(t, Options{
		VaultPath: vault.Path,
		Invoker:   app.CommandInvoker(),
	})
	payload, err := json.Marshal(invokeRequest{
		Command: "query",
		Args: map[string]interface{}{
			"query_string": "project-by-status",
			"inputs":       map[string]interface{}{"status": "active"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, server.Handler(), "POST", "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "", string(payload))
	result := decodeResult(t, rec)
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !result.OK {
		t.Fatalf("query with inputs failed: %#v", result)
	}
	typed, ok := result.Data.(commandpayload.QueryObjectResult)
	if !ok {
		// HTTP JSON round-trip through encode/decode uses generic maps.
		data := decodeDataMap(t, result)
		items, _ := data["items"].([]interface{})
		if len(items) != 1 {
			t.Fatalf("items = %#v, want 1 active project", data["items"])
		}
		item := items[0].(map[string]interface{})
		if item["id"] != "projects/alpha" {
			t.Fatalf("item = %#v, want projects/alpha", item)
		}
		return
	}
	if len(typed.Items) != 1 || typed.Items[0].ID != "projects/alpha" {
		t.Fatalf("items = %#v, want projects/alpha", typed.Items)
	}
}

func TestSavedQueryListAndSchemaExposeArgs(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`queries:
  project-todos:
    query: "trait:todo refs([[{{args.project}}]])"
    args: [project]
    description: "Todos for a project"
  open-projects:
    query: "type:project .status==active"
    description: "Active projects"
`).
		Build()
	server := newTestServer(t, Options{
		VaultPath: vault.Path,
		Invoker:   app.CommandInvoker(),
	})

	listRec := doJSON(t, server.Handler(), "POST", "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"query_saved_list","args":{}}`)
	listResult := decodeResult(t, listRec)
	if !listResult.OK {
		t.Fatalf("query_saved_list: %#v", listResult)
	}
	listData := decodeDataMap(t, listResult)
	queries, _ := listData["queries"].([]interface{})
	if len(queries) != 2 {
		t.Fatalf("queries = %#v, want 2", listData["queries"])
	}
	byName := map[string]map[string]interface{}{}
	for _, raw := range queries {
		entry := raw.(map[string]interface{})
		byName[entry["name"].(string)] = entry
	}
	todoArgs, _ := byName["project-todos"]["args"].([]interface{})
	if len(todoArgs) != 1 || todoArgs[0] != "project" {
		t.Fatalf("project-todos args = %#v, want [project]", byName["project-todos"]["args"])
	}
	openArgs, ok := byName["open-projects"]["args"].([]interface{})
	if !ok {
		t.Fatalf("open-projects args missing or not an array: %#v", byName["open-projects"]["args"])
	}
	if len(openArgs) != 0 {
		t.Fatalf("open-projects args = %#v, want []", openArgs)
	}

	schemaRec := doJSON(t, server.Handler(), "GET", "http://127.0.0.1/v1/schema", testToken, "127.0.0.1", "", "")
	schemaResult := decodeResult(t, schemaRec)
	if !schemaResult.OK {
		t.Fatalf("schema: %#v", schemaResult)
	}
	schemaData := decodeDataMap(t, schemaResult)
	schemaQueries, _ := schemaData["queries"].(map[string]interface{})
	todos := schemaQueries["project-todos"].(map[string]interface{})
	if args, _ := todos["args"].([]interface{}); len(args) != 1 || args[0] != "project" {
		t.Fatalf("schema project-todos args = %#v", todos["args"])
	}
	open := schemaQueries["open-projects"].(map[string]interface{})
	if args, ok := open["args"].([]interface{}); !ok || len(args) != 0 {
		t.Fatalf("schema open-projects args = %#v, want []", open["args"])
	}
}

func TestPerchGapsRequireAuth(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	cases := []struct {
		name, method, path, body string
	}{
		{name: "session", method: "GET", path: "http://127.0.0.1/v1/session"},
		{name: "schema", method: "GET", path: "http://127.0.0.1/v1/schema"},
		{name: "query-inputs", method: "POST", path: "http://127.0.0.1/v1/invoke",
			body: `{"command":"query","args":{"query_string":"project-todos","inputs":{"project":"projects/raven"}}}`},
		{name: "objects", method: "POST", path: "http://127.0.0.1/v1/invoke",
			body: `{"command":"objects","args":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := doJSON(t, server.Handler(), tc.method, tc.path, "", "127.0.0.1", "", tc.body)
			if rec.Code != 401 {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			result := decodeResult(t, rec)
			if result.OK || result.Error == nil || result.Error.Code != codes.ErrUnauthorized {
				t.Fatalf("result = %#v, want UNAUTHORIZED", result)
			}
		})
	}
}

func TestObjectsListsIndexCorpusWithoutBodies(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\nQueen of Asgard.\n").
		WithFile("projects/website.md", "---\ntype: project\ntitle: Website\nstatus: active\n---\nShip it.\n").
		Build()
	reindexVault(t, vault.Path)
	server := newTestServer(t, Options{
		VaultPath: vault.Path,
		Invoker:   app.CommandInvoker(),
	})

	page1 := invokeObjects(t, server, map[string]interface{}{"limit": 1, "offset": 0})
	if page1.Total != 2 || page1.Returned != 1 || !page1.HasMore || page1.NextOffset == nil || *page1.NextOffset != 1 {
		t.Fatalf("page1 pagination = %#v", page1)
	}
	if page1.Items[0].ID == "" || page1.Items[0].Type == "" || page1.Items[0].FilePath == "" {
		t.Fatalf("page1 item missing identity: %#v", page1.Items[0])
	}
	assertNoBodyFields(t, page1.Items[0])

	page2 := invokeObjects(t, server, map[string]interface{}{"limit": 1, "offset": 1})
	if page2.Returned != 1 || page2.HasMore {
		t.Fatalf("page2 pagination = %#v", page2)
	}
	if page1.Items[0].ID == page2.Items[0].ID {
		t.Fatalf("pages returned the same object %q", page1.Items[0].ID)
	}

	all := invokeObjects(t, server, map[string]interface{}{})
	if all.Returned != 2 || all.HasMore {
		t.Fatalf("unlimited pagination = %#v", all)
	}
	byID := map[string]commandpayload.ObjectSummaryItem{}
	for _, item := range all.Items {
		byID[item.ID] = item
		assertNoBodyFields(t, item)
	}
	if byID["people/freya"].Name != "Freya" || byID["people/freya"].Type != "person" {
		t.Fatalf("freya = %#v", byID["people/freya"])
	}
	if byID["projects/website"].Name != "Website" || byID["projects/website"].FilePath != "projects/website.md" {
		t.Fatalf("website = %#v", byID["projects/website"])
	}
}

func invokeObjects(t *testing.T, server *Server, args map[string]interface{}) commandpayload.ObjectsResult {
	t.Helper()
	if args == nil {
		args = map[string]interface{}{}
	}
	payload, err := json.Marshal(invokeRequest{Command: "objects", Args: args})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, server.Handler(), "POST", "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "", string(payload))
	result := decodeResult(t, rec)
	if rec.Code != 200 || !result.OK {
		t.Fatalf("objects invoke failed: status=%d result=%#v", rec.Code, result)
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var out commandpayload.ObjectsResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode objects payload: %v\n%s", err, raw)
	}
	return out
}

func assertNoBodyFields(t *testing.T, item commandpayload.ObjectSummaryItem) {
	t.Helper()
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"content", "fields", "body", "line"} {
		if _, ok := generic[key]; ok {
			t.Fatalf("object summary includes %q: %s", key, raw)
		}
	}
}

func mustToday(t *testing.T) time.Time {
	t.Helper()
	day, err := vault.ParseDateArg("today")
	if err != nil {
		t.Fatal(err)
	}
	return day
}

func TestInvokeSavedQueryUnknownArgsHint(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`queries:
  project-by-status:
    query: "type:project .status=={{args.status}}"
    args: [status]
`).
		Build()
	server := newTestServer(t, Options{
		VaultPath: vault.Path,
		Invoker:   app.CommandInvoker(),
	})
	rec := doJSON(t, server.Handler(), "POST", "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "",
		`{"command":"query","args":{"query_string":"project-by-status","args":{"status":"active"}}}`)
	result := decodeResult(t, rec)
	if result.OK || result.Error == nil || result.Error.Code != "INVALID_ARGS" {
		t.Fatalf("result = %#v, want INVALID_ARGS", result)
	}
	raw, _ := json.Marshal(result.Error.Details)
	if !strings.Contains(string(raw), "args.inputs") {
		t.Fatalf("details missing inputs hint: %s", raw)
	}
}
