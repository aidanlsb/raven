package query

import (
	"strings"
	"testing"
	"time"
)

func TestCompareValues_DerefsStringPointers(t *testing.T) {
	a := "b"
	b := "a"
	if compareValues(&a, &b) <= 0 {
		t.Fatalf("expected %q > %q", a, b)
	}
}

func TestCompareValues_Numeric(t *testing.T) {
	if compareValues("10", "2") <= 0 {
		t.Fatalf("expected numeric 10 > 2")
	}
	if compareValues(1, "1") != 0 {
		t.Fatalf("expected 1 == \"1\"")
	}
}

func TestCompareValues_Temporal(t *testing.T) {
	// date vs date
	if compareValues("2025-02-01", "2025-01-01") <= 0 {
		t.Fatalf("expected 2025-02-01 > 2025-01-01")
	}
	// datetime vs datetime
	if compareValues("2025-02-01T10:00", "2025-02-01T09:00") <= 0 {
		t.Fatalf("expected later datetime to compare greater")
	}
	// invalid date stays string (lexicographic)
	if compareValues("2025-13-45", "2025-02-01") == 0 {
		t.Fatalf("expected invalid date string to not equal valid date")
	}
}

func TestBuildValueCondition_NumericUsesCast(t *testing.T) {
	p := &ValuePredicate{
		Value:     "10",
		CompareOp: CompareGt,
	}
	cond, args := buildValueCondition(p, "t.value")
	if cond != "CAST(t.value AS REAL) > ?" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0].(float64) != 10 {
		t.Fatalf("arg = %#v", args[0])
	}
}

func TestBuildValueCondition_StringEqIsCaseInsensitive(t *testing.T) {
	p := &ValuePredicate{
		Value:     "TODO",
		CompareOp: CompareEq,
	}
	cond, _ := buildValueCondition(p, "t.value")
	if cond != "LOWER(t.value) = LOWER(?)" {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildValueCondition_DateFilterPast(t *testing.T) {
	p := &ValuePredicate{
		Value:     "past",
		CompareOp: CompareEq,
	}
	cond, args := buildValueCondition(p, "t.value")
	if cond != "t.value < ? AND t.value IS NOT NULL" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	today := time.Now().Format("2006-01-02")
	if args[0] != today {
		t.Fatalf("arg = %#v, want %q", args[0], today)
	}
}

func TestBuildValueCondition_DateFilterPastNotEqual(t *testing.T) {
	p := &ValuePredicate{
		Value:     "past",
		CompareOp: CompareNeq,
	}
	cond, args := buildValueCondition(p, "t.value")
	if cond != "NOT (t.value < ? AND t.value IS NOT NULL)" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	today := time.Now().Format("2006-01-02")
	if args[0] != today {
		t.Fatalf("arg = %#v, want %q", args[0], today)
	}
}

func TestLikeCond_EscapesWildcards(t *testing.T) {
	e := &Executor{}
	p := &StringFuncPredicate{
		FuncType:      StringFuncIncludes,
		Field:         "title",
		Value:         `a%b_c\z`,
		CaseSensitive: true,
	}

	cond, args, err := e.buildStringFuncPredicateSQL(p, "o")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cond != "json_extract(o.fields, ?) LIKE ? ESCAPE '\\'" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 2 {
		t.Fatalf("args len = %d", len(args))
	}
	if args[0] != "$.title" {
		t.Fatalf("path arg = %#v", args[0])
	}
	if args[1] != `%a\%b\_c\\z%` {
		t.Fatalf("pattern arg = %#v", args[1])
	}
}

func TestBuildFieldPredicateSQL_ScalarOrArrayCaseInsensitiveEquality(t *testing.T) {
	e := &Executor{}
	p := &FieldPredicate{
		Field:     "tags",
		Value:     "Urgent",
		CompareOp: CompareEq,
	}

	cond, args, err := e.buildFieldPredicateSQL(p, "o")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(args) != 4 {
		t.Fatalf("args len = %d", len(args))
	}
	if args[0] != "$.tags" || args[1] != "Urgent" || args[2] != "$.tags" || args[3] != "Urgent" {
		t.Fatalf("args = %#v", args)
	}

	// Spot-check the important parts without relying on exact whitespace.
	if !containsAll(cond,
		"LOWER(json_extract(o.fields, ?)) = LOWER(?)",
		"EXISTS (",
		"FROM json_each(o.fields, ?)",
		"LOWER(json_each.value) = LOWER(?)",
	) {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildFieldPredicateSQL_ScalarOrArrayCaseInsensitiveNotEquals(t *testing.T) {
	e := &Executor{}
	p := &FieldPredicate{
		Field:     "tags",
		Value:     "Urgent",
		CompareOp: CompareNeq,
	}

	cond, args, err := e.buildFieldPredicateSQL(p, "o")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(args) != 4 {
		t.Fatalf("args len = %d", len(args))
	}

	if !containsAll(cond,
		"LOWER(json_extract(o.fields, ?)) != LOWER(?)",
		"NOT EXISTS (",
		"FROM json_each(o.fields, ?)",
		"LOWER(json_each.value) = LOWER(?)",
	) {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildElementEqualitySQL_NumericUsesCast(t *testing.T) {
	e := &Executor{}
	p := &ElementEqualityPredicate{
		Value:     "10",
		CompareOp: CompareGt,
	}
	cond, args, err := e.buildElementEqualitySQL(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cond != "CAST(json_each.value AS REAL) > ?" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 || args[0].(float64) != 10 {
		t.Fatalf("args = %#v", args)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
