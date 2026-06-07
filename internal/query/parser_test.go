package query

import (
	"strings"
	"testing"
)

func TestParseObjectQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantType QueryType
		wantName string
		wantErr  bool
	}{
		{
			name:     "simple type query",
			input:    "type:project",
			wantType: QueryTypeObject,
			wantName: "project",
		},
		{
			name:     "simple trait query",
			input:    "trait:due",
			wantType: QueryTypeTrait,
			wantName: "due",
		},
		{
			name:     "simple asset query",
			input:    "asset",
			wantType: QueryTypeAsset,
		},
		{
			name:     "asset query with predicate",
			input:    "asset .extension==pdf",
			wantType: QueryTypeAsset,
		},
		{
			name:    "invalid query type",
			input:   "foo:bar",
			wantErr: true,
		},
		{
			name:    "missing type name",
			input:   "type:",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", q.Type, tt.wantType)
			}
			if q.TypeName != tt.wantName {
				t.Errorf("TypeName = %v, want %v", q.TypeName, tt.wantName)
			}
		})
	}
}

func TestParseRejectsAssetKind(t *testing.T) {
	t.Parallel()

	_, err := Parse("asset:pdf")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "asset query root is bare 'asset'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsLegacyObjectRoot(t *testing.T) {
	t.Parallel()

	_, err := Parse("object:project")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "legacy 'object:' queries are no longer supported; use 'type:'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsUnterminatedLiterals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unterminated string literal",
			input: `type:project .title=="Unclosed`,
		},
		{
			name:  "unterminated raw string literal",
			input: `type:project content(r"Unclosed)`,
		},
		{
			name:  "unterminated regex literal",
			input: `type:project matches(.title, /unclosed)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.input); err == nil {
				t.Fatal("expected parse error, got nil")
			}
		})
	}
}

func TestParseRejectsUnexpectedTrailingTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unmatched closing parenthesis after type",
			input: `type:project)`,
		},
		{
			name:  "unmatched closing parenthesis after predicate",
			input: `type:project .status==active)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.input); err == nil {
				t.Fatal("expected parse error, got nil")
			}
		})
	}
}

func TestParseReportsShellPipeGuidance(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:experiment_review | sort -field:created_at | head 1`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'|' (pipe) is not a shell pipe") {
		t.Fatalf("expected pipe guidance, got %q", msg)
	}
	if !strings.Contains(msg, "--pipe | jq") {
		t.Fatalf("expected shell pipeline example, got %q", msg)
	}
}

func TestParseReportsShellPipeGuidanceAfterPredicate(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:experiment_review .status==open | jq`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'|' (pipe) is not a shell pipe") {
		t.Fatalf("expected pipe guidance, got %q", msg)
	}
}

func TestParseReportsShellPipeGuidanceInArrayQuantifier(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:book any(.tags, _ == "raven" | jq)`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'|' (pipe) is not a shell pipe") {
		t.Fatalf("expected pipe guidance, got %q", msg)
	}
}

func TestParseUnexpectedTokenUsesReadableTokenName(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:project =active`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unexpected token '='") {
		t.Fatalf("expected readable token name, got %q", msg)
	}
	if strings.Contains(msg, "unexpected token 22") {
		t.Fatalf("expected symbolic token name instead of enum number, got %q", msg)
	}
}

func TestParseFieldPredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		wantField  string
		wantValue  string
		wantExists bool
		wantNeg    bool
	}{
		{
			name:      "simple field",
			input:     "type:project .status==active",
			wantField: "status",
			wantValue: "active",
		},
		{
			name:      "negated field",
			input:     "type:project !.status==done",
			wantField: "status",
			wantValue: "done",
			wantNeg:   true,
		},
		{
			name:      "quoted string value",
			input:     `type:project .title=="My Project"`,
			wantField: "title",
			wantValue: "My Project",
		},
		{
			name:      "quoted string with spaces",
			input:     `type:book .author=="J.R.R. Tolkien"`,
			wantField: "author",
			wantValue: "J.R.R. Tolkien",
		},
		{
			name:      "negated quoted string",
			input:     `type:project !.status=="in progress"`,
			wantField: "status",
			wantValue: "in progress",
			wantNeg:   true,
		},
		{
			name:      "field ref value uses wikilink target",
			input:     `type:meeting .attendees==[[people/freya|Freya]]`,
			wantField: "attendees",
			wantValue: "people/freya",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			fp, ok := q.Predicate.(*FieldPredicate)
			if !ok {
				t.Fatalf("expected FieldPredicate, got %T", q.Predicate)
			}
			if fp.Field != tt.wantField {
				t.Errorf("Field = %v, want %v", fp.Field, tt.wantField)
			}
			if !tt.wantExists && fp.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", fp.Value, tt.wantValue)
			}
			if fp.IsExists != tt.wantExists {
				t.Errorf("IsExists = %v, want %v", fp.IsExists, tt.wantExists)
			}
			if fp.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", fp.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseHasPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         string
		wantTraitName string
		wantNeg       bool
	}{
		{
			name:          "shorthand has",
			input:         "type:meeting has(trait:due)",
			wantTraitName: "due",
		},
		{
			name:          "full has subquery",
			input:         "type:meeting has(trait:due)",
			wantTraitName: "due",
		},
		{
			name:          "negated has",
			input:         "type:meeting !has(trait:due)",
			wantTraitName: "due",
			wantNeg:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			hp, ok := q.Predicate.(*HasPredicate)
			if !ok {
				t.Fatalf("expected HasPredicate, got %T", q.Predicate)
			}
			if hp.SubQuery.TypeName != tt.wantTraitName {
				t.Errorf("trait name = %v, want %v", hp.SubQuery.TypeName, tt.wantTraitName)
			}
			if hp.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", hp.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseScopeNavigationPredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
	}{
		{
			name:         "section in type",
			input:        "section in(type:project)",
			predType:     "in",
			wantTypeName: "project",
		},
		{
			name:         "section within type",
			input:        "section within(type:project)",
			predType:     "within",
			wantTypeName: "project",
		},
		{
			name:         "trait within type",
			input:        "trait:todo within(type:project)",
			predType:     "within",
			wantTypeName: "project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			var subQuery *Query
			switch p := q.Predicate.(type) {
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				subQuery = p.SubQuery
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				subQuery = p.SubQuery
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}

			if subQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", subQuery.TypeName, tt.wantTypeName)
			}
		})
	}
}

func TestParseScopeNavigationTargets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		predType   string
		wantTarget string
	}{
		{
			name:       "in target",
			input:      "section in(website)",
			predType:   "in",
			wantTarget: "website",
		},
		{
			name:       "within target",
			input:      "section within(projects/website)",
			predType:   "within",
			wantTarget: "projects/website",
		},
		{
			name:       "trait in target",
			input:      "trait:todo in(projects/website#tasks)",
			predType:   "in",
			wantTarget: "projects/website#tasks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			switch p := q.Predicate.(type) {
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				if p.Target != tt.wantTarget {
					t.Errorf("Target = %v, want %v", p.Target, tt.wantTarget)
				}
				if p.SubQuery != nil {
					t.Error("expected no SubQuery")
				}
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				if p.Target != tt.wantTarget {
					t.Errorf("Target = %v, want %v", p.Target, tt.wantTarget)
				}
				if p.SubQuery != nil {
					t.Error("expected no SubQuery")
				}
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}
		})
	}
}

func TestParseTraitPredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		predType  string
		wantValue string
	}{
		{
			name:      "value predicate",
			input:     "trait:due .value==past",
			predType:  "value",
			wantValue: "past",
		},
		{
			name:      "value predicate with quoted string",
			input:     `trait:status .value=="in progress"`,
			predType:  "value",
			wantValue: "in progress",
		},
		{
			name:      "value predicate with spaces",
			input:     `trait:priority .value=="very high"`,
			predType:  "value",
			wantValue: "very high",
		},
		{
			name:      "value predicate with ref",
			input:     "trait:assignee .value==[[people/freya]]",
			predType:  "value",
			wantValue: "people/freya",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			switch p := q.Predicate.(type) {
			case *FieldPredicate:
				if tt.predType != "value" {
					t.Fatalf("expected %s, got field", tt.predType)
				}
				if p.Field != "value" {
					t.Errorf("Field = %v, want value", p.Field)
				}
				if p.Value != tt.wantValue {
					t.Errorf("Value = %v, want %v", p.Value, tt.wantValue)
				}
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}
		})
	}
}

