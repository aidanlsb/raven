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

func TestMaybeSplitInlineSavedQueryArgs(t *testing.T) {
	queries := map[string]*config.SavedQuery{
		"proj-todos": {
			Query: "trait:todo refs([[{{args.project}}]])",
			Args:  []string{"project"},
		},
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "split saved query with positional input",
			args: []string{"proj-todos raven"},
			want: []string{"proj-todos", "raven"},
		},
		{
			name: "split saved query with key value input",
			args: []string{"proj-todos project=raven"},
			want: []string{"proj-todos", "project=raven"},
		},
		{
			name: "split saved query with quoted positional input",
			args: []string{`proj-todos "raven app"`},
			want: []string{"proj-todos", "raven app"},
		},
		{
			name: "split saved query with quoted key value input",
			args: []string{`proj-todos project="raven app"`},
			want: []string{"proj-todos", "project=raven app"},
		},
		{
			name: "full query remains unchanged",
			args: []string{`trait:todo content("my task")`},
			want: []string{`trait:todo content("my task")`},
		},
		{
			name: "unknown saved query remains unchanged",
			args: []string{"unknown raven"},
			want: []string{"unknown raven"},
		},
		{
			name: "invalid quoting remains unchanged",
			args: []string{`proj-todos "raven`},
			want: []string{`proj-todos "raven`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maybeSplitInlineSavedQueryArgs(tt.args, queries)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("maybeSplitInlineSavedQueryArgs(%v) = %#v, want %#v", tt.args, got, tt.want)
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
		name         string
		queryName    string
		args         []string
		declaredArgs []string
		want         map[string]string
		wantErr      bool
	}{
		{
			name:         "no args",
			queryName:    "project-todos",
			args:         nil,
			declaredArgs: []string{"project"},
			want:         nil,
		},
		{
			name:         "key value input",
			queryName:    "project-todos",
			args:         []string{"project=projects/raven"},
			declaredArgs: []string{"project"},
			want:         map[string]string{"project": "projects/raven"},
		},
		{
			name:         "positional input",
			queryName:    "project-todos",
			args:         []string{"projects/raven"},
			declaredArgs: []string{"project"},
			want:         map[string]string{"project": "projects/raven"},
		},
		{
			name:         "mixed key value and positional",
			queryName:    "project-todos",
			args:         []string{"status=active", "projects/raven"},
			declaredArgs: []string{"project", "status"},
			want: map[string]string{
				"project": "projects/raven",
				"status":  "active",
			},
		},
		{
			name:         "inputs provided but args not declared",
			queryName:    "project-todos",
			args:         []string{"project=projects/raven"},
			declaredArgs: nil,
			wantErr:      true,
		},
		{
			name:         "unknown key",
			queryName:    "project-todos",
			args:         []string{"team=raven"},
			declaredArgs: []string{"project"},
			wantErr:      true,
		},
		{
			name:         "too many positional inputs",
			queryName:    "project-todos",
			args:         []string{"projects/raven", "extra"},
			declaredArgs: []string{"project"},
			wantErr:      true,
		},
		{
			name:         "duplicate key input",
			queryName:    "project-todos",
			args:         []string{"project=one", "project=two"},
			declaredArgs: []string{"project"},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSavedQueryInputs(tt.queryName, tt.args, tt.declaredArgs)
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
				t.Fatalf("parseSavedQueryInputs(%v, %v) = %#v, want %#v", tt.args, tt.declaredArgs, got, tt.want)
			}
		})
	}
}

func TestNormalizeSavedQueryArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      []string
		expectErr bool
	}{
		{
			name: "normalizes whitespace",
			args: []string{" project ", "status"},
			want: []string{"project", "status"},
		},
		{
			name:      "duplicate args fail",
			args:      []string{"project", "project"},
			expectErr: true,
		},
		{
			name:      "empty args fail",
			args:      []string{"project", " "},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSavedQueryArgs("project-todos", tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeSavedQueryArgs(%v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestValidateSavedQueryInputDeclarations(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		declaredArgs []string
		expectErr    bool
	}{
		{
			name:         "no template inputs",
			query:        "trait:todo",
			declaredArgs: nil,
		},
		{
			name:         "template inputs fully declared",
			query:        "trait:todo refs([[{{args.project}}]])",
			declaredArgs: []string{"project"},
		},
		{
			name:         "template inputs missing declarations",
			query:        "trait:todo refs([[{{args.project}}]])",
			declaredArgs: nil,
			expectErr:    true,
		},
		{
			name:         "template inputs partially declared",
			query:        "trait:todo refs([[{{args.project}}]]) .value=={{args.status}}",
			declaredArgs: []string{"project"},
			expectErr:    true,
		},
		{
			name:         "escaped template input ignored",
			query:        `trait:todo content("\{{args.project}}") .value=={{args.status}}`,
			declaredArgs: []string{"status"},
		},
		{
			name:         "legacy inputs alias still accepted",
			query:        "trait:todo refs([[{{inputs.project}}]]) .value=={{inputs.status}}",
			declaredArgs: []string{"project", "status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSavedQueryInputDeclarations("project-todos", tt.query, tt.declaredArgs)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestExtractSavedQueryInputRefs(t *testing.T) {
	got := extractSavedQueryInputRefs(`trait:todo refs([[{{args.project}}]]) .value=={{inputs.status}} \{{args.ignored}} {{args.project}}`)
	want := []string{"project", "status"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractSavedQueryInputRefs() = %#v, want %#v", got, want)
	}
}

func TestResolveSavedQueryQueryString(t *testing.T) {
	query := &config.SavedQuery{
		Query: "trait:todo refs([[{{args.project}}]])",
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
	if !strings.Contains(err.Error(), "unknown variable: args.project") {
		t.Fatalf("unexpected error: %v", err)
	}

	legacyQuery := &config.SavedQuery{
		Query: "trait:todo refs([[{{inputs.project}}]])",
	}
	got, err = resolveSavedQueryQueryString("project-todos", legacyQuery, map[string]string{"project": "projects/raven"})
	if err != nil {
		t.Fatalf("unexpected error for legacy query syntax: %v", err)
	}
	if got != "trait:todo refs([[projects/raven]])" {
		t.Fatalf("resolved legacy query = %q, want %q", got, "trait:todo refs([[projects/raven]])")
	}

	_, err = resolveSavedQueryQueryString("empty", &config.SavedQuery{}, nil)
	if err == nil {
		t.Fatalf("expected error for empty saved query")
	}
	if !strings.Contains(err.Error(), "has no query defined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeSavedQueryTemplateVars(t *testing.T) {
	got := normalizeSavedQueryTemplateVars(`trait:todo refs([[{{args.project}}]]) .value=={{args.status}} \{{args.literal}}`)
	want := `trait:todo refs([[{{inputs.project}}]]) .value=={{inputs.status}} \{{args.literal}}`
	if got != want {
		t.Fatalf("normalizeSavedQueryTemplateVars() = %q, want %q", got, want)
	}
}
