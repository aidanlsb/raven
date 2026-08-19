package mcp

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// compactToolParamSpecs maps each compact tool to the parameter spec that drives
// its strict validation. Tool input schemas must be generated from exactly these
// specs, so this table is the fixture for the schema-generation parity tests.
func compactToolParamSpecs() map[string]struct {
	order []string
	spec  map[string]parameterSpec
} {
	return map[string]struct {
		order []string
		spec  map[string]parameterSpec
	}{
		compactToolDiscover: {order: discoverParamOrder, spec: discoverParamSpec()},
		compactToolDescribe: {order: describeParamOrder, spec: describeParamSpec()},
		compactToolInvoke:   {order: invokeWrapperParamOrder, spec: invokeWrapperParamSpec()},
	}
}

// TestGeneratedToolSchemasMatchValidationSpecs asserts that every compact tool's
// advertised input schema is generated from the same parameter spec used for
// validation. This is the guard against re-introducing hand-written tool
// schemas that can drift from the enforced contract.
func TestGeneratedToolSchemasMatchValidationSpecs(t *testing.T) {
	t.Parallel()

	tools := make(map[string]Tool)
	for _, tool := range GenerateToolSchemas() {
		tools[tool.Name] = tool
	}

	for name, fixture := range compactToolParamSpecs() {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("missing generated tool %q", name)
		}

		wantProps := commands.ParameterProperties(fixture.order, fixture.spec)
		if !reflect.DeepEqual(tool.InputSchema.Properties, wantProps) {
			t.Fatalf("%s properties = %#v, want %#v", name, tool.InputSchema.Properties, wantProps)
		}

		wantRequired := commands.RequiredParameterNames(fixture.order, fixture.spec)
		if !reflect.DeepEqual(normalizeStrings(tool.InputSchema.Required), normalizeStrings(wantRequired)) {
			t.Fatalf("%s required = %#v, want %#v", name, tool.InputSchema.Required, wantRequired)
		}

		// The advertised property names must be exactly the parameter names the
		// validator accepts. If they diverge, the schema is no longer generated
		// from the validation spec.
		if got, want := schemaPropertyNames(tool.InputSchema.Properties), specNames(fixture.spec); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s advertises properties %v, validator accepts %v", name, got, want)
		}

		if tool.InputSchema.Type != "object" {
			t.Fatalf("%s input schema type = %q, want object", name, tool.InputSchema.Type)
		}
	}
}

// TestInvokeWrapperSchemaMatchesToolSchema asserts the wrapper schema echoed in
// INVALID_ARGS error details is derived from the same specs as the raven_invoke
// tool schema, so the schema an agent sees on failure matches discovery.
func TestInvokeWrapperSchemaMatchesToolSchema(t *testing.T) {
	t.Parallel()

	var invokeTool Tool
	for _, tool := range GenerateToolSchemas() {
		if tool.Name == compactToolInvoke {
			invokeTool = tool
		}
	}

	wrapper := compactInvokeWrapperSchema()
	if !reflect.DeepEqual(wrapper["properties"], invokeTool.InputSchema.Properties) {
		t.Fatalf("wrapper properties = %#v, want %#v", wrapper["properties"], invokeTool.InputSchema.Properties)
	}
	wrapperRequired, _ := wrapper["required"].([]string)
	if !reflect.DeepEqual(normalizeStrings(wrapperRequired), normalizeStrings(invokeTool.InputSchema.Required)) {
		t.Fatalf("wrapper required = %#v, want %#v", wrapper["required"], invokeTool.InputSchema.Required)
	}
}