func TestParseInWithin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
	}{
		{
			name:         "in shorthand",
			input:        "trait:due in(type:meeting)",
			predType:     "in",
			wantTypeName: "meeting",
		},
		{
			name:         "in full",
			input:        "trait:due in(type:meeting)",
			predType:     "in",
			wantTypeName: "meeting",
		},
		{
			name:         "within shorthand",
			input:        "trait:highlight within(type:date)",
			predType:     "within",
			wantTypeName: "date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			var subQuery *Query
			switch p := q.Predicate.(type) {
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				subQuery = p.SubQuery
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				subQuery = p.SubQuery
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}

			if subQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", subQuery.TypeName, tt.wantTypeName)
			}
		})
	}
}

func TestParseInWithinTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		predType   string
		wantTarget string
	}{
		{
			name:       "in target",
			input:      "trait:due in(website)",
			predType:   "in",
			wantTarget: "website",
		},
		{
			name:       "within target",
			input:      "trait:highlight within(projects/website)",
			predType:   "within",
			wantTarget: "projects/website",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			switch p := q.Predicate.(type) {
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				if p.Target != tt.wantTarget {
					t.Errorf("Target = %v, want %v", p.Target, tt.wantTarget)
				}
				if p.SubQuery != nil {
					t.Error("expected no SubQuery")
				}
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				if p.Target != tt.wantTarget {
					t.Errorf("Target = %v, want %v", p.Target, tt.wantTarget)
				}
				if p.SubQuery != nil {
					t.Error("expected no SubQuery")
				}
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}
		})
	}
}

func TestParseBooleanComposition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "multiple predicates AND",
			input: "type:project .status==active has(trait:due)",
		},
		{
			name:  "OR predicate",
			input: "type:project .status==active | .status==done",
		},
		{
			name:  "grouped predicates",
			input: "type:project (.status==active | .status==done)",
		},
		{
			name:  "OR with function predicate",
			input: "type:project .status==active | has(trait:due)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			if tt.name == "multiple predicates AND" {
				gp, ok := q.Predicate.(*GroupPredicate)
				if !ok || len(gp.Predicates) != 2 {
					t.Errorf("expected GroupPredicate with 2 predicates, got %T", q.Predicate)
				}
			}
		})
	}
}

func TestParseRefsPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		wantTarget string
		wantSubQ   bool
		wantNeg    bool
	}{
		{
			name:       "refs with target",
			input:      "type:meeting refs([[projects/website]])",
			wantTarget: "projects/website",
		},
		{
			name:       "refs with shorthand target",
			input:      "type:meeting refs(cursor)",
			wantTarget: "cursor",
		},
		{
			name:     "refs with subquery",
			input:    "type:meeting refs(type:project)",
			wantSubQ: true,
		},
		{
			name:       "negated refs",
			input:      "type:meeting !refs([[projects/website]])",
			wantTarget: "projects/website",
			wantNeg:    true,
		},
		{
			name:     "refs with complex subquery",
			input:    "type:meeting refs(type:project .status==active)",
			wantSubQ: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			rp, ok := q.Predicate.(*RefsPredicate)
			if !ok {
				t.Fatalf("expected RefsPredicate, got %T", q.Predicate)
			}
			if tt.wantTarget != "" && rp.Target != tt.wantTarget {
				t.Errorf("Target = %v, want %v", rp.Target, tt.wantTarget)
			}
			if tt.wantSubQ && rp.SubQuery == nil {
				t.Error("expected SubQuery, got nil")
			}
			if !tt.wantSubQ && rp.SubQuery != nil {
				t.Error("expected no SubQuery")
			}
			if rp.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", rp.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseContentPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantTerm string
		wantNeg  bool
		wantErr  bool
	}{
		{
			name:     "simple content search",
			input:    `type:person content("colleague")`,
			wantTerm: "colleague",
		},
		{
			name:     "content with multiple words",
			input:    `type:project content("api design")`,
			wantTerm: "api design",
		},
		{
			name:     "negated content search",
			input:    `type:person !content("contractor")`,
			wantTerm: "contractor",
			wantNeg:  true,
		},
		{
			name:    "content without quotes",
			input:   `type:person content(colleague)`,
			wantErr: true, // requires quoted string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			cp, ok := q.Predicate.(*ContentPredicate)
			if !ok {
				t.Fatalf("expected ContentPredicate, got %T", q.Predicate)
			}
			if cp.SearchTerm != tt.wantTerm {
				t.Errorf("SearchTerm = %q, want %q", cp.SearchTerm, tt.wantTerm)
			}
			if cp.Negated() != tt.wantNeg {
				t.Errorf("Negated() = %v, want %v", cp.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseHasContainsScopePredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
		wantNeg      bool
	}{
		{
			name:         "has trait",
			input:        "type:project has(trait:todo)",
			predType:     "has",
			wantTypeName: "todo",
		},
		{
			name:         "has section",
			input:        "type:project has(section .title==Tasks)",
			predType:     "has",
			wantTypeName: "",
		},
		{
			name:         "contains trait",
			input:        "type:project contains(trait:todo .value==done)",
			predType:     "contains",
			wantTypeName: "todo",
		},
		{
			name:         "negated contains section",
			input:        "type:project !contains(section .title==Tasks)",
			predType:     "contains",
			wantTypeName: "",
			wantNeg:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			var subQuery *Query
			var negated bool
			switch p := q.Predicate.(type) {
			case *HasPredicate:
				if tt.predType != "has" {
					t.Fatalf("expected %s, got has", tt.predType)
				}
				subQuery = p.SubQuery
				negated = p.Negated()
			case *ContainsPredicate:
				if tt.predType != "contains" {
					t.Fatalf("expected %s, got contains", tt.predType)
				}
				subQuery = p.SubQuery
				negated = p.Negated()
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}

			if subQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", subQuery.TypeName, tt.wantTypeName)
			}
			if negated != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", negated, tt.wantNeg)
			}
		})
	}
}

