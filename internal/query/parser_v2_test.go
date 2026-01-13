package query

import (
	"testing"
)

func TestParseV2FieldPredicates(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(*testing.T, *Query)
	}{
		{
			name:  "equals",
			input: "object:project .status==active",
			checkFunc: func(t *testing.T, q *Query) {
				if len(q.Predicates) != 1 {
					t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
				}
				fp, ok := q.Predicates[0].(*FieldPredicate)
				if !ok {
					t.Fatal("expected FieldPredicate")
				}
				if fp.Field != "status" {
					t.Errorf("expected field 'status', got '%s'", fp.Field)
				}
				if fp.Value != "active" {
					t.Errorf("expected value 'active', got '%s'", fp.Value)
				}
				if fp.CompareOp != CompareEq {
					t.Errorf("expected CompareEq, got %v", fp.CompareOp)
				}
			},
		},
		{
			name:  "not equals",
			input: "object:project .status!=done",
			checkFunc: func(t *testing.T, q *Query) {
				fp := q.Predicates[0].(*FieldPredicate)
				if fp.CompareOp != CompareNeq {
					t.Errorf("expected CompareNeq, got %v", fp.CompareOp)
				}
			},
		},
		{
			name:  "greater than",
			input: "object:task .priority>5",
			checkFunc: func(t *testing.T, q *Query) {
				fp := q.Predicates[0].(*FieldPredicate)
				if fp.CompareOp != CompareGt {
					t.Errorf("expected CompareGt, got %v", fp.CompareOp)
				}
				if fp.Value != "5" {
					t.Errorf("expected value '5', got '%s'", fp.Value)
				}
			},
		},
		{
			name:  "contains",
			input: `object:project .name~="website"`,
			checkFunc: func(t *testing.T, q *Query) {
				fp := q.Predicates[0].(*FieldPredicate)
				if fp.StringOp != StringContains {
					t.Errorf("expected StringContains, got %v", fp.StringOp)
				}
				if fp.Value != "website" {
					t.Errorf("expected value 'website', got '%s'", fp.Value)
				}
			},
		},
		{
			name:  "starts with",
			input: `object:project .name^="My"`,
			checkFunc: func(t *testing.T, q *Query) {
				fp := q.Predicates[0].(*FieldPredicate)
				if fp.StringOp != StringStartsWith {
					t.Errorf("expected StringStartsWith, got %v", fp.StringOp)
				}
			},
		},
		{
			name:  "ends with",
			input: `object:project .name$=".md"`,
			checkFunc: func(t *testing.T, q *Query) {
				fp := q.Predicates[0].(*FieldPredicate)
				if fp.StringOp != StringEndsWith {
					t.Errorf("expected StringEndsWith, got %v", fp.StringOp)
				}
			},
		},
		{
			name:  "regex",
			input: `object:project .name=~/^web.*api$/`,
			checkFunc: func(t *testing.T, q *Query) {
				fp := q.Predicates[0].(*FieldPredicate)
				if fp.StringOp != StringRegex {
					t.Errorf("expected StringRegex, got %v", fp.StringOp)
				}
				if fp.Value != "^web.*api$" {
					t.Errorf("expected value '^web.*api$', got '%s'", fp.Value)
				}
			},
		},
		{
			name:  "exists",
			input: "object:person .email==*",
			checkFunc: func(t *testing.T, q *Query) {
				fp := q.Predicates[0].(*FieldPredicate)
				if !fp.IsExists {
					t.Error("expected IsExists to be true")
				}
			},
		},
		{
			name:  "not exists",
			input: "object:person .email!=*",
			checkFunc: func(t *testing.T, q *Query) {
				fp := q.Predicates[0].(*FieldPredicate)
				if !fp.IsExists {
					t.Error("expected IsExists to be true")
				}
				if fp.CompareOp != CompareNeq {
					t.Errorf("expected CompareNeq, got %v", fp.CompareOp)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkFunc != nil && err == nil {
				tt.checkFunc(t, q)
			}
		})
	}
}

