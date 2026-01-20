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
	if len(q.Predicates) != 1 {
		t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
	}
	if gp, ok := q.Predicates[0].(*GroupPredicate); !ok || len(gp.Predicates) != 2 {
		t.Fatalf("expected GroupPredicate with 2 predicates, got %T", q.Predicates[0])
	}

	// Check pipeline
	if q.Pipeline == nil {
		t.Fatal("expected pipeline")
	}
	if len(q.Pipeline.Stages) != 4 {
		t.Fatalf("expected 4 pipeline stages, got %d", len(q.Pipeline.Stages))
	}
}

func TestParseStringFunctions(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(*testing.T, *Query)
	}{
		{
			name:  "includes on field",
			input: `object:project includes(.name, "api")`,
			checkFunc: func(t *testing.T, q *Query) {
				if len(q.Predicates) != 1 {
					t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
				}
				sfp, ok := q.Predicates[0].(*StringFuncPredicate)
				if !ok {
					t.Fatalf("expected StringFuncPredicate, got %T", q.Predicates[0])
				}
				if sfp.FuncType != StringFuncIncludes {
					t.Errorf("expected StringFuncIncludes, got %v", sfp.FuncType)
				}
				if sfp.Field != "name" {
					t.Errorf("expected field 'name', got '%s'", sfp.Field)
				}
				if sfp.Value != "api" {
					t.Errorf("expected value 'api', got '%s'", sfp.Value)
				}
				if sfp.CaseSensitive {
					t.Error("expected case-insensitive by default")
				}
			},
		},
		{
			name:  "includes case sensitive",
			input: `object:project includes(.name, "API", true)`,
			checkFunc: func(t *testing.T, q *Query) {
				sfp := q.Predicates[0].(*StringFuncPredicate)
				if !sfp.CaseSensitive {
					t.Error("expected case-sensitive")
				}
			},
		},
		{
			name:  "startswith",
			input: `object:project startswith(.name, "feature-")`,
			checkFunc: func(t *testing.T, q *Query) {
				sfp := q.Predicates[0].(*StringFuncPredicate)
				if sfp.FuncType != StringFuncStartsWith {
					t.Errorf("expected StringFuncStartsWith, got %v", sfp.FuncType)
				}
				if sfp.Value != "feature-" {
					t.Errorf("expected value 'feature-', got '%s'", sfp.Value)
				}
			},
		},
		{
			name:  "endswith",
			input: `object:project endswith(.name, "-service")`,
			checkFunc: func(t *testing.T, q *Query) {
				sfp := q.Predicates[0].(*StringFuncPredicate)
				if sfp.FuncType != StringFuncEndsWith {
					t.Errorf("expected StringFuncEndsWith, got %v", sfp.FuncType)
				}
			},
		},
		{
			name:  "matches regex",
			input: `object:person matches(.email, ".*@company\.com$")`,
			checkFunc: func(t *testing.T, q *Query) {
				sfp := q.Predicates[0].(*StringFuncPredicate)
				if sfp.FuncType != StringFuncMatches {
					t.Errorf("expected StringFuncMatches, got %v", sfp.FuncType)
				}
				if sfp.Value != `.*@company\.com$` {
					t.Errorf("expected regex pattern, got '%s'", sfp.Value)
				}
			},
		},
		{
			name:  "matches with raw string",
			input: `object:person matches(.email, r".*@company\.com$")`,
			checkFunc: func(t *testing.T, q *Query) {
				sfp := q.Predicates[0].(*StringFuncPredicate)
				// Raw string should not need double escaping
				if sfp.Value != ".*@company\\.com$" {
					t.Errorf("expected regex pattern, got '%s'", sfp.Value)
				}
			},
		},
		{
			name:  "matches with regex literal",
			input: `object:person matches(.email, /.*@company\.com$/)`,
			checkFunc: func(t *testing.T, q *Query) {
				sfp := q.Predicates[0].(*StringFuncPredicate)
				if sfp.Value != ".*@company\\.com$" {
					t.Errorf("expected regex pattern, got '%s'", sfp.Value)
				}
			},
		},
		{
			name:  "negated includes",
			input: `object:project !includes(.name, "test")`,
			checkFunc: func(t *testing.T, q *Query) {
				sfp := q.Predicates[0].(*StringFuncPredicate)
				if !sfp.Negated() {
					t.Error("expected negated predicate")
				}
			},
		},
		{
			name:  "multiple string functions",
			input: `object:project includes(.name, "api") endswith(.name, "-service")`,
			checkFunc: func(t *testing.T, q *Query) {
				if len(q.Predicates) != 1 {
					t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
				}
				gp, ok := q.Predicates[0].(*GroupPredicate)
				if !ok || len(gp.Predicates) != 2 {
					t.Fatalf("expected GroupPredicate with 2 predicates, got %T", q.Predicates[0])
				}
				sfp1 := gp.Predicates[0].(*StringFuncPredicate)
				sfp2 := gp.Predicates[1].(*StringFuncPredicate)
				if sfp1.FuncType != StringFuncIncludes {
					t.Errorf("expected first to be includes, got %v", sfp1.FuncType)
				}
				if sfp2.FuncType != StringFuncEndsWith {
					t.Errorf("expected second to be endswith, got %v", sfp2.FuncType)
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

func TestParseArrayQuantifiers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(*testing.T, *Query)
	}{
		{
			name:  "any with equality",
			input: `object:project any(.tags, _ == "urgent")`,
			checkFunc: func(t *testing.T, q *Query) {
				if len(q.Predicates) != 1 {
					t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
				}
				aqp, ok := q.Predicates[0].(*ArrayQuantifierPredicate)
				if !ok {
					t.Fatalf("expected ArrayQuantifierPredicate, got %T", q.Predicates[0])
				}
				if aqp.Quantifier != ArrayQuantifierAny {
					t.Errorf("expected ArrayQuantifierAny, got %v", aqp.Quantifier)
				}
				if aqp.Field != "tags" {
					t.Errorf("expected field 'tags', got '%s'", aqp.Field)
				}
				eep, ok := aqp.ElementPred.(*ElementEqualityPredicate)
				if !ok {
					t.Fatalf("expected ElementEqualityPredicate, got %T", aqp.ElementPred)
				}
				if eep.Value != "urgent" {
					t.Errorf("expected value 'urgent', got '%s'", eep.Value)
				}
			},
		},
		{
			name:  "all with startswith",
			input: `object:project all(.tags, startswith(_, "feature-"))`,
			checkFunc: func(t *testing.T, q *Query) {
				aqp := q.Predicates[0].(*ArrayQuantifierPredicate)
				if aqp.Quantifier != ArrayQuantifierAll {
					t.Errorf("expected ArrayQuantifierAll, got %v", aqp.Quantifier)
				}
				sfp, ok := aqp.ElementPred.(*StringFuncPredicate)
				if !ok {
					t.Fatalf("expected StringFuncPredicate, got %T", aqp.ElementPred)
				}
				if sfp.FuncType != StringFuncStartsWith {
					t.Errorf("expected StringFuncStartsWith, got %v", sfp.FuncType)
				}
				if !sfp.IsElementRef {
					t.Error("expected IsElementRef to be true")
				}
			},
		},
		{
			name:  "none with equality",
			input: `object:project none(.tags, _ == "deprecated")`,
			checkFunc: func(t *testing.T, q *Query) {
				aqp := q.Predicates[0].(*ArrayQuantifierPredicate)
				if aqp.Quantifier != ArrayQuantifierNone {
					t.Errorf("expected ArrayQuantifierNone, got %v", aqp.Quantifier)
				}
			},
		},
		{
			name:  "any with includes",
			input: `object:project any(.tags, includes(_, "api"))`,
			checkFunc: func(t *testing.T, q *Query) {
				aqp := q.Predicates[0].(*ArrayQuantifierPredicate)
				sfp := aqp.ElementPred.(*StringFuncPredicate)
				if sfp.FuncType != StringFuncIncludes {
					t.Errorf("expected StringFuncIncludes, got %v", sfp.FuncType)
				}
				if sfp.Value != "api" {
					t.Errorf("expected value 'api', got '%s'", sfp.Value)
				}
			},
		},
		{
			name:  "any with OR",
			input: `object:project any(.tags, _ == "urgent" | _ == "critical")`,
			checkFunc: func(t *testing.T, q *Query) {
				aqp := q.Predicates[0].(*ArrayQuantifierPredicate)
				orPred, ok := aqp.ElementPred.(*OrPredicate)
				if !ok {
					t.Fatalf("expected OrPredicate, got %T", aqp.ElementPred)
				}
				leftEq := orPred.Left.(*ElementEqualityPredicate)
				rightEq := orPred.Right.(*ElementEqualityPredicate)
				if leftEq.Value != "urgent" {
					t.Errorf("expected left value 'urgent', got '%s'", leftEq.Value)
				}
				if rightEq.Value != "critical" {
					t.Errorf("expected right value 'critical', got '%s'", rightEq.Value)
				}
			},
		},
		{
			name:  "any with AND",
			input: `object:project any(.tags, _ == "urgent" startswith(_, "feat-"))`,
			checkFunc: func(t *testing.T, q *Query) {
				aqp := q.Predicates[0].(*ArrayQuantifierPredicate)
				group, ok := aqp.ElementPred.(*GroupPredicate)
				if !ok || len(group.Predicates) != 2 {
					t.Fatalf("expected GroupPredicate with 2 predicates, got %T", aqp.ElementPred)
				}
				if _, ok := group.Predicates[0].(*ElementEqualityPredicate); !ok {
					t.Errorf("expected first predicate to be ElementEqualityPredicate")
				}
				if _, ok := group.Predicates[1].(*StringFuncPredicate); !ok {
					t.Errorf("expected second predicate to be StringFuncPredicate")
				}
			},
		},
		{
			name:  "negated any",
			input: `object:project !any(.tags, _ == "wontfix")`,
			checkFunc: func(t *testing.T, q *Query) {
				aqp := q.Predicates[0].(*ArrayQuantifierPredicate)
				if !aqp.Negated() {
					t.Error("expected negated predicate")
				}
			},
		},
		{
			name:  "combined with other predicates",
			input: `object:project .status==active any(.tags, _ == "urgent")`,
			checkFunc: func(t *testing.T, q *Query) {
				if len(q.Predicates) != 1 {
					t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
				}
				gp, ok := q.Predicates[0].(*GroupPredicate)
				if !ok || len(gp.Predicates) != 2 {
					t.Fatalf("expected GroupPredicate with 2 predicates, got %T", q.Predicates[0])
				}
				_, ok1 := gp.Predicates[0].(*FieldPredicate)
				_, ok2 := gp.Predicates[1].(*ArrayQuantifierPredicate)
				if !ok1 {
					t.Error("expected first predicate to be FieldPredicate")
				}
				if !ok2 {
					t.Error("expected second predicate to be ArrayQuantifierPredicate")
				}
			},
		},
		{
			name:  "element inequality",
			input: `object:project any(.tags, _ != "test")`,
			checkFunc: func(t *testing.T, q *Query) {
				aqp := q.Predicates[0].(*ArrayQuantifierPredicate)
				eep := aqp.ElementPred.(*ElementEqualityPredicate)
				if eep.CompareOp != CompareNeq {
					t.Errorf("expected CompareNeq, got %v", eep.CompareOp)
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

func TestParseRawStrings(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue string
	}{
		{
			name:      "raw string without escapes",
			input:     `object:project matches(.path, r"/api/v[0-9]+/.*")`,
			wantValue: "/api/v[0-9]+/.*",
		},
		{
			name:      "raw string with backslash",
			input:     `object:project matches(.path, r"C:\Users\.*")`,
			wantValue: "C:\\Users\\.*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			sfp := q.Predicates[0].(*StringFuncPredicate)
			if sfp.Value != tt.wantValue {
				t.Errorf("expected value '%s', got '%s'", tt.wantValue, sfp.Value)
			}
		})
	}
}