// TestCompactToolsShareSingleEnvelopeShape asserts that discover, describe, and
// invoke (success and error) all emit the same envelope shape: top-level keys
// are a subset of the commandexec.Result envelope, and every payload parses as a
// commandexec.Result.
func TestCompactToolsShareSingleEnvelopeShape(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	server := NewServer(v.Path)

	discoverOut, discoverErr := server.callCompactDiscover(nil)
	describeOut, describeErr := server.callCompactDescribe(map[string]interface{}{"command": "query"})
	describeMissOut, describeMissErr := server.callCompactDescribe(map[string]interface{}{"command": "not_a_command"})
	invokeOut, invokeErr := server.callCompactInvoke(map[string]interface{}{
		"command": "new",
		"args":    map[string]interface{}{"type": "person", "title": "Alice"},
	})
	invokeBadOut, invokeBadErr := server.callCompactInvoke(map[string]interface{}{"command": "not_a_command"})

	cases := []struct {
		name    string
		out     string
		isErr   bool
		wantErr bool
	}{
		{"discover", discoverOut, discoverErr, false},
		{"describe", describeOut, describeErr, false},
		{"describe_missing", describeMissOut, describeMissErr, true},
		{"invoke_success", invokeOut, invokeErr, false},
		{"invoke_error", invokeBadOut, invokeBadErr, true},
	}

	allowed := map[string]struct{}{
		"ok": {}, "data": {}, "error": {}, "warnings": {}, "meta": {},
	}

	for _, tc := range cases {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(tc.out), &raw); err != nil {
			t.Fatalf("%s: envelope is not valid JSON object: %v; out=%s", tc.name, err, tc.out)
		}
		if _, ok := raw["ok"]; !ok {
			t.Fatalf("%s: envelope missing top-level ok: %s", tc.name, tc.out)
		}
		for key := range raw {
			if _, ok := allowed[key]; !ok {
				t.Fatalf("%s: unexpected top-level key %q in envelope: %s", tc.name, key, tc.out)
			}
		}

		var result commandexec.Result
		if err := json.Unmarshal([]byte(tc.out), &result); err != nil {
			t.Fatalf("%s: envelope does not parse as commandexec.Result: %v; out=%s", tc.name, err, tc.out)
		}
		if result.OK == tc.wantErr {
			t.Fatalf("%s: result.OK=%v, wantErr=%v; out=%s", tc.name, result.OK, tc.wantErr, tc.out)
		}
		if tc.isErr != tc.wantErr {
			t.Fatalf("%s: isError=%v, wantErr=%v; out=%s", tc.name, tc.isErr, tc.wantErr, tc.out)
		}
		if tc.wantErr && result.Error == nil {
			t.Fatalf("%s: error envelope missing error object: %s", tc.name, tc.out)
		}
	}
}

// TestCompactInvokePreservesMutationPhase guards the requirement that invoke
// results keep meta.mutation.phase after the envelope was unified around
// commandexec.Result. discover/describe (read-only) must omit it.
func TestCompactInvokePreservesMutationPhase(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	server := NewServer(v.Path)

	out, isErr := server.callCompactInvoke(map[string]interface{}{
		"command": "new",
		"args":    map[string]interface{}{"type": "person", "title": "Alice"},
	})
	if isErr {
		t.Fatalf("invoke returned error: %s", out)
	}

	var result commandexec.Result
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal invoke result: %v", err)
	}
	if result.Meta == nil || result.Meta.Mutation == nil {
		t.Fatalf("expected meta.mutation on invoke result: %s", out)
	}
	if result.Meta.Mutation.Phase != commandexec.MutationPhaseApplied {
		t.Fatalf("meta.mutation.phase = %q, want applied; out=%s", result.Meta.Mutation.Phase, out)
	}
	if result.Meta.VaultContext == nil || result.Meta.VaultContext.Path != v.Path {
		t.Fatalf("expected meta.vault_context for pinned vault: %s", out)
	}

	describeOut, _ := server.callCompactDescribe(map[string]interface{}{"command": "query"})
	var describeResult commandexec.Result
	if err := json.Unmarshal([]byte(describeOut), &describeResult); err != nil {
		t.Fatalf("unmarshal describe result: %v", err)
	}
	if describeResult.Meta != nil && describeResult.Meta.Mutation != nil {
		t.Fatalf("did not expect meta.mutation on describe: %s", describeOut)
	}
}

