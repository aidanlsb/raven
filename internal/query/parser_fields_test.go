package query

import (
	"testing"
)

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