func TestParseComplexQueries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "nested has with value",
			input: "type:meeting has(trait:due .value==past)",
		},
		{
			name:  "in with field",
			input: "trait:highlight in(type:book .status==reading)",
		},
		{
			name:  "within section",
			input: "section within(type:topic .status==active)",
		},
		{
			name:  "complex with OR",
			input: "trait:highlight (in(type:book .status==reading) | in(type:article .status==reading))",
		},
		{
			name:  "multiple field predicates",
			input: "type:project .status==active .priority==high",
		},
		{
			name:  "contains with value predicate",
			input: "type:project contains(trait:todo .value==todo)",
		},
		{
			name:  "contains section with field predicate",
			input: "type:project contains(section .title==Tasks)",
		},
		{
			name:  "combined contains and field",
			input: "type:project .status==active contains(trait:todo)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q == nil {
				t.Fatal("expected non-nil query")
			}
		})
	}
}

func TestParseAtPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         string
		wantTraitName string
		wantNeg       bool
	}{
		{
			name:          "at with shorthand trait",
			input:         "trait:due at(trait:todo)",
			wantTraitName: "todo",
		},
		{
			name:          "at with full trait subquery",
			input:         "trait:due at(trait:todo)",
			wantTraitName: "todo",
		},
		{
			name:          "at with trait subquery and value",
			input:         "trait:due at(trait:priority .value==high)",
			wantTraitName: "priority",
		},
		{
			name:          "negated at",
			input:         "trait:due !at(trait:todo)",
			wantTraitName: "todo",
			wantNeg:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			ap, ok := q.Predicate.(*AtPredicate)
			if !ok {
				t.Fatalf("expected AtPredicate, got %T", q.Predicate)
			}
			if ap.SubQuery.TypeName != tt.wantTraitName {
				t.Errorf("trait name = %v, want %v", ap.SubQuery.TypeName, tt.wantTraitName)
			}
			if ap.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", ap.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseRefdPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		wantTarget string
		wantSubQ   bool
		wantNeg    bool
	}{
		{
			name:       "refd with target",
			input:      "type:project refd([[meetings/standup]])",
			wantTarget: "meetings/standup",
		},
		{
			name:       "refd with shorthand target",
			input:      "type:project refd(cursor)",
			wantTarget: "cursor",
		},
		{
			name:     "refd with type subquery",
			input:    "type:project refd(type:meeting)",
			wantSubQ: true,
		},
		{
			name:       "negated refd",
			input:      "type:project !refd([[meetings/standup]])",
			wantTarget: "meetings/standup",
			wantNeg:    true,
		},
		{
			name:     "refd with complex subquery",
			input:    "type:person refd(type:project .status==active)",
			wantSubQ: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			rp, ok := q.Predicate.(*RefdPredicate)
			if !ok {
				t.Fatalf("expected RefdPredicate, got %T", q.Predicate)
			}
			if tt.wantTarget != "" && rp.Target != tt.wantTarget {
				t.Errorf("Target = %v, want %v", rp.Target, tt.wantTarget)
			}
			if tt.wantSubQ && rp.SubQuery == nil {
				t.Error("expected SubQuery, got nil")
			}
			if !tt.wantSubQ && rp.SubQuery != nil {
				t.Error("expected no SubQuery")
			}
			if rp.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", rp.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseComparisonOperators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         string
		wantCompareOp CompareOp
		wantValue     string
	}{
		{
			name:          "value less than",
			input:         "trait:due .value<2025-01-01",
			wantCompareOp: CompareLt,
			wantValue:     "2025-01-01",
		},
		{
			name:          "value greater than",
			input:         "trait:priority .value>5",
			wantCompareOp: CompareGt,
			wantValue:     "5",
		},
		{
			name:          "value less than or equal",
			input:         "trait:due .value<=2025-12-31",
			wantCompareOp: CompareLte,
			wantValue:     "2025-12-31",
		},
		{
			name:          "value greater than or equal",
			input:         "trait:score .value>=100",
			wantCompareOp: CompareGte,
			wantValue:     "100",
		},
		{
			name:          "value equals (default)",
			input:         "trait:status .value==active",
			wantCompareOp: CompareEq,
			wantValue:     "active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			fp, ok := q.Predicate.(*FieldPredicate)
			if !ok {
				t.Fatalf("expected FieldPredicate, got %T", q.Predicate)
			}
			if fp.Field != "value" {
				t.Errorf("Field = %v, want value", fp.Field)
			}
			if fp.CompareOp != tt.wantCompareOp {
				t.Errorf("CompareOp = %v, want %v", fp.CompareOp, tt.wantCompareOp)
			}
			if fp.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", fp.Value, tt.wantValue)
			}
		})
	}
}