// TestSavedQueriesResourceMatchesCommand asserts the raven://queries/saved
// resource content matches the query_saved_list command's data, so the resource
// cannot drift from the CLI/tool output.
func TestSavedQueriesResourceMatchesCommand(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`queries:
  open-projects:
    query: "type:project .status==active"
    args: [status]
    description: "Active projects"
    options:
      limit: 20
      ids: true
`).
		Build()
	server := NewServer(v.Path)

	resource := callResourcesRead(t, server, "raven://queries/saved")
	var resourcePayload map[string]interface{}
	if err := json.Unmarshal([]byte(resource.Text), &resourcePayload); err != nil {
		t.Fatalf("unmarshal resource content: %v", err)
	}

	out, isErr := server.callCompactInvoke(map[string]interface{}{"command": "query_saved_list"})
	if isErr {
		t.Fatalf("query_saved_list failed: %s", out)
	}
	var commandEnvelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &commandEnvelope); err != nil {
		t.Fatalf("unmarshal query_saved_list: %v", err)
	}

	if !reflect.DeepEqual(resourcePayload["queries"], commandEnvelope.Data["queries"]) {
		t.Fatalf("resource queries != command queries\nresource=%#v\ncommand=%#v", resourcePayload["queries"], commandEnvelope.Data["queries"])
	}

	// Legacy runtime options remain readable but are not part of the saved
	// definition exposed by either surface.
	queries, ok := resourcePayload["queries"].([]interface{})
	if !ok || len(queries) != 1 {
		t.Fatalf("expected one saved query, got %#v", resourcePayload["queries"])
	}
	first, ok := queries[0].(map[string]interface{})
	if !ok {
		t.Fatalf("saved query entry type = %T", queries[0])
	}
	if _, ok := first["args"]; !ok {
		t.Fatalf("expected args in saved query payload: %#v", first)
	}
	if _, ok := first["options"]; ok {
		t.Fatalf("did not expect legacy options in saved query payload: %#v", first)
	}
}

// TestSchemaResourceMatchesSharedReader asserts the raven://schema/current
// resource returns exactly what the shared schema reader returns, so the
// resource does not re-implement schema-file I/O.
func TestSchemaResourceMatchesSharedReader(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	server := NewServer(v.Path)

	resource := callResourcesRead(t, server, "raven://schema/current")

	want, exists, err := schema.ReadRawSchema(v.Path)
	if err != nil {
		t.Fatalf("ReadRawSchema: %v", err)
	}
	if !exists {
		t.Fatal("expected schema.yaml to exist for test vault")
	}
	if resource.Text != want {
		t.Fatalf("schema resource content mismatch\ngot:  %q\nwant: %q", resource.Text, want)
	}
	if resource.MimeType != "text/yaml" {
		t.Fatalf("schema resource mimeType = %q, want text/yaml", resource.MimeType)
	}
}

// TestSavedQueriesPayloadParityBetweenServiceAndCommand is a lower-level guard
// that the resource shaping helper (querysvc.SavedQueryInfo.Payload) is what the
// command uses, independent of transport.
func TestSavedQueriesPayloadParityBetweenServiceAndCommand(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`queries:
  a-query:
    query: "type:project"
    description: "A"
`).
		Build()

	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{SkipSchema: true})
	result, err := querysvc.List(rt, querysvc.ListRequest{VaultPath: v.Path})
	if err != nil {
		t.Fatalf("querysvc.List: %v", err)
	}
	if len(result.Queries) != 1 {
		t.Fatalf("expected 1 saved query, got %d", len(result.Queries))
	}
	payload := result.Queries[0].Payload()
	for _, key := range []string{"name", "query", "args", "description"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing %q: %#v", key, payload)
		}
	}
}

func schemaPropertyNames(props map[string]interface{}) []string {
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func specNames(spec map[string]parameterSpec) []string {
	names := make([]string, 0, len(spec))
	for name := range spec {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeStrings(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}
