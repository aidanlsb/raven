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
		{`link .source_id=="projects/raven"`, "link has no field 'source_id'"},
		{`link includes(.line, "1")`, "link has no field 'line'"},
		{"link in(type:project)", "in() predicate is not valid for link queries"},
		{"link refs([[project/raven]])", "refs() predicate is not valid for link queries"},
		{"link links(.ext==pdf)", "links() predicate is not valid for link queries"},
		{"link refd(type:project)", "refd() predicate is not valid for link queries"},
		{`link content("guide")`, "content() predicate is not valid for link queries"},
		{"link any(.ext, _ == pdf)", "array predicates are not valid for link queries"},
		{"link .is_image>0", "link field '.is_image' only supports == and !="},
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
		err = v.Validate(q)
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("Validate(%q) error = %T %v, want *ValidationError", queryStr, err, err)
		}
		if !strings.Contains(validationErr.Message, "link has no field 'source_id'") {
			t.Fatalf("Validate(%q) message = %q", queryStr, validationErr.Message)
		}
	}
}
