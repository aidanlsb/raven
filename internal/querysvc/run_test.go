package querysvc

import (
	"reflect"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestMatchInvocation(t *testing.T) {
	t.Parallel()

	vaultCfg := &config.VaultConfig{
		Queries: map[string]*config.SavedQuery{
			"proj-todos": {
				Query: "trait:todo refs([[{{args.project}}]])",
				Args:  []string{"project"},
			},
		},
	}

	tests := []struct {
		name        string
		raw         string
		wantMatched bool
		wantName    string
		wantInputs  []string
	}{
		{
			name:        "positional input",
			raw:         "proj-todos raven",
			wantMatched: true,
			wantName:    "proj-todos",
			wantInputs:  []string{"raven"},
		},
		{
			name:        "key value input",
			raw:         "proj-todos project=raven",
			wantMatched: true,
			wantName:    "proj-todos",
			wantInputs:  []string{"project=raven"},
		},
		{
			name:        "quoted positional input",
			raw:         `proj-todos "raven app"`,
			wantMatched: true,
			wantName:    "proj-todos",
			wantInputs:  []string{"raven app"},
		},
		{
			name:        "bare saved query name",
			raw:         "proj-todos",
			wantMatched: true,
			wantName:    "proj-todos",
			wantInputs:  []string{},
		},
		{
			name:        "full trait query is not a saved invocation",
			raw:         `trait:todo content("my task")`,
			wantMatched: false,
		},
		{
			name:        "unknown name is not matched",
			raw:         "unknown raven",
			wantMatched: false,
		},
		{
			name:        "section root is never a saved invocation",
			raw:         "section .title==Tasks",
			wantMatched: false,
		},
		{
			name:        "link root is never a saved invocation",
			raw:         "link .ext==pdf",
			wantMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, saved, inputs, matched := MatchInvocation(vaultCfg, tt.raw)
			if matched != tt.wantMatched {
				t.Fatalf("MatchInvocation(%q) matched = %v, want %v", tt.raw, matched, tt.wantMatched)
			}
			if !matched {
				return
			}
			if name != tt.wantName {
				t.Fatalf("MatchInvocation(%q) name = %q, want %q", tt.raw, name, tt.wantName)
			}
			if saved == nil {
				t.Fatalf("MatchInvocation(%q) saved = nil, want definition", tt.raw)
			}
			if !reflect.DeepEqual(inputs, tt.wantInputs) {
				t.Fatalf("MatchInvocation(%q) inputs = %#v, want %#v", tt.raw, inputs, tt.wantInputs)
			}
		})
	}
}

func TestMatchInvocationWithoutSavedQueries(t *testing.T) {
	t.Parallel()

	if _, _, _, matched := MatchInvocation(nil, "anything"); matched {
		t.Fatalf("MatchInvocation(nil) should not match")
	}
	if _, _, _, matched := MatchInvocation(&config.VaultConfig{}, "anything"); matched {
		t.Fatalf("MatchInvocation(empty) should not match")
	}
}

func TestResolveRunOptionsUsesInvocationOptionsForSavedQuery(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	rt := newQueryRuntime(t, vaultPath)
	if _, err := Set(rt, SetRequest{
		VaultPath:   vaultPath,
		Name:        "open-issues",
		QueryString: "type:issue .status=={{args.status}}",
		Args:        []string{"status"},
	}); err != nil {
		t.Fatalf("Set() unexpected error: %v", err)
	}

	// No invocation flags: the query resolves with zero-value runtime behavior.
	opts, err := ResolveRunOptions(rt, "open-issues open", nil)
	if err != nil {
		t.Fatalf("ResolveRunOptions() unexpected error: %v", err)
	}
	if !opts.IsSaved || opts.SavedName != "open-issues" {
		t.Fatalf("ResolveRunOptions() saved = %v/%q, want true/open-issues", opts.IsSaved, opts.SavedName)
	}
	if opts.ResolvedQuery != "type:issue .status==open" {
		t.Fatalf("ResolveRunOptions() resolved = %q, want %q", opts.ResolvedQuery, "type:issue .status==open")
	}
	if opts.Browse || opts.Limit != 0 {
		t.Fatalf("ResolveRunOptions() runtime options = browse %v / limit %d, want zero values", opts.Browse, opts.Limit)
	}

	// Runtime flags are supplied when invoking the saved query.
	explicitBrowse := true
	explicitLimit := 5
	opts, err = ResolveRunOptions(rt, "open-issues open", &RunOptionOverrides{
		Browse: &explicitBrowse,
		Limit:  &explicitLimit,
	})
	if err != nil {
		t.Fatalf("ResolveRunOptions() unexpected error: %v", err)
	}
	if !opts.Browse {
		t.Fatalf("ResolveRunOptions() browse = false, want invocation value true")
	}
	if opts.Limit != 5 {
		t.Fatalf("ResolveRunOptions() limit = %d, want invocation value 5", opts.Limit)
	}
}

func TestResolveRunOptionsNonSavedQuery(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	rt := newQueryRuntime(t, vaultPath)
	explicitIDs := true
	opts, err := ResolveRunOptions(rt, "type:project .status==active", &RunOptionOverrides{
		IDs: &explicitIDs,
	})
	if err != nil {
		t.Fatalf("ResolveRunOptions() unexpected error: %v", err)
	}
	if opts.IsSaved {
		t.Fatalf("ResolveRunOptions() IsSaved = true, want false for a concrete query")
	}
	if opts.ResolvedQuery != "type:project .status==active" {
		t.Fatalf("ResolveRunOptions() resolved = %q, want the raw query", opts.ResolvedQuery)
	}
	if !opts.IDs {
		t.Fatalf("ResolveRunOptions() ids = false, want explicit true")
	}
}