func TestParseInPredicates(t *testing.T) {
	t.Parallel()
	t.Run("trait value in list", func(t *testing.T) {
		q, err := Parse(`trait:todo oneof(.value, [todo,done])`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q.Predicate == nil {
			t.Fatal("expected predicate, got nil")
		}
		if _, ok := q.Predicate.(*OrPredicate); !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicate)
		}
	})

	t.Run("trait value not in list via negation", func(t *testing.T) {
		q, err := Parse(`trait:todo !oneof(.value, [todo,done])`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q.Predicate == nil {
			t.Fatal("expected predicate, got nil")
		}
		np, ok := q.Predicate.(*NotPredicate)
		if !ok {
			t.Fatalf("expected NotPredicate, got %T", q.Predicate)
		}
		if _, ok := np.Inner.(*OrPredicate); !ok {
			t.Fatalf("expected NotPredicate wrapping OrPredicate, got %T", np.Inner)
		}
	})

	t.Run("object field in list", func(t *testing.T) {
		q, err := Parse(`type:project oneof(.status, [active,backlog])`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q.Predicate == nil {
			t.Fatal("expected predicate, got nil")
		}
		if _, ok := q.Predicate.(*OrPredicate); !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicate)
		}
	})

	t.Run("in() errors on empty list", func(t *testing.T) {
		_, err := Parse(`trait:todo oneof(.value, [])`)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("bracket list is not valid after ==", func(t *testing.T) {
		_, err := Parse(`trait:todo .value==[todo,done]`)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestParseFieldComparisonOperators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         string
		wantField     string
		wantCompareOp CompareOp
		wantValue     string
	}{
		{
			name:          "field less than",
			input:         "type:project .priority<5",
			wantField:     "priority",
			wantCompareOp: CompareLt,
			wantValue:     "5",
		},
		{
			name:          "field greater than or equal",
			input:         "type:task .count>=10",
			wantField:     "count",
			wantCompareOp: CompareGte,
			wantValue:     "10",
		},
		{
			name:          "field equals (default)",
			input:         "type:project .status==active",
			wantField:     "status",
			wantCompareOp: CompareEq,
			wantValue:     "active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			fp, ok := q.Predicate.(*FieldPredicate)
			if !ok {
				t.Fatalf("expected FieldPredicate, got %T", q.Predicate)
			}
			if fp.Field != tt.wantField {
				t.Errorf("Field = %v, want %v", fp.Field, tt.wantField)
			}
			if fp.CompareOp != tt.wantCompareOp {
				t.Errorf("CompareOp = %v, want %v", fp.CompareOp, tt.wantCompareOp)
			}
			if fp.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", fp.Value, tt.wantValue)
			}
		})
	}
}

