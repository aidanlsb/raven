package query

import (
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestCompareValues_DerefsStringPointers(t *testing.T) {
	t.Parallel()
	a := "b"
	b := "a"
	if compareValues(&a, &b) <= 0 {
		t.Fatalf("expected %q > %q", a, b)
	}
}

func TestCompareValues_NormalizesNilLikeValues(t *testing.T) {
	t.Parallel()

	var nilString *string

	if compareValues(nil, nilString) != 0 {
		t.Fatal("expected plain nil and typed nil pointer to compare equal")
	}
	if compareValues(nilString, nil) != 0 {
		t.Fatal("expected typed nil pointer and plain nil to compare equal")
	}
	if compareValues(nil, "a") >= 0 {
		t.Fatal("expected nil to sort before concrete values")
	}
	if compareValues(nilString, "a") >= 0 {
		t.Fatal("expected typed nil pointer to sort before concrete values")
	}
	if compareValues("a", nilString) <= 0 {
		t.Fatal("expected concrete values to sort after typed nil pointer")
	}
}

func TestCompareValues_Numeric(t *testing.T) {
	t.Parallel()
	if compareValues("10", "2") <= 0 {
		t.Fatalf("expected numeric 10 > 2")
	}
	if compareValues(1, "1") != 0 {
		t.Fatalf("expected 1 == \"1\"")
	}
}

func TestCompareValues_Temporal(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	e := &Executor{}
	p := &ValuePredicate{
		Value:     "10",
		CompareOp: CompareGt,
	}
	cond, args := e.buildValueCondition(p, "t.value")
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
	t.Parallel()
	e := &Executor{}
	p := &ValuePredicate{
		Value:     "TODO",
		CompareOp: CompareEq,
	}
	cond, _ := e.buildValueCondition(p, "t.value")
	if cond != "LOWER(t.value) = LOWER(?)" {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildValueCondition_DateFilterToday(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 5, 10, 30, 0, 0, time.UTC)
	e := &Executor{nowFn: func() time.Time { return now }}
	p := &ValuePredicate{
		Value:     "today",
		CompareOp: CompareEq,
	}
	cond, args := e.buildValueCondition(p, "t.value")
	if cond != "t.value = ?" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	today := now.Format("2006-01-02")
	if args[0] != today {
		t.Fatalf("arg = %#v, want %q", args[0], today)
	}
}

func TestBuildValueCondition_DateFilterTomorrowNotEqual(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 5, 10, 30, 0, 0, time.UTC)
	e := &Executor{nowFn: func() time.Time { return now }}
	p := &ValuePredicate{
		Value:     "tomorrow",
		CompareOp: CompareNeq,
	}
	cond, args := e.buildValueCondition(p, "t.value")
	if cond != "t.value != ?" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	if args[0] != tomorrow {
		t.Fatalf("arg = %#v, want %q", args[0], tomorrow)
	}
}

func TestBuildCompareCondition_RelativeInstantOrdering(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 5, 10, 30, 0, 0, time.UTC)
	e := &Executor{nowFn: func() time.Time { return now }}
	cond, args := e.buildCompareCondition("today", CompareLt, false, "t.value")
	if cond != "t.value < ?" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	today := now.Format("2006-01-02")
	if args[0] != today {
		t.Fatalf("arg = %#v, want %q", args[0], today)
	}
}

func TestBuildCompareCondition_UnknownKeywordFallsBackToString(t *testing.T) {
	t.Parallel()
	e := &Executor{}
	cond, args := e.buildCompareCondition("this-week", CompareEq, false, "t.value")
	if cond != "LOWER(t.value) = LOWER(?)" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 || args[0] != "this-week" {
		t.Fatalf("args = %#v", args)
	}
}

