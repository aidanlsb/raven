package localapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestRevisionRequiresAuth(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, Options{})
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/revision", "", "127.0.0.1", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSessionAndRevisionExposeIndexRevision(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	reindexVault(t, vault.Path)

	server := newTestServer(t, Options{
		VaultPath: vault.Path,
		Invoker:   app.CommandInvoker(),
	})
	session := getSession(t, server)
	if session.Revision.Generation == "" {
		t.Fatal("session revision generation is empty")
	}
	rev := getRevision(t, server)
	if rev != session.Revision {
		t.Fatalf("GET /v1/revision = %#v, session.revision = %#v", rev, session.Revision)
	}

	before := rev
	payload, err := json.Marshal(invokeRequest{
		Command: "set",
		Args: map[string]interface{}{
			"reference": "people/alice",
			"field":     map[string]interface{}{"name": "Freya"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "", string(payload))
	result := decodeResult(t, rec)
	if rec.Code != 200 || !result.OK {
		t.Fatalf("set failed: status=%d result=%#v", rec.Code, result)
	}
	after := getRevision(t, server)
	if after.Generation != before.Generation {
		t.Fatalf("generation changed on set: %q -> %q", before.Generation, after.Generation)
	}
	if after.Counter <= before.Counter {
		t.Fatalf("counter after set = %d, want > %d", after.Counter, before.Counter)
	}
}

func TestInvokeMutationsAdvanceRevision(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		command     string
		args        map[string]interface{}
		fullRebuild bool
	}{
		{
			name:    "delete",
			command: "delete",
			args:    map[string]interface{}{"reference": "people/alice"},
		},
		{
			name:    "write",
			command: "write",
			args: map[string]interface{}{
				"type":  "person",
				"title": "Thor",
				"field": map[string]interface{}{"name": "Thor"},
			},
		},
		{
			name:    "schema_add_type",
			command: "schema_add_type",
			args: map[string]interface{}{
				"name":       "company",
				"name-field": "name",
			},
		},
		{
			name:        "reindex",
			command:     "reindex",
			args:        map[string]interface{}{"full": true},
			fullRebuild: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vault := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
				Build()
			reindexVault(t, vault.Path)
			server := newTestServer(t, Options{
				VaultPath:  vault.Path,
				Invoker:    app.CommandInvoker(),
				ConfigPath: isolatedConfigPath(t),
			})
			before := getRevision(t, server)
			payload, err := json.Marshal(invokeRequest{Command: tc.command, Args: tc.args})
			if err != nil {
				t.Fatal(err)
			}
			rec := doJSON(t, server.Handler(), http.MethodPost, "http://127.0.0.1/v1/invoke", testToken, "127.0.0.1", "", string(payload))
			result := decodeResult(t, rec)
			if rec.Code != 200 || !result.OK {
				t.Fatalf("%s failed: status=%d result=%#v", tc.command, rec.Code, result)
			}
			after := getRevision(t, server)
			if tc.fullRebuild {
				if after.Generation == "" || after.Generation == before.Generation {
					t.Fatalf("generation after %s = %q, want a new value (was %q)", tc.command, after.Generation, before.Generation)
				}
				return
			}
			if after.Generation != before.Generation {
				t.Fatalf("generation changed on %s: %q -> %q", tc.command, before.Generation, after.Generation)
			}
			if after.Counter <= before.Counter {
				t.Fatalf("counter after %s = %d, want > %d", tc.command, after.Counter, before.Counter)
			}
		})
	}
}

func getRevision(t *testing.T, server *Server) index.Revision {
	t.Helper()
	rec := doJSON(t, server.Handler(), http.MethodGet, "http://127.0.0.1/v1/revision", testToken, "127.0.0.1", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revision status = %d body=%s", rec.Code, rec.Body.String())
	}
	result := decodeResult(t, rec)
	if !result.OK {
		t.Fatalf("revision failed: %#v", result)
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var rev index.Revision
	if err := json.Unmarshal(raw, &rev); err != nil {
		t.Fatalf("decode revision: %v\n%s", err, raw)
	}
	return rev
}
