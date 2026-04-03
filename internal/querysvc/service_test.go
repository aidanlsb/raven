package querysvc

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestParseInputs(t *testing.T) {
	t.Parallel()
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
			got, err := ParseInputs(tt.queryName, tt.args, tt.declaredArgs)
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
				t.Fatalf("ParseInputs(%v, %v) = %#v, want %#v", tt.args, tt.declaredArgs, got, tt.want)
			}
		})
	}
}

func TestParseInputsWithKeyValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		args         []string
		keyValueArgs []string
		declaredArgs []string
		want         map[string]string
		wantErr      bool
	}{
		{
			name:         "combines positional args with typed key values",
			args:         []string{"projects/raven"},
			keyValueArgs: []string{"status=active"},
			declaredArgs: []string{"project", "status"},
			want: map[string]string{
				"project": "projects/raven",
				"status":  "active",
			},
		},
		{
			name:         "allows undeclared trailing args to stay omitted when unused",
			args:         nil,
			keyValueArgs: []string{"status=active"},
			declaredArgs: []string{"status", "project"},
			want: map[string]string{
				"status": "active",
			},
		},
		{
			name:         "duplicate keys across sources fail",
			args:         []string{"status=active"},
			keyValueArgs: []string{"status=done"},
			declaredArgs: []string{"status"},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInputsWithKeyValues("project-todos", tt.args, tt.keyValueArgs, tt.declaredArgs)
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
				t.Fatalf("ParseInputsWithKeyValues() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeArgs(t *testing.T) {
	t.Parallel()
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
			got, err := NormalizeArgs(tt.args)
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
				t.Fatalf("NormalizeArgs(%v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestSplitInlineInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		want   []string
		wantOK bool
	}{
		{
			name:   "quoted positional input",
			input:  `proj-todos "raven app"`,
			want:   []string{"proj-todos", "raven app"},
			wantOK: true,
		},
		{
			name:   "quoted key value input",
			input:  `proj-todos project="raven app"`,
			want:   []string{"proj-todos", "project=raven app"},
			wantOK: true,
		},
		{
			name:   "single quoted positional input",
			input:  `proj-todos 'raven app'`,
			want:   []string{"proj-todos", "raven app"},
			wantOK: true,
		},
		{
			name:   "invalid quoting",
			input:  `proj-todos "raven app`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SplitInlineInvocation(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("SplitInlineInvocation() ok = %v, want %v", ok, tt.wantOK)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SplitInlineInvocation() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValidateInputDeclarations(t *testing.T) {
	t.Parallel()
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
			err := ValidateInputDeclarations("project-todos", tt.query, tt.declaredArgs)
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
	t.Parallel()
	got := extractSavedQueryInputRefs(`trait:todo refs([[{{args.project}}]]) .value=={{inputs.status}} \{{args.ignored}} {{args.project}}`)
	want := []string{"project", "status"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractSavedQueryInputRefs() = %#v, want %#v", got, want)
	}
}

func TestResolveQueryString(t *testing.T) {
	t.Parallel()
	query := &config.SavedQuery{
		Query: "trait:todo refs([[{{args.project}}]])",
	}

	got, err := ResolveQueryString("project-todos", query, map[string]string{"project": "projects/raven"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "trait:todo refs([[projects/raven]])" {
		t.Fatalf("resolved query = %q, want %q", got, "trait:todo refs([[projects/raven]])")
	}

	_, err = ResolveQueryString("project-todos", query, nil)
	if err == nil {
		t.Fatalf("expected error for missing input")
	}
	if !strings.Contains(err.Error(), "unknown variable: args.project") {
		t.Fatalf("unexpected error: %v", err)
	}

	legacyQuery := &config.SavedQuery{
		Query: "trait:todo refs([[{{inputs.project}}]])",
	}
	got, err = ResolveQueryString("project-todos", legacyQuery, map[string]string{"project": "projects/raven"})
	if err != nil {
		t.Fatalf("unexpected error for legacy query syntax: %v", err)
	}
	if got != "trait:todo refs([[projects/raven]])" {
		t.Fatalf("resolved legacy query = %q, want %q", got, "trait:todo refs([[projects/raven]])")
	}

	_, err = ResolveQueryString("empty", &config.SavedQuery{}, nil)
	if err == nil {
		t.Fatalf("expected error for empty saved query")
	}
	if !strings.Contains(err.Error(), "has no query defined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSavedQuery(t *testing.T) {
	t.Parallel()
	query := &config.SavedQuery{
		Query: "object:project .status=={{args.status}}",
		Args:  []string{"status", "project"},
	}

	got, err := ResolveSavedQuery("project-by-status", query, nil, []string{"status=active"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "object:project .status==active" {
		t.Fatalf("resolved query = %q, want %q", got, "object:project .status==active")
	}
}

func TestNormalizeSavedQueryTemplateVars(t *testing.T) {
	t.Parallel()
	got := normalizeSavedQueryTemplateVars(`trait:todo refs([[{{args.project}}]]) .value=={{args.status}} \{{args.literal}}`)
	want := `trait:todo refs([[{{inputs.project}}]]) .value=={{inputs.status}} \{{args.literal}}`
	if got != want {
		t.Fatalf("normalizeSavedQueryTemplateVars() = %q, want %q", got, want)
	}
}

func TestParseApplyCommand(t *testing.T) {
	t.Parallel()
	parsed, err := ParseApplyCommand([]string{"set", "status=done"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Command != "set" {
		t.Fatalf("command = %q, want %q", parsed.Command, "set")
	}
	if !reflect.DeepEqual(parsed.Args, []string{"status=done"}) {
		t.Fatalf("args = %#v, want %#v", parsed.Args, []string{"status=done"})
	}

	if _, err := ParseApplyCommand(nil); err == nil {
		t.Fatalf("expected error for empty apply command")
	}
}