func TestLikeCond_EscapesWildcards(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	e := &Executor{}
	p := &FieldPredicate{
		Field:     "tags",
		Value:     "Urgent",
		CompareOp: CompareEq,
	}

	cond, args, err := e.buildFieldPredicateSQL(p, "o", "")
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
	t.Parallel()
	e := &Executor{}
	p := &FieldPredicate{
		Field:     "tags",
		Value:     "Urgent",
		CompareOp: CompareNeq,
	}

	cond, args, err := e.buildFieldPredicateSQL(p, "o", "")
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

func TestBuildFieldPredicateSQL_SchemaAwareScalarEqualitySkipsJSONArrayPath(t *testing.T) {
	t.Parallel()

	e := &Executor{}
	e.SetSchema(&schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"status": {Type: schema.FieldTypeString},
				},
			},
		},
	})

	p := &FieldPredicate{
		Field:     "status",
		Value:     "active",
		CompareOp: CompareEq,
	}

	cond, args, err := e.buildFieldPredicateSQL(p, "o", "project")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(args) != 2 {
		t.Fatalf("args len = %d", len(args))
	}
	if strings.Contains(cond, "json_each") {
		t.Fatalf("expected scalar field SQL without json_each, got %q", cond)
	}
	if cond != "LOWER(json_extract(o.fields, ?)) = LOWER(?)" {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildFieldPredicateSQL_SchemaAwareArrayEqualitySkipsScalarPath(t *testing.T) {
	t.Parallel()

	e := &Executor{}
	e.SetSchema(&schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"tags": {Type: schema.FieldTypeStringArray},
				},
			},
		},
	})

	p := &FieldPredicate{
		Field:     "tags",
		Value:     "urgent",
		CompareOp: CompareEq,
	}

	cond, args, err := e.buildFieldPredicateSQL(p, "o", "project")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(args) != 2 {
		t.Fatalf("args len = %d", len(args))
	}
	if strings.Contains(cond, "LOWER(json_extract") {
		t.Fatalf("expected array field SQL without scalar equality branch, got %q", cond)
	}
	if !containsAll(cond, "EXISTS (", "FROM json_each(o.fields, ?)", "LOWER(json_each.value) = LOWER(?)") {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildElementEqualitySQL_NumericUsesCast(t *testing.T) {
	t.Parallel()
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

func TestBuildAncestorPredicateSQL_AddsDepthGuard(t *testing.T) {
	t.Parallel()

	e := &Executor{}
	p := &AncestorPredicate{
		SubQuery: &Query{Type: QueryTypeObject, TypeName: "project"},
	}

	cond, args, err := e.buildAncestorPredicateSQL(p, "o")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(args) == 0 || args[0] != recursivePredicateMaxDepth {
		t.Fatalf("args = %#v", args)
	}
	if !containsAll(cond, "WITH RECURSIVE ancestors AS", "1 AS depth", "a.depth + 1", "WHERE a.depth < ?") {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildDescendantPredicateSQL_AddsDepthGuard(t *testing.T) {
	t.Parallel()

	e := &Executor{}
	p := &DescendantPredicate{
		SubQuery: &Query{Type: QueryTypeObject, TypeName: "section"},
	}

	cond, args, err := e.buildDescendantPredicateSQL(p, "o")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(args) == 0 || args[0] != recursivePredicateMaxDepth {
		t.Fatalf("args = %#v", args)
	}
	if !containsAll(cond, "WITH RECURSIVE descendants AS", "1 AS depth", "d.depth + 1", "WHERE d.depth < ?") {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildContainsPredicateSQL_AddsDepthGuard(t *testing.T) {
	t.Parallel()

	e := &Executor{}
	p := &ContainsPredicate{
		SubQuery: &Query{Type: QueryTypeTrait, TypeName: "todo"},
	}

	cond, args, err := e.buildContainsPredicateSQL(p, "o")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(args) == 0 || args[0] != recursivePredicateMaxDepth {
		t.Fatalf("args = %#v", args)
	}
	if !containsAll(cond, "WITH RECURSIVE subtree AS", "1 AS depth", "s.depth + 1", "WHERE s.depth < ?") {
		t.Fatalf("cond = %q", cond)
	}
}

func TestBuildWithinPredicateSQL_AddsDepthGuard(t *testing.T) {
	t.Parallel()

	e := &Executor{}
	p := &WithinPredicate{
		SubQuery: &Query{Type: QueryTypeObject, TypeName: "project"},
	}

	cond, args, err := e.buildWithinPredicateSQL(p, "t")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(args) == 0 || args[0] != recursivePredicateMaxDepth {
		t.Fatalf("args = %#v", args)
	}
	if !containsAll(cond, "WITH RECURSIVE ancestors AS", "1 AS depth", "a.depth + 1", "WHERE a.depth < ?") {
		t.Fatalf("cond = %q", cond)
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
