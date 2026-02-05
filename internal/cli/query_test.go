package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
)

func TestJoinQueryArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "single arg unchanged",
			args: []string{`trait:due content("hello world")`},
			want: `trait:due content("hello world")`,
		},
		{
			name: "multiple args joined with space",
			args: []string{"trait:due", ".value==past"},
			want: "trait:due .value==past",
		},
		{
			name: "mixed predicates",
			args: []string{"trait:due", `content("my task")`, ".value==past"},
			want: `trait:due content("my task") .value==past`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinQueryArgs(tt.args)
			if got != tt.want {
				t.Errorf("joinQueryArgs(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestBuildUnknownQuerySuggestion_IncludesReadOpenForResolvableRefs(t *testing.T) {
	// Use the real vault via local DB open; this test should stay stable because it uses
	// an in-memory index rather than relying on a specific vault.
	//
	// Create an in-memory DB and insert a known object ID so the resolver can resolve the short name.
	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.DB().Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES (?, ?, ?, ?, '{}')`,
		"project/growth-experiments",
		"objects/project/growth-experiments.md",
		"project",
		1,
	)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	s := buildUnknownQuerySuggestion(db, "growth-experiments", "daily", nil)
	if s == "" {
		t.Fatalf("expected suggestion")
	}
	if !strings.Contains(s, "rvn read") || !strings.Contains(s, "rvn open") {
		t.Fatalf("expected read/open hint, got: %q", s)
	}
}

func TestParseSavedQueryInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "no args",
			args: nil,
			want: nil,
		},
		{
			name: "single input",
			args: []string{"project=projects/raven"},
			want: map[string]string{"project": "projects/raven"},
		},
		{
			name:    "missing equals",
			args:    []string{"project"},
			wantErr: true,
		},
		{
			name:    "empty key",
			args:    []string{"=projects/raven"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSavedQueryInputs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseSavedQueryInputs(%v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestResolveSavedQueryQueryString(t *testing.T) {
	query := &config.SavedQuery{
		Query: "trait:todo refs([[{{inputs.project}}]])",
	}

	got, err := resolveSavedQueryQueryString("project-todos", query, map[string]string{"project": "projects/raven"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "trait:todo refs([[projects/raven]])" {
		t.Fatalf("resolved query = %q, want %q", got, "trait:todo refs([[projects/raven]])")
	}

	_, err = resolveSavedQueryQueryString("project-todos", query, nil)
	if err == nil {
		t.Fatalf("expected error for missing input")
	}
	if !strings.Contains(err.Error(), "unknown variable: inputs.project") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = resolveSavedQueryQueryString("empty", &config.SavedQuery{}, nil)
	if err == nil {
		t.Fatalf("expected error for empty saved query")
	}
	if !strings.Contains(err.Error(), "has no query defined") {
		t.Fatalf("unexpected error: %v", err)
	}
}