func TestParseRefdShorthand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		wantTypeName string
	}{
		{
			name:         "refd shorthand",
			input:        "type:project refd(type:meeting)",
			wantTypeName: "meeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			rp, ok := q.Predicate.(*RefdPredicate)
			if !ok {
				t.Fatalf("expected RefdPredicate, got %T", q.Predicate)
			}
			if rp.SubQuery == nil {
				t.Fatal("expected SubQuery, got nil")
			}
			if rp.SubQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", rp.SubQuery.TypeName, tt.wantTypeName)
			}
		})
	}
}

func TestParseDirectTargetPredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		predType   string
		wantTarget string
		wantNeg    bool
	}{
		{
			name:       "in with direct target",
			input:      "section in([[projects/website]])",
			predType:   "in",
			wantTarget: "projects/website",
		},
		{
			name:       "within with direct target",
			input:      "section within([[projects/website]])",
			predType:   "within",
			wantTarget: "projects/website",
		},
		{
			name:       "contains with direct target",
			input:      "type:project contains(section .id==[[projects/website#overview]])",
			predType:   "contains",
			wantTarget: "projects/website#overview",
		},
		{
			name:       "has with direct target",
			input:      "type:project has(section .id==[[projects/website#tasks]])",
			predType:   "has",
			wantTarget: "projects/website#tasks",
		},
		{
			name:       "in with direct target (trait query)",
			input:      "trait:todo in([[projects/website]])",
			predType:   "in",
			wantTarget: "projects/website",
		},
		{
			name:       "within with direct target (trait query)",
			input:      "trait:todo within([[projects/website]])",
			predType:   "within",
			wantTarget: "projects/website",
		},
		{
			name:       "negated in with direct target",
			input:      "section !in([[projects/website]])",
			predType:   "in",
			wantTarget: "projects/website",
			wantNeg:    true,
		},
		{
			name:       "short reference",
			input:      "trait:todo within([[website]])",
			predType:   "within",
			wantTarget: "website",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			var target string
			var negated bool
			switch p := q.Predicate.(type) {
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *HasPredicate:
				if tt.predType != "has" {
					t.Fatalf("expected %s, got has", tt.predType)
				}
				target = refTargetFromSectionIDPredicate(t, p.SubQuery)
				negated = p.Negated()
			case *ContainsPredicate:
				if tt.predType != "contains" {
					t.Fatalf("expected %s, got contains", tt.predType)
				}
				target = refTargetFromSectionIDPredicate(t, p.SubQuery)
				negated = p.Negated()
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}

			if target != tt.wantTarget {
				t.Errorf("Target = %v, want %v", target, tt.wantTarget)
			}
			if negated != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", negated, tt.wantNeg)
			}
		})
	}
}

func refTargetFromSectionIDPredicate(t *testing.T, q *Query) string {
	t.Helper()
	if q == nil || q.Predicate == nil {
		t.Fatal("expected section subquery predicate")
	}
	if q.Type != QueryTypeSection {
		t.Fatalf("subquery type = %v, want section", q.Type)
	}
	fp, ok := q.Predicate.(*FieldPredicate)
	if !ok {
		t.Fatalf("subquery predicate = %T, want FieldPredicate", q.Predicate)
	}
	if fp.Field != "id" || !fp.IsRefValue {
		t.Fatalf("field predicate = %#v, want .id ref value", fp)
	}
	return fp.Value
}

func TestParseNavigationPredicateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "scope nav rejects brace subqueries",
			input:   "section in({type:date})",
			wantErr: "brace subqueries are no longer supported; use in(type:...) or in([[target]])",
		},
		{
			name:    "trait scope nav rejects brace subqueries",
			input:   "trait:due in({type:meeting})",
			wantErr: "brace subqueries are no longer supported; use in(type:...) or in([[target]])",
		},
		{
			name:    "scope nav rejects self reference",
			input:   "section in(_)",
			wantErr: "self-reference '_' is no longer supported; write an explicit target or subquery instead",
		},
		{
			name:    "trait scope nav rejects self reference",
			input:   "trait:due in(_)",
			wantErr: "self-reference '_' is no longer supported; write an explicit target or subquery instead",
		},
		{
			name:    "scope nav requires target or subquery",
			input:   "section in()",
			wantErr: "expected scope query or target in in()",
		},
		{
			name:    "trait scope nav requires target or subquery",
			input:   "trait:due in()",
			wantErr: "expected scope query or target in in()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseBooleanEdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("chained OR produces flat OrPredicate", func(t *testing.T) {
		q, err := Parse("type:project .status==active | .status==paused | .status==done | .status==archived")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		op, ok := q.Predicate.(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicate)
		}
		if len(op.Predicates) != 4 {
			t.Errorf("expected 4 OR branches, got %d", len(op.Predicates))
		}
	})

	t.Run("AND of two OR groups", func(t *testing.T) {
		q, err := Parse("type:project (.status==active | .status==paused) (.priority==high | .priority==medium)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gp, ok := q.Predicate.(*GroupPredicate)
		if !ok {
			t.Fatalf("expected GroupPredicate, got %T", q.Predicate)
		}
		if len(gp.Predicates) != 2 {
			t.Fatalf("expected 2 AND branches, got %d", len(gp.Predicates))
		}
		or1, ok1 := gp.Predicates[0].(*OrPredicate)
		or2, ok2 := gp.Predicates[1].(*OrPredicate)
		if !ok1 || !ok2 {
			t.Fatalf("expected both branches to be OrPredicate, got %T and %T", gp.Predicates[0], gp.Predicates[1])
		}
		if len(or1.Predicates) != 2 || len(or2.Predicates) != 2 {
			t.Errorf("expected 2 branches in each OR, got %d and %d", len(or1.Predicates), len(or2.Predicates))
		}
	})

	t.Run("negated AND inside OR", func(t *testing.T) {
		q, err := Parse("type:project .status==active | !(.status==paused .priority==low)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		op, ok := q.Predicate.(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicate)
		}
		if len(op.Predicates) != 2 {
			t.Fatalf("expected 2 OR branches, got %d", len(op.Predicates))
		}
		np, ok := op.Predicates[1].(*NotPredicate)
		if !ok {
			t.Fatalf("expected NotPredicate as second OR branch, got %T", op.Predicates[1])
		}
		gp, ok := np.Inner.(*GroupPredicate)
		if !ok {
			t.Fatalf("expected GroupPredicate inside NotPredicate, got %T", np.Inner)
		}
		if len(gp.Predicates) != 2 {
			t.Errorf("expected 2 AND branches inside negation, got %d", len(gp.Predicates))
		}
	})

	t.Run("negated single predicate", func(t *testing.T) {
		q, err := Parse("type:project !.status==done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		fp, ok := q.Predicate.(*FieldPredicate)
		if !ok {
			t.Fatalf("expected FieldPredicate, got %T", q.Predicate)
		}
		if !fp.Negated() {
			t.Error("expected negated predicate")
		}
	})

	t.Run("NotPredicate from negated group", func(t *testing.T) {
		q, err := Parse("type:project !(.status==active | .status==paused)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		np, ok := q.Predicate.(*NotPredicate)
		if !ok {
			t.Fatalf("expected NotPredicate, got %T", q.Predicate)
		}
		op, ok := np.Inner.(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate inside NotPredicate, got %T", np.Inner)
		}
		if len(op.Predicates) != 2 {
			t.Errorf("expected 2 OR branches, got %d", len(op.Predicates))
		}
	})

	t.Run("in() produces flat OrPredicate", func(t *testing.T) {
		q, err := Parse("type:project oneof(.status, [active,paused,done])")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		op, ok := q.Predicate.(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicate)
		}
		if len(op.Predicates) != 3 {
			t.Errorf("expected 3 OR branches from in(), got %d", len(op.Predicates))
		}
	})
}