func TestParseV2ValuePredicates(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(*testing.T, *Query)
	}{
		{
			name:  "equals",
			input: "trait:due value==past",
			checkFunc: func(t *testing.T, q *Query) {
				vp := q.Predicates[0].(*ValuePredicate)
				if vp.Value != "past" {
					t.Errorf("expected value 'past', got '%s'", vp.Value)
				}
				if vp.CompareOp != CompareEq {
					t.Errorf("expected CompareEq, got %v", vp.CompareOp)
				}
			},
		},
		{
			name:  "less than date",
			input: "trait:due value<2025-01-01",
			checkFunc: func(t *testing.T, q *Query) {
				vp := q.Predicates[0].(*ValuePredicate)
				if vp.CompareOp != CompareLt {
					t.Errorf("expected CompareLt, got %v", vp.CompareOp)
				}
			},
		},
		{
			name:  "contains",
			input: `trait:status value~="progress"`,
			checkFunc: func(t *testing.T, q *Query) {
				vp := q.Predicates[0].(*ValuePredicate)
				if vp.StringOp != StringContains {
					t.Errorf("expected StringContains, got %v", vp.StringOp)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkFunc != nil && err == nil {
				tt.checkFunc(t, q)
			}
		})
	}
}

func TestParseV2NoShorthand(t *testing.T) {
	// These should fail because shorthands are removed
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "has shorthand fails",
			input:   "object:meeting has:due",
			wantErr: true,
		},
		{
			name:    "has with subquery works",
			input:   "object:meeting has:{trait:due}",
			wantErr: false,
		},
		{
			name:    "parent shorthand fails",
			input:   "object:meeting parent:date",
			wantErr: true,
		},
		{
			name:    "parent with subquery works",
			input:   "object:meeting parent:{object:date}",
			wantErr: false,
		},
		{
			name:    "parent with target works",
			input:   "object:meeting parent:[[daily/2025-01-01]]",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseV2Pipeline(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(*testing.T, *Query)
	}{
		{
			name:  "simple sort",
			input: "object:project .status==active |> sort(.name, asc)",
			checkFunc: func(t *testing.T, q *Query) {
				if q.Pipeline == nil {
					t.Fatal("expected pipeline")
				}
				if len(q.Pipeline.Stages) != 1 {
					t.Fatalf("expected 1 stage, got %d", len(q.Pipeline.Stages))
				}
				ss, ok := q.Pipeline.Stages[0].(*SortStage)
				if !ok {
					t.Fatal("expected SortStage")
				}
				if len(ss.Criteria) != 1 {
					t.Fatalf("expected 1 criterion, got %d", len(ss.Criteria))
				}
				if ss.Criteria[0].Field != "name" {
					t.Errorf("expected field 'name', got '%s'", ss.Criteria[0].Field)
				}
				if !ss.Criteria[0].IsField {
					t.Error("expected IsField to be true")
				}
				if ss.Criteria[0].Descending {
					t.Error("expected ascending sort")
				}
			},
		},
		{
			name:  "sort descending",
			input: "object:project |> sort(.priority, desc)",
			checkFunc: func(t *testing.T, q *Query) {
				ss := q.Pipeline.Stages[0].(*SortStage)
				if !ss.Criteria[0].Descending {
					t.Error("expected descending sort")
				}
			},
		},
		{
			name:  "limit",
			input: "object:project |> limit(10)",
			checkFunc: func(t *testing.T, q *Query) {
				ls := q.Pipeline.Stages[0].(*LimitStage)
				if ls.N != 10 {
					t.Errorf("expected limit 10, got %d", ls.N)
				}
			},
		},
		{
			name:  "sort and limit",
			input: "object:project |> sort(.name, asc) limit(5)",
			checkFunc: func(t *testing.T, q *Query) {
				if len(q.Pipeline.Stages) != 2 {
					t.Fatalf("expected 2 stages, got %d", len(q.Pipeline.Stages))
				}
				if _, ok := q.Pipeline.Stages[0].(*SortStage); !ok {
					t.Error("expected first stage to be SortStage")
				}
				if _, ok := q.Pipeline.Stages[1].(*LimitStage); !ok {
					t.Error("expected second stage to be LimitStage")
				}
			},
		},
		{
			name:  "filter",
			input: "object:project |> filter(todos > 0)",
			checkFunc: func(t *testing.T, q *Query) {
				fs := q.Pipeline.Stages[0].(*FilterStage)
				if fs.Expr.Left != "todos" {
					t.Errorf("expected left 'todos', got '%s'", fs.Expr.Left)
				}
				if fs.Expr.Op != CompareGt {
					t.Errorf("expected CompareGt, got %v", fs.Expr.Op)
				}
				if fs.Expr.Right != "0" {
					t.Errorf("expected right '0', got '%s'", fs.Expr.Right)
				}
			},
		},
		{
			name:  "assignment with subquery",
			input: "object:project |> todos = count({trait:todo})",
			checkFunc: func(t *testing.T, q *Query) {
				as := q.Pipeline.Stages[0].(*AssignmentStage)
				if as.Name != "todos" {
					t.Errorf("expected name 'todos', got '%s'", as.Name)
				}
				if as.Aggregation != AggCount {
					t.Errorf("expected AggCount, got %v", as.Aggregation)
				}
				if as.SubQuery == nil {
					t.Fatal("expected subquery")
				}
				if as.SubQuery.TypeName != "todo" {
					t.Errorf("expected type 'todo', got '%s'", as.SubQuery.TypeName)
				}
			},
		},
		{
			name:  "assignment with nav function",
			input: "object:person |> mentions = count(refd(_))",
			checkFunc: func(t *testing.T, q *Query) {
				as := q.Pipeline.Stages[0].(*AssignmentStage)
				if as.Name != "mentions" {
					t.Errorf("expected name 'mentions', got '%s'", as.Name)
				}
				if as.NavFunc == nil {
					t.Fatal("expected nav function")
				}
				if as.NavFunc.Name != "refd" {
					t.Errorf("expected nav func 'refd', got '%s'", as.NavFunc.Name)
				}
			},
		},
		{
			name:  "assignment with field aggregation",
			input: "object:project |> maxPriority = max(.priority, {object:task parent:_})",
			checkFunc: func(t *testing.T, q *Query) {
				as := q.Pipeline.Stages[0].(*AssignmentStage)
				if as.Name != "maxPriority" {
					t.Errorf("expected name 'maxPriority', got '%s'", as.Name)
				}
				if as.Aggregation != AggMax {
					t.Errorf("expected AggMax, got %v", as.Aggregation)
				}
				if as.AggField != "priority" {
					t.Errorf("expected AggField 'priority', got '%s'", as.AggField)
				}
				if as.SubQuery == nil {
					t.Fatal("expected subquery")
				}
				if as.SubQuery.TypeName != "task" {
					t.Errorf("expected type 'task', got '%s'", as.SubQuery.TypeName)
				}
			},
		},
		{
			name:  "sum with field",
			input: "object:project |> total = sum(.amount, {object:invoice refs:_})",
			checkFunc: func(t *testing.T, q *Query) {
				as := q.Pipeline.Stages[0].(*AssignmentStage)
				if as.Aggregation != AggSum {
					t.Errorf("expected AggSum, got %v", as.Aggregation)
				}
				if as.AggField != "amount" {
					t.Errorf("expected AggField 'amount', got '%s'", as.AggField)
				}
			},
		},
		{
			name:  "full pipeline",
			input: "object:project .status==active |> todos = count({trait:todo}) filter(todos > 0) sort(todos, desc) limit(10)",
			checkFunc: func(t *testing.T, q *Query) {
				if len(q.Pipeline.Stages) != 4 {
					t.Fatalf("expected 4 stages, got %d", len(q.Pipeline.Stages))
				}
				// Check each stage type
				if _, ok := q.Pipeline.Stages[0].(*AssignmentStage); !ok {
					t.Error("expected first stage to be AssignmentStage")
				}
				if _, ok := q.Pipeline.Stages[1].(*FilterStage); !ok {
					t.Error("expected second stage to be FilterStage")
				}
				if _, ok := q.Pipeline.Stages[2].(*SortStage); !ok {
					t.Error("expected third stage to be SortStage")
				}
				if _, ok := q.Pipeline.Stages[3].(*LimitStage); !ok {
					t.Error("expected fourth stage to be LimitStage")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkFunc != nil && err == nil {
				tt.checkFunc(t, q)
			}
		})
	}
}

func TestParseV2ComplexQuery(t *testing.T) {
	// Test a complex query from the spec
	input := `object:project .status==active has:{trait:due value==past} |> todos = count({trait:todo value==todo ancestor:_}) filter(todos > 0) sort(todos, desc) limit(10)`

	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Check basic structure
	if q.Type != QueryTypeObject {
		t.Error("expected object query")
	}
	if q.TypeName != "project" {
		t.Errorf("expected type 'project', got '%s'", q.TypeName)
	}

	// Check predicates
	if len(q.Predicates) != 2 {
		t.Fatalf("expected 2 predicates, got %d", len(q.Predicates))
	}

	// Check pipeline
	if q.Pipeline == nil {
		t.Fatal("expected pipeline")
	}
	if len(q.Pipeline.Stages) != 4 {
		t.Fatalf("expected 4 pipeline stages, got %d", len(q.Pipeline.Stages))
	}
}
