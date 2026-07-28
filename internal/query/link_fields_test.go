package query

import (
	"errors"
	"strings"
	"testing"
)

func TestValidator_LinkPredicates(t *testing.T) {
	t.Parallel()
	v := NewValidator(nil)

	allowed := []string{
		"link .ext==pdf",
		"link .is_image==true",
		"link .scheme==url",
		`link .source_id=="projects/raven"`,
		"link .source_type==project",
		`link startswith(.file_path, "projects/")`,
		"link .line>=10",
		"link .position_start>=0",
		"link .position_end>0",
		`link includes(.raw_target, "docs/")`,
		`link matches(.display, "guide|manual")`,
		"link within(type:project)",
		`link within(section includes(.title, "Tasks"))`,
	}
	for _, queryStr := range allowed {
		t.Run("allowed/"+queryStr, func(t *testing.T) {
			q, err := Parse(queryStr)
			if err != nil {
				t.Fatalf("Parse(%q): %v", queryStr, err)
			}
			if err := v.Validate(q); err != nil {
				t.Fatalf("Validate(%q): %v", queryStr, err)
			}
		})
	}

	rejected := []struct {
		query   string
		message string
	}{
		{"link .unknown==x", "link has no field 'unknown'"},
		{`link includes(.line, "1")`, "string function predicates are not valid for link field '.line'"},
		{"link in(type:project)", "in() predicate is not valid for link queries"},
		{"link refs([[project/raven]])", "refs() predicate is not valid for link queries"},
		{"link links(.ext==pdf)", "links() predicate is not valid for link queries"},
		{"link refd(type:project)", "refd() predicate is not valid for link queries"},
		{`link content("guide")`, "content() predicate is not valid for link queries"},
		{"link any(.ext, _ == pdf)", "array predicates are not valid for link queries"},
		{"link .is_image>0", "link field '.is_image' only supports == and !="},
		{"link .line==abc", "link field '.line' requires a numeric value"},
		{"link exists(.ext)", "exists() is not valid for link field '.ext'"},
		{"link !exists(.ext)", "!exists() is not valid for link field '.ext'"},
		{"type:project links(exists(.ext))", "exists() is not valid for link field '.ext'"},
	}
	for _, tt := range rejected {
		t.Run("rejected/"+tt.query, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.query, err)
			}
			err = v.Validate(q)
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("Validate(%q) error = %T %v, want *ValidationError", tt.query, err, err)
			}
			if !strings.Contains(validationErr.Message, tt.message) {
				t.Fatalf("message = %q, want substring %q", validationErr.Message, tt.message)
			}
		})
	}
}

func TestValidator_LinkExistsErrorSuggestsEmptyComparison(t *testing.T) {
	t.Parallel()

	q, err := Parse("link exists(.ext)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	err = NewValidator(nil).Validate(q)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Validate() error = %T %v, want *ValidationError", err, err)
	}
	if validationErr.Suggestion != `Use .ext=="" to match an indexed empty value` {
		t.Fatalf("suggestion = %q", validationErr.Suggestion)
	}
}

func TestValidator_LinkRootAndLinksPredicateShareFieldGrammar(t *testing.T) {
	t.Parallel()
	v := NewValidator(nil)

	for _, queryStr := range []string{
		`link .source_id=="projects/raven"`,
		`type:project links(.source_id=="projects/raven")`,
	} {
		q, err := Parse(queryStr)
		if err != nil {
			t.Fatalf("Parse(%q): %v", queryStr, err)
		}
		if err := v.Validate(q); err != nil {
			t.Fatalf("Validate(%q): %v", queryStr, err)
		}
	}
}
